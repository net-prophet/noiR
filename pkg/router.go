package noir

import (
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
)

type Router interface {
	GetQueue() *Queue
	HandleNext() error
	Handle(*pb.NoirRequest) error
	NextCommand() (*pb.NoirRequest, error)
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
func (r *router) NextCommand() (*pb.NoirRequest, error) {
	msg, popErr := r.queue.BlockUntilNext(0)
	if popErr != nil {
		log.Errorf("queue error %s", popErr)
		return nil, popErr
	}

	var request pb.NoirRequest
	log.Infof("unpacking %s", msg)
	p_err := UnmarshalRequest(msg, &request)
	if p_err != nil {
		log.Errorf("message parse error %s", p_err)
		return nil, p_err
	}
	return &request, nil
}

func (r *router) Handle(request *pb.NoirRequest) error {
	target, healthErr := r.health.FirstAvailableWorkerID(request.Action)

	queue := r.health.GetWorkerQueue(target)

	if healthErr != nil {
		log.Errorf("error assigning %s", healthErr)
		return healthErr
	}

	queueErr := EnqueueRequest(*queue, request)

	if queueErr != nil {
		log.Errorf("error sending to worker %s", queueErr)
		return queueErr
	}

	return nil
}

func (r *router) GetQueue() *Queue {
	return &r.queue
}
