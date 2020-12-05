package noir

import (
	"github.com/go-redis/redis"
	sfu "github.com/pion/ion-sfu/pkg"
	"time"
)

const (
	RouterTopic   = "noir/"
	WebrtcTimeout = 25 * time.Second
	RouterMaxAge  = WebrtcTimeout
)

type Worker interface {
	Run()
	HandleNext()
	GetQueue() Queue
	ID() string
}

// worker runs 2 go threads -- Router() takes incoming router messages and loadbalances
// commands across commands queues on nodes while CommandRunner() runs commands on this node's queue
type worker struct {
	id       string
	sfu      *sfu.SFU
	commands Queue
}

func NewRedisWorkerQueue(client *redis.Client, id string) Queue {
	return NewRedisQueue(client, RouterTopic + id, RouterMaxAge)
}

func NewRedisWorker(id string, sfu *sfu.SFU, client *redis.Client) Worker {
	return &worker{id, sfu, NewRedisWorkerQueue(client, id)}
}

func NewWorker(id string, sfu *sfu.SFU, queue Queue) Worker {
	return &worker{id, sfu, queue}
}

func (w *worker) Run() {

	go w.CommandRunner()
	for {

	}
}


func (w *worker) CommandRunner() {

}

func (w *worker) HandleNext() {

}

func (w *worker) ID() string {
	return w.id
}
func (w *worker) GetQueue() Queue {
	return w.commands
}
