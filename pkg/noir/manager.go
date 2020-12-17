package noir

import (
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	ManagerPingFrequency = 10 * time.Second
	PeerPingFrequency    = 25 * time.Second
	WorkersKey           = "noir/list_workers"
)

type Manager struct {
	id       string
	redis    *redis.Client
	updated  time.Time
	router   Router
	worker   Worker
	config   sfu.Config
	sfu      *NoirSFU
	statuses map[string]pb.WorkerData
	clients  map[string]*sfu.Peer
	rooms    map[string]Room
	mu       sync.RWMutex
}

func NewManager(sfu *NoirSFU, client *redis.Client, nodeID string) Manager {
	routerQueue := NewRedisQueue(client, RouterTopic, RouterMaxAge)
	workerQueue := NewRedisQueue(client, pb.KeyWorkerTopic(nodeID), RouterMaxAge)
	workerQueue.Cleanup()
	manager := NewRedisManager(sfu, client, nodeID)
	worker := NewWorker(nodeID, &manager, workerQueue)
	router := NewRouter(routerQueue, &manager)
	manager.SetWorker(&worker)
	manager.SetRouter(&router)
	manager.Checkin()
	return manager
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
}

func (m *Manager) CloseClient(clientID string) {
	client := m.clients[clientID]
	defer m.redis.Del(pb.KeyPeerToRoom(clientID))

	if client != nil {
		client.Close()
	}

	m.mu.Lock()
	delete(m.clients, clientID)
	m.mu.Unlock()
}

func (m *Manager) CreateClient(signal *pb.SignalRequest) (*sfu.Peer, error) {
	join := signal.GetJoin()
	pid := signal.Id
	provider := *m.SFU()
	room, err := m.CreateRoomIfNotExists(join.Sid)

	if room.Options.MaxAgeSeconds > 0 {
		if time.Now().After(GetEndTime(room)) && time.Now().Before(GetCleanupTime(room)) {
			return nil, errors.New("rejecting joining an expired room")
		}
	}

	if err != nil {
		return nil, errors.New(fmt.Sprintf("unable to ensure room %s: %s", join.Sid, err))
	}
	peer := sfu.NewPeer(provider)
	err = m.ClientPing(pid, join.Sid)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("unable to ping new peer %s: %s", pid, err))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[pid] = peer
	return peer, nil
}

func (m *Manager) ClientPing(pid string, sid string) error {
	return m.redis.Set(pb.KeyPeerToRoom(pid), sid, PeerPingFrequency).Err()
}

func (m *Manager) GetQueue(topic string, maxAge time.Duration) Queue {
	return NewRedisQueue(m.redis, topic, maxAge)
}

func (m *Manager) WorkerForRoom(roomID string) (string, error) {
	workerID, err := m.redis.Get(pb.KeyRoomToWorker(roomID)).Result()
	if err != nil {
		workerID, err = m.FirstAvailableWorkerID("room.new")
		if err != nil {
			return "", err
		}
		m.ClaimRoomNode(roomID, workerID)
	}
	return workerID, err
}

func (m *Manager) WorkerCount() int {
	return int(m.redis.HLen(WorkersKey).Val())
}

func (m *Manager) RoomCount() int {
	return len(m.rooms)
}

func NewRedisManager(provider *NoirSFU, client *redis.Client, nodeID string) Manager {
	manager := Manager{redis: client,
		statuses: make(map[string]pb.WorkerData),
		clients:  make(map[string]*sfu.Peer),
		rooms:    make(map[string]Room),
		sfu:      provider,
		id:       nodeID,
	}
	(*provider).AttachManager(&manager)
	return manager

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
	status := &pb.WorkerData{Id: id, LastUpdate: timestamppb.Now()}
	value, err := proto.Marshal(status)
	if err != nil {
		return err
	}
	err = m.redis.HSet(WorkersKey, id, value).Err()
	if err != nil {
		return err
	}
	m.statuses[id] = *status
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
	ids, err := m.redis.HKeys(WorkersKey).Result()
	if err != nil || len(ids) == 0 {
		return "", errors.New("no workers available")
	}
	return ids[rand.Intn(len(ids))], nil
}

func (m *Manager) GetRemoteWorkerData(workerID string) (*pb.WorkerData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	loaded, err := m.LoadData(pb.KeyWorkerData(workerID))
	if err != nil {
		return nil, err
	}
	status := loaded.GetWorker()
	m.statuses[workerID] = *status
	return status, nil
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

func (m *Manager) FirstAvailableWorkerID(action string) (string, error) {
	return m.RandomWorkerId()
}

func (m *Manager) UpdateAvailableWorkers() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids, err := m.redis.HKeys(WorkersKey).Result()
	if err != nil {
		log.Errorf("error getting workers from redis %s", err)
		return err
	}

	m.statuses = make(map[string]pb.WorkerData)

	for _, id := range ids {
		status, err := m.redis.HGet(WorkersKey, id).Result()
		if err != nil {
			log.Errorf("error getting worker data %s", err)
			return err
		}

		var decode pb.WorkerData

		if err := proto.Unmarshal([]byte(status), &decode); err != nil {
			log.Errorf("error decoding worker data, ignoring worker %s", err)
			delete(m.statuses, id)
			continue
		}
		if age := time.Now().Sub(decode.LastUpdate.AsTime()); age > 2*ManagerPingFrequency && decode.Id != m.worker.ID() {
			log.Warnf("haven't heard from %s; marking it offline", decode.Id)
			m.MarkOffline(decode.Id)
			continue
		}

		m.statuses[id] = decode
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
	return m.LookupPeerRoomID(signal.Id)
}

func (m *Manager) LookupPeerRoomID(peerID string) (string, error) {
	return m.redis.Get(pb.KeyPeerToRoom(peerID)).Result()
}

func (m *Manager) GetLocalRoom(roomID string) (*Room, error) {
	room := m.rooms[roomID]
	return &room, nil
}

func (m *Manager) GetRemoteRoomExists(roomID string) (bool, error) {
	val, err := m.redis.Exists(pb.KeyRoomToWorker(roomID)).Result()
	return val == 0, err
}

func (m *Manager) GetRemoteRoomData(roomID string) (*pb.RoomData, error) {
	loaded, err := m.LoadData(pb.KeyRoomData(roomID))
	if err != nil {
		log.Errorf("error loading room data! %s", err)
		return nil, err
	}
	room := m.rooms[roomID]
	room.UpdateData(loaded.GetRoom())
	return room.LatestData(), nil
}

func (m *Manager) ClaimRoomNode(roomID string, nodeID string) (bool, error) {
	return m.redis.SetNX(pb.KeyRoomToWorker(roomID), nodeID, 10*time.Second).Result()
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

// Local Memory Manager

func NewTestManager(driver string, config Config) Manager {
	rdb := redis.NewClient(&redis.Options{
		Addr:     driver,
		Password: "",
		DB:       0,
	})
	sfu := NewNoirSFU(config)
	return NewRedisManager(&sfu, rdb, RandomString(4))
}

func (m *Manager) WorkerData(id string) *pb.WorkerData {
	status := m.statuses[id]
	return &status
}

func (m *Manager) Noir() {
	if m.worker == nil || m.router == nil {
		panic("Manager not initialized")
	}
	info := time.NewTicker(5 * time.Second)
	update := time.NewTicker(20 * time.Second)
	checkin := time.NewTicker(15 * time.Second)
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	m.UpdateAvailableWorkers()

	go m.worker.HandleForever()
	go m.router.HandleForever()

	for {
		select {
		case <-checkin.C:
			m.Checkin()
		case <-update.C:
			m.UpdateAvailableWorkers()
		case <-info.C:
			log.Debugf("%s: noirs=%d users=%d rooms=%d", m.worker.ID(), len(m.statuses), len(m.clients), m.RoomCount())
		case <-quit:
			log.Warnf("quit requested, cleaning up...")
			info.Stop()
			update.Stop()
			m.Cleanup()
			log.Debugf("cleaned up ok!")
			os.Exit(1)
			return
		}
	}
}

func (m *Manager) MarkOffline(workerID string) {
	m.redis.HDel(WorkersKey, workerID)
}

func (m *Manager) OpenRoomFromRequest(admin *pb.RoomAdminRequest) error {
	_, err := m.GetRemoteRoomData(admin.RoomID)
	if err == nil {
		return errors.New("room already exists") // Room exists
	}
	openRoom := admin.GetOpenRoom()

	log.Infof("creating room %s", admin.RoomID)
	room := NewRoom(admin.RoomID)
	room.SetOptions(openRoom.GetOptions())
	SaveRoomData(admin.RoomID, &room.data, m)
	return nil
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
		log.Infof("looked up room %s, expires at %s", roomID, GetEndTime(data))
		log.Infof("looked up room %s, cleanup at %s", roomID, GetCleanupTime(data))
		if err == nil {
			return data, nil // Room exists
		}
		return nil, err
	} else {
		// Room did not exist, create it
		log.Infof("room %s does not exist, auto-creating", roomID)

		room := NewRoom(roomID)
		room.data.Options.MaxAgeSeconds = 30
		room.data.Options.KeyExpiryFactor = 2

		SaveRoomData(roomID, &room.data, m)

		return &room.data, nil
	}
}
