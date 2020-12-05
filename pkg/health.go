package pkg

import (
	"encoding/json"
	"errors"
	"github.com/go-redis/redis"
	"github.com/gogo/protobuf/proto"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
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
	UpdateAvailableWorkers()
	WorkerCount() (int64, error)
	GetWorkerQueue(string) Queue
	Cleanup()
}

type redisHealth struct {
	redis   *redis.Client
	workers map[string]pb.WorkerStatus
	updated time.Time
}

func NewRedisHealth(redis *redis.Client) Health {
	return &redisHealth{redis: redis, workers: nil, updated: time.Now().Add(-1 * UpdateFrequency)}
}

func (h *redisHealth) Checkin(worker *Worker) error {
	id := (*worker).ID()
	value, err := proto.Marshal(&pb.WorkerStatus{Id: id})
	if err != nil {
		return err
	}
	return h.redis.HSet(WorkersKey, id, value).Err()
}

func (h *redisHealth) RandomWorkerId() string {
	ids, err := h.redis.HKeys(WorkersKey).Result()
	if err != nil || len(ids) == 0 {
		return ""
	}
	return ids[rand.Intn(len(ids))]
}

func (h *redisHealth) GetWorkerQueue(id string) Queue {
	return NewRedisWorkerQueue(h.redis, id)
}

func (h *redisHealth) FirstAvailableWorkerID(action string) (string, error) {
	if time.Now().Sub(h.updated) > UpdateFrequency {
		h.UpdateAvailableWorkers()
	}
	id := h.RandomWorkerId()
	if id == "" {
		return "", errors.New("no workers available")
	}
	return id, nil
}

func (h *redisHealth) UpdateAvailableWorkers() {
	ids, err := h.redis.HKeys(WorkersKey).Result()
	if err != nil {
		log.Errorf("error getting workers from redis %s", err)
		panic("aborting")
	}

	h.workers = map[string]pb.WorkerStatus{}
	for _, id := range ids {
		status, err := h.redis.HGet(WorkersKey, id).Result()
		if err != nil {
			log.Errorf("error getting worker status %s", err)
			panic("aborting")
		}

		if err := json.Unmarshal([]byte(status), h.workers[id]); err != nil {
			log.Errorf("error decoding worker status, ignoring worker %s", err)
			delete(h.workers, id)
			continue
		}
	}

	h.updated = time.Now()
}

func (h *redisHealth) WorkerCount() (int64, error) {
	return h.redis.HLen(WorkersKey).Result()
}

func (h *redisHealth) Cleanup() {
	h.redis.Del(WorkersKey)
}


// Local Memory Health

func NewTestHealth() Health {
	if os.Getenv("TEST_REDIS") != "" {
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

func (h *locmemHealth) GetWorkerQueue(id string) Queue {
	return (*h.workers[id].worker).GetQueue()
}

func (h *locmemHealth) Checkin(worker *Worker) error {
	h.workers[(*worker).ID()] = locmemWorker{worker: worker, status: pb.WorkerStatus{}}
	return nil
}

func (h *locmemHealth) WorkerCount() (int64, error) {
	return int64(len(h.workers)), nil
}

func (h *locmemHealth) UpdateAvailableWorkers() {

}

func (h *locmemHealth) Cleanup() {

}
