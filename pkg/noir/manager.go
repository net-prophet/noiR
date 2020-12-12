package noir

import (
	"errors"
	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	ManagerPingFrequency  = 10 * time.Second
	PeerPingFrequency = 25 * time.Second
	WorkersKey       = "noir/list_workers"
)

type Manager struct {
	id string
	redis    *redis.Client
	updated  time.Time
	router   Router
	worker   Worker
	config   sfu.Config
	sfu      *NoirSFU
	statuses map[string]pb.WorkerStatus
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


func (m *Manager) OpenRoom(roomID string, session *sfu.Session) *Room {
	room := NewRoom(roomID, m.ID(), session)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms[roomID] = room
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
	client.Close()

	m.mu.Lock()
	delete(m.clients, clientID)
	m.mu.Unlock()
}

func (m *Manager) CreateClient(signal *pb.SignalRequest) *sfu.Peer {
	join := signal.GetJoin()
	pid := signal.Id
	provider := *m.SFU()
	peer := sfu.NewPeer(provider)
	m.ClientPing(pid, join.Sid)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[pid] = peer
	return peer
}

func (m *Manager) ClientPing(pid string, sid string) error {
	return m.redis.Set(pb.KeyPeerToRoom(pid), sid, PeerPingFrequency).Err()
}

func(m *Manager) GetQueue(topic string, maxAge time.Duration) Queue {
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
		statuses: make(map[string]pb.WorkerStatus),
		clients:  make(map[string]*sfu.Peer),
		rooms:    make(map[string]Room),
		sfu: provider,
		id: nodeID,
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
	now, _ := time.Now().MarshalText()
	status := &pb.WorkerStatus{Id: id, LastUpdate: string(now)}
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

func (m *Manager) GetRemoteWorkerStatus(workerID string) (*pb.WorkerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	loaded, err := m.LoadStatus(pb.KeyWorkerStatus(workerID))
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

	m.statuses = make(map[string]pb.WorkerStatus)

	for _, id := range ids {
		status, err := m.redis.HGet(WorkersKey, id).Result()
		if err != nil {
			log.Errorf("error getting worker status %s", err)
			return err
		}

		var decode pb.WorkerStatus

		if err := proto.Unmarshal([]byte(status), &decode); err != nil {
			log.Errorf("error decoding worker status, ignoring worker %s", err)
			delete(m.statuses, id)
			continue
		}
		var last time.Time
		last.UnmarshalText([]byte(decode.LastUpdate))
		if age := time.Now().Sub(last); age > 2*ManagerPingFrequency && decode.Id != m.worker.ID() {
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

func (m *Manager) GetRemoteRoomStatus(roomID string) (*pb.RoomStatus, error) {
	loaded, err := m.LoadStatus(pb.KeyRoomStatus(roomID))
	if err != nil {
		return nil, err
	}
	room := m.rooms[roomID]
	room.UpdateStatus(loaded.GetRoom())
	return room.LatestStatus(), nil
}

func (m *Manager) ClaimRoomNode(roomID string, nodeID string) (bool, error) {
	return m.redis.SetNX(pb.KeyRoomToWorker(roomID), nodeID, 10*time.Second).Result()
}

// Only on RedisManager
func (m *Manager) SaveStatus(key string, status *pb.NoirStatus, expiry time.Duration) error {
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

func (m *Manager) LoadStatus(key string) (*pb.NoirStatus, error) {
	var load *pb.NoirStatus
	data, err := m.redis.Get(key).Result()
	if err != nil {
		log.Warnf("failed loading noirstatus %s", err)
		return nil, err
	}
	if err = proto.Unmarshal([]byte(data), load); err != nil {
		log.Warnf("failed unmarshaling noirstatus %s", err)
		return nil, err
	}

	return load, nil
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

func (m *Manager) WorkerStatus(id string) *pb.WorkerStatus {
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
