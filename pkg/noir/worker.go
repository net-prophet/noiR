package noir

import (
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"sync"
	"time"
)

const (
	RouterTopic   = "noir/"
	WebrtcTimeout = 25 * time.Second
	RouterMaxAge  = WebrtcTimeout
)

type Worker interface {
	HandleForever()
	HandleNext(timeout time.Duration) error
	RegisterHandler(name string, handler JobHandler)
	GetQueue() *Queue
	ID() string
}

// worker runs 2 go threads -- Router() takes incoming router messages and loadbalances
// commands across commands queues on nodes while CommandRunner() runs commands on this node's queue
type worker struct {
	id          string
	manager     *Manager
	jobHandlers map[string]JobHandler
	queue       Queue
	mu          sync.RWMutex
}

type JobHandler func(request *pb.NoirRequest) RunnableJob

func NewRedisWorkerQueue(client *redis.Client, id string) Queue {
	return NewRedisQueue(client, pb.KeyWorkerTopic(id), RouterMaxAge)
}

func NewRedisWorker(id string, manager *Manager, client *redis.Client) Worker {
	return &worker{id: id, manager: manager, queue: NewRedisWorkerQueue(client, id), jobHandlers: map[string]JobHandler{}}
}

func NewWorker(id string, manager *Manager, queue Queue) Worker {
	return &worker{id: id, manager: manager, queue: queue, jobHandlers: map[string]JobHandler{}}
}

func (w *worker) HandleForever() {
	log.Debugf("worker starting on topic %s", w.queue.Topic())
	for {
		if err := w.HandleNext(0); err != nil {
			log.Errorf("worker handler error %s", err)
			time.Sleep(1 * time.Second)
		}
	}
}

func (w *worker) HandleNext(timeout time.Duration) error {
	request, err := w.NextCommand(timeout)
	if err != nil {
		return err
	}
	return w.Handle(request)
}

func (w *worker) RegisterHandler(name string, handler JobHandler) {
	log.Debugf("register job handler: %s", name)
	w.jobHandlers[name] = handler
}

func (w *worker) NextCommand(timeout time.Duration) (*pb.NoirRequest, error) {
	msg, popErr := w.queue.BlockUntilNext(timeout)
	if popErr != nil {
		log.Errorf("queue error %s", popErr)
		return nil, popErr
	}

	var request pb.NoirRequest
	p_err := UnmarshalRequest(msg, &request)
	if p_err != nil {
		log.Errorf("message parse error: %s", p_err)
		return nil, p_err
	}
	return &request, nil
}

func (w *worker) ID() string {
	return w.id
}
func (w *worker) GetQueue() *Queue {
	return &w.queue
}
func (w *worker) Handle(request *pb.NoirRequest) error {
	log.Debugf("handle %s", request.Action)
	if request.GetSignal() != nil {
		return w.HandleSignal(request)
	}
	if request.GetAdmin() != nil {
		return w.HandleAdmin(request)
	}
	return nil
}
