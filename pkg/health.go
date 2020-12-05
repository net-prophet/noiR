package noir

import (
	"errors"
	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"math/rand"
	"os"
	"time"
)

const (
	UpdateFrequency  = 10 * time.Second
	CheckinFrequency = 5 * time.Second
	WorkersKey       = "noir/list_workers"
)

type Health interface {
	FirstAvailableWorkerID(string) (string, error)
	Checkin(worker *Worker) error
	UpdateAvailableWorkers() error
	WorkerCount() int
	GetWorkerQueue(string) *Queue
	Cleanup()
}

type redisHealth struct {
	redis   *redis.Client
	workers map[string]pb.WorkerStatus
	updated time.Time
}

func NewRedisHealth(redis *redis.Client) Health {
	return &redisHealth{redis: redis, workers: map[string]pb.WorkerStatus{}, updated: time.Now().Add(-1 * UpdateFrequency)}
}

func (h *redisHealth) Checkin(worker *Worker) error {
	id := (*worker).ID()
	status := &pb.WorkerStatus{Id: id, At: time.Now().String()}
	value, err := proto.Marshal(status)
	if err != nil {
		return err
	}
	err = h.redis.HSet(WorkersKey, id, value).Err()
	if err != nil {
		return err
	}
	h.workers[id] = *status
	return nil
}

func (h *redisHealth) RandomWorkerId() string {
	ids, err := h.redis.HKeys(WorkersKey).Result()
	if err != nil || len(ids) == 0 {
		return ""
	}
	return ids[rand.Intn(len(ids))]
}

func (h *redisHealth) GetWorkerQueue(id string) *Queue {
	queue := NewRedisWorkerQueue(h.redis, id)
	return &queue
}

func (h *redisHealth) FirstAvailableWorkerID(action string) (string, error) {
	id := h.RandomWorkerId()
	if id == "" {
		return "", errors.New("no workers available")
	}
	return id, nil
}

func (h *redisHealth) UpdateAvailableWorkers() error {
	ids, err := h.redis.HKeys(WorkersKey).Result()
	if err != nil {
		log.Errorf("error getting workers from redis %s", err)
		return err
	}

	for _, id := range ids {
		status, err := h.redis.HGet(WorkersKey, id).Result()
		if err != nil {
			log.Errorf("error getting worker status %s", err)
			return err
		}

		log.Warnf("decoding %s", status)


		var decode pb.WorkerStatus

		if err := proto.Unmarshal([]byte(status), &decode); err != nil {
			log.Errorf("error decoding worker status, ignoring worker %s", err)
			delete(h.workers, id)
			continue
		}
		h.workers[id] = decode
	}

	h.updated = time.Now()
	return nil
}

func (h *redisHealth) WorkerCount() (int) {
	return len(h.workers)
}

func (h *redisHealth) Cleanup() {
	h.redis.Del(WorkersKey)
}


// Local Memory Health

func NewTestHealth(driver string) Health {
	if driver != "" && driver != "locmem" {
		rdb := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("TEST_REDIS"),
			Password: "",
			DB:       0,
		})
		return NewRedisHealth(rdb)
	} else {
		return NewLocalMemoryHealth()
	}
}

type locmemWorker struct {
	worker *Worker
	status pb.WorkerStatus
}

type locmemHealth struct {
	workers map[string]locmemWorker
}

func NewLocalMemoryHealth() Health {
	health := &locmemHealth{
		workers: map[string]locmemWorker{},
	}
	return health
}

func (h *locmemHealth) FirstAvailableWorkerID(string) (string, error) {
	for _, local := range h.workers {
		return (*local.worker).ID(), nil
	}

	return "", errors.New("0 workers")
}

func (h *locmemHealth) GetWorkerQueue(id string) *Queue {
	return (*h.workers[id].worker).GetQueue()
}

func (h *locmemHealth) Checkin(worker *Worker) error {
	h.workers[(*worker).ID()] = locmemWorker{worker: worker, status: pb.WorkerStatus{}}
	return nil
}

func (h *locmemHealth) WorkerCount() int {
	return len(h.workers)
}

func (h *locmemHealth) UpdateAvailableWorkers() error {
	return nil
}

func (h *locmemHealth) Cleanup() {

}


// HEALTH TEST UTILITIES
func NewTestSetup(driver string) (Health, Worker, Router, *sfu.SFU) {
	ion := sfu.NewSFU(sfu.Config{Log: log.Config{Level: "trace"}})
	routerQueue := MakeTestQueue(driver, "router/route/router")
	routerQueue.Cleanup()
	workerQueue := MakeTestQueue(driver, "noir/test-worker")
	workerQueue.Cleanup()
	worker := NewWorker("test-worker", ion, workerQueue)
	health := NewTestHealth(driver)
	health.Cleanup()
	health.Checkin(&worker)
	router := MakeRouter(routerQueue, health)

	return health, worker, router, ion
}
