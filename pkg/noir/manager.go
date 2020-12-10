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
	UpdateFrequency  = 10 * time.Second
	CheckinFrequency = 5 * time.Second
	WorkersKey       = "noir/list_workers"
)

type Manager interface {
	SFU() *NoirSFU
	FirstAvailableWorkerID(string) (string, error)
	Checkin() error
	SetWorker(worker *Worker)
	SetRouter(router *Router)
	UpdateAvailableWorkers() error
	WorkerCount() int
	WorkerStatus(workerID string) *pb.WorkerStatus
	GetRemoteWorkerStatus(workerID string) (*pb.WorkerStatus, error)
	GetRemoteWorkerQueue(workerID string) *Queue
	GetRouter() *Router
	GetWorker() *Worker
	Join(request *pb.SignalRequest) *NoirPeer
	Start()
	Cleanup()
	LookupSignalRoomID(*pb.SignalRequest) (string, error)
	WorkerForRoom(roomID string) (string, error)
	GetLocalRoom(roomID string) (*Room, error)
	GetRemoteRoomStatus(roomID string) (*pb.RoomStatus, error)
	ClaimRoomNode(roomID string, nodeID string) (bool, error)
}

type manager struct {
	redis    *redis.Client
	updated  time.Time
	router   Router
	worker   Worker
	sfu      NoirSFU
	statuses map[string]pb.WorkerStatus
	peers    map[string]NoirPeer
	rooms    map[string]Room
	mu 		sync.RWMutex
}

func (m *manager) Join(signal *pb.SignalRequest) *NoirPeer {
	m.mu.Lock()
	defer m.mu.Unlock()
	join := signal.GetJoin()
	pid := signal.Id
	peer := NewNoirPeer(m.sfu, pid, join.Sid)
	m.peers[pid] = peer
	return &peer
}

func (m *manager) WorkerForRoom(roomID string) (string, error) {
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

func (m *manager) WorkerCount() int {
	return int(m.redis.HLen(WorkersKey).Val())
}

func NewRedisManager(c sfu.Config, client *redis.Client) Manager {
	sfu := NewNoirSFU(c)
	return &manager{redis: client, sfu: sfu, statuses: make(map[string]pb.WorkerStatus),
		peers: make(map[string]NoirPeer),
		rooms: make(map[string]Room),
	}
}

func (m *manager) SFU() *NoirSFU {
	return &m.sfu
}

func (m *manager) Checkin() error {
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

func (m *manager) SetWorker(w *Worker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worker = *w
}

func (m *manager) SetRouter(r *Router) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.router = *r
}

func (m *manager) RandomWorkerId() (string, error) {
	ids, err := m.redis.HKeys(WorkersKey).Result()
	if err != nil || len(ids) == 0 {
		return "", errors.New("no workers available")
	}
	return ids[rand.Intn(len(ids))], nil
}

func (m *manager) GetRemoteWorkerStatus(workerID string) (*pb.WorkerStatus, error) {
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

func (m *manager) GetRemoteWorkerQueue(id string) *Queue {
	queue := NewRedisWorkerQueue(m.redis, id)
	return &queue
}

func (m *manager) GetRouter() *Router {
	return &(m.router)
}

func (m *manager) GetWorker() *Worker {
	return &(m.worker)
}

func (m *manager) FirstAvailableWorkerID(action string) (string, error) {
	return m.RandomWorkerId()
}

func (m *manager) UpdateAvailableWorkers() error {
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
		if age := time.Now().Sub(last) ; age > 2 * UpdateFrequency && decode.Id != m.worker.ID() {
			log.Warnf("haven't heard from %s; marking it offline", decode.Id)
			m.MarkOffline(decode.Id)
			continue
		}

		m.statuses[id] = decode
	}

	m.updated = time.Now()
	return nil
}

func (m *manager) Cleanup() {
	m.redis.HDel(WorkersKey, m.worker.ID())
	m.redis.Close()
}

func (m *manager) LookupSignalRoomID(request *pb.SignalRequest) (string, error) {
	switch request.Payload.(type) {
	case *pb.SignalRequest_Join:
		return request.GetJoin().Sid, nil
	}
	return "", errors.New("unable to find room for signal")
}

func (m *manager) GetLocalRoom(roomID string) (*Room, error) {
	room := m.rooms[roomID]
	return &room, nil
}

func (m *manager) GetRemoteRoomExists(roomID string) (bool, error) {
	val, err := m.redis.Exists(pb.KeyRoomToWorker(roomID)).Result()
	return val == 0, err
}

func (m *manager) GetRemoteRoomStatus(roomID string) (*pb.RoomStatus, error) {
	loaded, err := m.LoadStatus(pb.KeyRoomStatus(roomID))
	if err != nil {
		return nil, err
	}
	m.rooms[roomID].UpdateStatus(loaded.GetRoom())
	return m.rooms[roomID].LatestStatus(), nil
}

func (m *manager) ClaimRoomNode(roomID string, nodeID string) (bool, error) {
	return m.redis.SetNX(pb.KeyRoomToWorker(roomID), nodeID, 10*time.Second).Result()
}

// Only on RedisManager
func (m *manager) SaveStatus(key string, status *pb.NoirStatus, expiry time.Duration) error {
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

func (m *manager) LoadStatus(key string) (*pb.NoirStatus, error) {
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

func NewTestManager(driver string, config sfu.Config) Manager {
	if driver != "" {
		rdb := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("TEST_REDIS"),
			Password: "",
			DB:       0,
		})
		return NewRedisManager(config, rdb)
	}
	return nil
}

func (m *manager) WorkerStatus(id string) *pb.WorkerStatus {
	status := m.statuses[id]
	return &status
}

func (m *manager) Start() {
	if m.worker == nil || m.router == nil {
		panic("manager not initialized")
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
			case <- checkin.C:
				m.Checkin()
			case <- update.C:
				m.UpdateAvailableWorkers()
			case <- info.C:
				log.Infof("%s: noirs=%d users=%d rooms=%d", m.worker.ID(), len(m.statuses), len(m.peers), len(m.rooms))
			case <- quit:
				info.Stop()
				update.Stop()
				m.MarkOffline(m.worker.ID())
				return
		}
	}
}

func (m *manager) MarkOffline(workerID string) {
	m.redis.HDel(WorkersKey, workerID)
}