package noir

import (
	"encoding/json"
	"github.com/go-redis/redis"
	"github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
)

type Router interface {
	HandleNext() error
	Handle(*proto.NoirRequest) error
	NextCommand() (*proto.NoirRequest, error)
}

type router struct {
	queue  Queue
	health Health
}

func MakeRouterQueue(client *redis.Client) Queue {
	return NewRedisQueue(client, RouterTopic, RouterMaxAge)
}

func MakeRouter(queue Queue, health Health) Router {
	return &router{queue, health}
}
func (r *router) Router() {
	for {
		r.HandleNext()
	}
}

func (r *router) HandleNext() error {
	request, err := r.NextCommand()
	if err != nil {
		return err
	}
	return r.Handle(request)
}
func (r *router) NextCommand() (*proto.NoirRequest, error) {
	msg, msg_err := r.queue.BlockUntilNext(0)
	if msg_err != nil {
		log.Errorf("queue error %s", msg_err)
		return nil, msg_err
	}

	var request proto.NoirRequest
	log.Infof("unpacking %s", msg)
	j_err := json.Unmarshal(msg, &request)
	if j_err != nil {
		log.Errorf("message parse error %s", j_err)
		return nil, j_err
	}
	return &request, nil
}

func (r *router) Handle(request *proto.NoirRequest) error {
	target, healthErr := r.health.FirstAvailableWorkerID(request.Action)

	queue := r.health.GetWorkerQueue(target)

	if healthErr != nil {
		log.Errorf("error assigning %s", healthErr)
		return healthErr
	}

	queueErr := EnqueueRequest(queue, request)

	if queueErr != nil {
		log.Errorf("error sending to worker %s", queueErr)
		return queueErr
	}

	return nil
}
