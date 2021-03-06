package noir

import (
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	ManagerPingFrequency = 10 * time.Second
	QueueMessageTimeout  = 25 * time.Second
)

type Manager struct {
	id           string
	redis        *redis.Client
	updated      time.Time
	router       Router
	worker       Worker
	config       sfu.Config
	sfu          *NoirSFU
	nodes        map[string]pb.NodeData
	users        map[string]*sfu.Peer
	rooms        map[string]Room
	nodeServices []string
	mu           sync.RWMutex
}

func SetupNoir(sfu *NoirSFU, client *redis.Client, nodeID string, services string) Manager {
	routerQueue := NewRedisQueue(client, RouterTopic, RouterMaxAge)
	workerQueue := NewRedisQueue(client, pb.KeyWorkerTopic(nodeID), RouterMaxAge)
	workerQueue.Cleanup()
	manager := NewRedisManager(sfu, client, nodeID, services)
	worker := NewWorker(nodeID, &manager, workerQueue)
	router := NewRouter(routerQueue, &manager)
	manager.SetWorker(&worker)
	manager.SetRouter(&router)
	manager.Checkin()
	return manager
}

func NewRedisManager(provider *NoirSFU, client *redis.Client, nodeID string, services string) Manager {
	manager := Manager{redis: client,
		nodes:        make(map[string]pb.NodeData),
		users:        make(map[string]*sfu.Peer),
		rooms:        make(map[string]Room),
		sfu:          provider,
		id:           nodeID,
		nodeServices: strings.Split(services, ","),
	}
	(*provider).AttachManager(&manager)
	return manager
}

// The Noir thread launches the worker and router, and then
// handles ticker tasks (update cluster health, print status)
// and watches for the quit servers to cleanup
func (m *Manager) Noir() {
	if m.worker == nil || m.router == nil {
		panic("Manager not initialized")
	}
	info := time.NewTicker(5 * time.Second)
	updateNodes := time.NewTicker(20 * time.Second)
	checkin := time.NewTicker(15 * time.Second)
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	if err := m.Checkin(); err != nil {
		panic("unable to checkin node as healthy")
	}

	if err := m.UpdateAvailableNodes(); err != nil {
		panic("unable to retrieve cluster status")
	}

	// Worker is always enabled, but we check services for Signal and Admin
	go m.worker.HandleForever()

	if m.IsServiceEnabled("router") {
		go m.router.HandleForever()
	}

	for {
		select {
		case <-checkin.C:
			if err := m.Checkin(); err != nil {
				panic("unable to checkin node as healthy")
			}
		case <-updateNodes.C:
			if err := m.UpdateAvailableNodes(); err != nil {
				panic("unable to retrieve cluster status")
			}
			if len(m.nodes) == 0 {
				panic("no node jobData found in redis (not even my own!)")
			}
		case <-info.C:
			log.Infof("%s: noirs=%d rooms=%d users=%d",
				m.worker.ID(),
				len(m.nodes),
				m.RoomCount(),
				len(m.users),
			)
		case <-quit:
			log.Warnf("quit requested, cleaning up...")
			info.Stop()
			updateNodes.Stop()
			m.Cleanup()
			log.Debugf("cleaned up ok!")
			os.Exit(1)
			return
		}
	}
}

func (m *Manager) IsServiceEnabled(service string) bool {
	return ServiceInList(service, m.nodeServices)
}

func ServiceInList(service string, check []string) bool {
	for _, v := range check {
		if v == service || v == "*" {
			return true
		}
	}
	return false
}

func (m *Manager) BindRoomSession(room Room, session *sfu.Session) *Room {
	room.Bind(session, m)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms[room.id] = room
	return &room
}

func (m *Manager) CloseRoom(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rooms, roomID)
	m.redis.ZRem(pb.KeyRoomScores(), roomID)
}

func (m *Manager) DisconnectUser(userID string) {
	userData, err := m.GetRemoteUserData(userID)

	// Cleanup the SFU peer
	client := m.users[userID]
	if client != nil {
		client.Close()
	}

	if userData != nil && err == nil {

		if userData.Options.MaxAgeSeconds == -1 {
			defer m.redis.Del(pb.KeyUserData(userID))
		}

		defer m.redis.HDel(pb.KeyRoomUsers(userData.RoomID), userID)

		m.UpdateRoomScore(userData.RoomID)
	}

	// Send Kill to the Peer Queues
	toPeerQueue := m.GetQueue(pb.KeyTopicToPeer(userID))
	fromPeerQueue := m.GetQueue(pb.KeyTopicFromPeer(userID))

	EnqueueRequest(toPeerQueue, &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: userID,
				Payload: &pb.SignalRequest_Kill{
					Kill: true,
				},
			},
		},
	})

	EnqueueReply(fromPeerQueue, &pb.NoirReply{
		Command: &pb.NoirReply_Signal{
			Signal: &pb.SignalReply{
				Id: userID,
				Payload: &pb.SignalReply_Kill{
					Kill: true,
				},
			},
		},
	})

	m.mu.Lock()
	delete(m.users, userID)
	m.mu.Unlock()
}

func (m *Manager) ConnectUser(signal *pb.SignalRequest) (*sfu.Peer, *pb.UserData, error) {
	join := signal.GetJoin()
	pid := signal.Id
	provider := *m.SFU()
	room, err := m.CreateRoomIfNotExists(join.Sid)

	var offer webrtc.SessionDescription
	offer = webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(join.Description),
	}

	if room.Options.MaxAgeSeconds > 0 {
		if time.Now().After(GetRoomEndTime(room)) && time.Now().Before(GetRoomCleanupTime(room)) {
			return nil, nil, errors.New("rejecting joining an expired room")
		}
	}

	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("unable to ensure room %s: %s", join.Sid, err))
	}
	desc, err := ParseSDP(offer)
	if err != nil {
		return nil, nil, err
	}

	numTracks := len(desc.MediaDescriptions)

	var publishing bool

	if numTracks == 1 && desc.MediaDescriptions[0].MediaName.Media == "application" {
		publishing = false
	} else if numTracks >= 1 {
		// we have more than 1 media track, or the 1 track we have is not jobData
		publishing = true
	}

	peer := sfu.NewPeer(provider)

	// TODO -- Check if user exists first
	userData := &pb.UserData{
		Id:         pid,
		LastUpdate: timestamppb.Now(),
		RoomID:     join.Sid,
		Publishing: publishing,
		Options:    &pb.UserOptions{MaxAgeSeconds: -1},
	}

	m.SaveData(pb.KeyUserData(pid), &pb.NoirObject{Data: &pb.NoirObject_User{User: userData}}, 0)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[pid] = peer

	m.UpdateRoomScore(join.Sid)

	return peer, userData, nil
}

func (m *Manager) UpdateRoomScore(roomID string) {
	if room, ok := m.rooms[roomID]; ok {
		score := float64(len(room.session.Peers()))
		if score > 0 {
			m.redis.ZAdd(pb.KeyRoomScores(), redis.Z{
				Score:  score,
				Member: roomID,
			})
		} else {
			m.redis.ZRem(pb.KeyRoomScores(), roomID)
		}
	} else {
	}
}

func (m *Manager) GetQueue(topic string) Queue {
	return NewRedisQueue(m.redis, topic, QueueMessageTimeout)
}

func (m *Manager) WorkerForRoom(roomID string) (string, error) {
	roomData, err := m.GetRemoteRoomData(roomID)
	return roomData.GetNodeID(), err
}

func (m *Manager) NodeCount() int {
	return int(m.redis.HLen(pb.KeyNodeMap()).Val())
}

func (m *Manager) RoomCount() int {
	return len(m.rooms)
}

func (m *Manager) ID() string {
	return m.id
}

func (m *Manager) SFU() *NoirSFU {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sfu
}

func (m *Manager) Checkin() error {
	id := m.worker.ID()
	status := &pb.NoirObject{
		Data: &pb.NoirObject_Node{
			Node: &pb.NodeData{
				Id:         id,
				LastUpdate: timestamppb.Now(),
				Services:   m.nodeServices,
			},
		},
	}
	value, err := proto.Marshal(status)
	if err != nil {
		return err
	}
	err = m.redis.HSet(pb.KeyNodeMap(), id, value).Err()
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) SetWorker(w *Worker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worker = *w
}

func (m *Manager) SetRouter(r *Router) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.router = *r
}

func (m *Manager) RandomWorkerId() (string, error) {
	ids, err := m.redis.HKeys(pb.KeyNodeMap()).Result()
	if err != nil || len(ids) == 0 {
		return "", errors.New("no nodes available")
	}
	return ids[rand.Intn(len(ids))], nil
}

func (m *Manager) GetRemoteWorkerQueue(id string) *Queue {
	queue := NewRedisWorkerQueue(m.redis, id)
	return &queue
}

func (m *Manager) GetRouter() *Router {
	return &(m.router)
}

func (m *Manager) GetWorker() *Worker {
	return &(m.worker)
}

func (m *Manager) GetNodes() map[string]pb.NodeData {
	return m.nodes
}

func (m *Manager) NodesForService(service string) []string {
	if m.NeedsUpdate() {
		m.UpdateAvailableNodes()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	available := []string{}
	for _, nodeData := range m.nodes {
		if ServiceInList(service, nodeData.Services) && ValidateHealthy(&nodeData) {
			available = append(available, nodeData.Id)
		}
	}
	log.Debugf("available for %s: %s", service, available)
	return available
}

func (m *Manager) RandomNodeForService(service string) (string, error) {
	candidates := m.NodesForService(service)
	if len(candidates) > 0 {
		return candidates[rand.Intn(len(candidates))], nil
	} else {
		return "", errors.New("No " + service + " nodes available")
	}
}

func (m *Manager) UpdateAvailableNodes() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids, err := m.redis.HKeys(pb.KeyNodeMap()).Result()
	if err != nil {
		log.Errorf("error getting nodes from redis %s", err)
		return err
	}

	m.nodes = make(map[string]pb.NodeData)

	for _, id := range ids {
		status, err := m.redis.HGet(pb.KeyNodeMap(), id).Result()
		if err != nil {
			log.Errorf("error getting worker jobData %s", err)
			return err
		}

		var decode pb.NoirObject

		if err := proto.Unmarshal([]byte(status), &decode); err != nil {
			log.Errorf("error decoding worker jobData, ignoring worker %s", err)
			delete(m.nodes, id)
			continue
		}
		remote := decode.GetNode()

		if !ValidateHealthy(remote) && remote.Id != m.worker.ID() {
			log.Warnf("haven't heard from %s; marking it offline", remote.Id)
			m.MarkOffline(remote.Id)
			continue
		}

		m.nodes[id] = *decode.GetNode()
	}

	m.updated = time.Now()
	return nil
}

func (m *Manager) Cleanup() {
	m.MarkOffline(m.worker.ID())
	workQueue := *m.worker.GetQueue()
	workQueue.Cleanup()
	m.redis.Close()
}

func (m *Manager) LookupSignalRoomID(signal *pb.SignalRequest) (string, error) {
	switch signal.Payload.(type) {
	case *pb.SignalRequest_Join:
		return signal.GetJoin().Sid, nil
	}
	user, err := m.GetRemoteUserData(signal.Id)
	if err != nil || user == nil {
		return "", errors.New("user not found")
	}
	return user.RoomID, nil
}

func (m *Manager) GetLocalRoom(roomID string) (*Room, error) {
	room := m.rooms[roomID]
	return &room, nil
}

func (m *Manager) GetRemoteRoomExists(roomID string) (bool, error) {
	val, err := m.redis.Exists(pb.KeyRoomData(roomID)).Result()
	return int(val) == 1, err
}

func (m *Manager) GetRemoteRoomData(roomID string) (*pb.RoomData, error) {
	loaded, err := m.LoadData(pb.KeyRoomData(roomID))
	if err != nil {
		log.Errorf("error loading room jobData! %s", err)
		return nil, err
	}
	room := m.rooms[roomID]
	room.UpdateData(loaded.GetRoom())
	return room.LatestData(), nil
}

func (m *Manager) ClaimRoomNode(roomID string, nodeID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if exists, _ := m.GetRemoteRoomExists(roomID); exists == false {
		room := NewRoom(roomID) // Just used for the jobData
		data := &room.data
		data.NodeID = m.id
		err := SaveRoomData(roomID, data, m)
		m.redis.HSet(pb.KeyNodeRooms(m.id), roomID, 1)
		log.Infof("claimed room %s", roomID)
		return err == nil, err
	} else {
		data, err := m.GetRemoteRoomData(roomID)
		if err == nil && data.NodeID != "" {
			if _, alive := m.nodes[data.NodeID]; alive {
				log.Warnf("tried claiming busy room")
				return false, nil
			}
			data.NodeID = m.id
			err := SaveRoomData(roomID, data, m)
			m.redis.HSet(pb.KeyNodeRooms(m.id), roomID, 1)
			log.Infof("claimed room %s", roomID)
			return err == nil, err
		} else if err != nil {
			log.Errorf("error claiming room %s", err)
			return false, err
		}
	}
	return false, nil
}

// Only on RedisManager
func (m *Manager) SaveData(key string, status *pb.NoirObject, expiry time.Duration) error {
	data, err := proto.Marshal(status)

	if err != nil {
		log.Warnf("failed marshaling noirstatus %s", err)
		return err
	}

	err = m.redis.Set(key, data, expiry).Err()

	if err != nil {
		log.Warnf("failed marshaling noirstatus %s", err)
		return err
	}

	return nil
}

func (m *Manager) LoadData(key string) (*pb.NoirObject, error) {
	var load pb.NoirObject
	data, err := m.redis.Get(key).Result()
	if err != nil {
		log.Warnf("failed loading noirstatus %s", err)
		return nil, err
	}
	if err = proto.Unmarshal([]byte(data), &load); err != nil {
		log.Warnf("failed unmarshaling noirstatus %s", err)
		return nil, err
	}

	return &load, nil
}

func (m *Manager) WorkerData(id string) *pb.NodeData {
	status := m.nodes[id]
	return &status
}

func (m *Manager) MarkOffline(nodeID string) {
	for _, room := range m.redis.HKeys(pb.KeyNodeRooms(nodeID)).Val() {
		for _, user := range m.redis.HKeys(pb.KeyRoomUsers(room)).Val() {
			m.redis.Del(pb.KeyUserData(user))
		}
	}
	m.redis.Del(pb.KeyNodeRooms(nodeID))
	m.redis.HDel(pb.KeyNodeMap(), nodeID)
}

func (m *Manager) CountMatchingKeys(pattern string) (int64, error) {
	CountMatching := redis.NewScript(`
		local result = redis.call('SCAN', ARGV[1], 'MATCH', ARGV[2], 'COUNT', 1000)
        result[2] = #result[2]
        return result
	`)

	output, err := CountMatching.Run(m.redis, []string{}, []string{"0", pattern}).Result()
	result := output.([]string)

	if err != nil {
		return 0, err
	}
	sum := int64(0)
	for result[0] != "0" {
		add, _ := strconv.Atoi(result[1])
		sum += int64(add)
	}
	return sum, nil
}

func (m *Manager) CreateRoomIfNotExists(roomID string) (*pb.RoomData, error) {
	if room, ok := m.rooms[roomID]; ok {
		return &room.data, nil // Room exists
	}
	exists, err := m.redis.Exists(pb.KeyRoomData(roomID)).Result()

	if err != nil {
		return nil, err
	}

	if exists == 1 {
		data, err := m.GetRemoteRoomData(roomID)
		if err == nil {
			return data, nil // Room exists
		}
		return nil, err
	} else {
		// Room did not exist, create it
		log.Infof("room %s does not exist, auto-creating", roomID)

		room := NewRoom(roomID)
		room.data.NodeID = m.id
		room.data.LastUpdate = timestamppb.Now()

		SaveRoomData(roomID, &room.data, m)

		return &room.data, nil
	}
}

func (m *Manager) ValidateOffer(room *pb.RoomData, userID string, offer webrtc.SessionDescription) (*sdp.SessionDescription, error) {
	desc, err := ParseSDP(offer)
	return desc, err
}

func (m *Manager) GetRemoteUserData(userID string) (*pb.UserData, error) {
	loaded, err := m.LoadData(pb.KeyUserData(userID))
	if err != nil {
		log.Errorf("error loading user#%s jobData! %s", userID, err)
		return nil, err
	}
	return loaded.GetUser(), nil
}

func (m *Manager) GetRemoteNodeData(nodeID string) (*pb.NodeData, error) {
	loaded := &pb.NoirObject{}
	data, err := m.redis.HGet(pb.KeyNodeMap(), nodeID).Result()
	if err != nil {
		log.Warnf("failed loading noirstatus %s", err)
		return nil, err
	}

	if err = proto.Unmarshal([]byte(data), loaded); err != nil {
		log.Warnf("failed unmarshaling noirstatus %s", err)
		return nil, err
	}

	if err != nil {
		log.Errorf("error loading room jobData! %s", err)
		return nil, err
	}
	return loaded.GetNode(), nil
}

func (m *Manager) ValidateHealthyNodeID(nodeID string) error {
	exists, _ := m.redis.HExists(pb.KeyNodeMap(), nodeID).Result()
	if exists == false {
		return errors.New("no such room")
	}

	data, invalid := m.GetRemoteNodeData(nodeID)
	if invalid == nil && ValidateHealthy(data) {
		return nil
	}
	return invalid
}

func ValidateHealthy(node *pb.NodeData) bool {
	age := time.Now().Sub(node.GetLastUpdate().AsTime())
	healthyWindow := 2 * ManagerPingFrequency
	return age < healthyWindow
}

func (m *Manager) NeedsUpdate() bool {
	age := time.Now().Sub(m.updated)
	healthyWindow := ManagerPingFrequency / 2
	healthyAge := (age < healthyWindow)
	hasNodes := len(m.nodes) > 0
	return !healthyAge || !hasNodes
}
