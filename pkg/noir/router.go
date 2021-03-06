package noir

import (
	"errors"
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
)

type Router interface {
	HandleForever()
	GetQueue() *Queue
	HandleNext() error
	Handle(*pb.NoirRequest) error
	NextCommand() (*pb.NoirRequest, error)
}

type router struct {
	queue Queue
	mgr   *Manager
}

func NewRedisRouter(client *redis.Client, mgr *Manager) Router {
	queue := NewRedisQueue(client, pb.KeyRouterTopic(), RouterMaxAge)
	return &router{queue, mgr}
}

func NewRouter(queue Queue, mgr *Manager) Router {
	return &router{queue, mgr}
}
func (r *router) HandleForever() {
	log.Debugf("router starting on topic %s", r.queue.Topic())
	for {
		if err := r.HandleNext(); err != nil {
			log.Errorf("error routing command: %s", err)
		}
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
	p_err := UnmarshalRequest(msg, &request)
	if p_err != nil {
		log.Errorf("message parse error: %s", p_err)
		return nil, p_err
	}
	return &request, nil
}

func (r *router) TargetForSignal(action string, signal *pb.SignalRequest) (string, error) {
	// Signal messages get routed to the worker handling the Room
	roomID, _ := r.mgr.LookupSignalRoomID(signal)

	roomExists, _ := r.mgr.GetRemoteRoomExists(roomID)

	if roomExists == false {
		// Assign the first peer queue a Room to a new worker based on capacity
		log.Infof("no such roomID, routing to random worker")
		target, err := r.mgr.RandomNodeForService("sfu")
		claimed, err := r.mgr.ClaimRoomNode(roomID, target)
		if claimed == true && err == nil {
			return target, nil
		} else {
			return "", err
		}
	} else {
		roomData, err := r.mgr.GetRemoteRoomData(roomID)
		if err != nil {
			log.Errorf("error getting roomID jobData: %s", err)
			return "", err
		}


		if r.mgr.ValidateHealthyNodeID(roomData.NodeID) == nil {
			log.Debugf("room %s is on healthy node %s", roomData.Id, roomData.NodeID)
			return roomData.NodeID, nil
		} else {
			target, err := r.mgr.RandomNodeForService("sfu")
			log.Infof("reassigning %s to node %s", roomID, target)
			if err != nil {
				log.Warnf("")
				return "", err
			}
			claimed, err := r.mgr.ClaimRoomNode(roomData.Id, target)
			if err != nil {
				return "", err
			}
			if claimed == false {
				return "", errors.New("unable to assign roomID")
			}
			log.Infof("roomID %s was assigned to %s which is offline, moved to %s", roomID, roomData.NodeID, target)
			return target, nil
		}
	}
}

func (r *router) Handle(request *pb.NoirRequest) error {
	var routeErr error
	target := ""
	log.Infof("routing: %s", request.Action)
	if request.GetSignal() != nil {
		target, routeErr = r.TargetForSignal(request.Action, request.GetSignal())
	} else {
		// Assign each action to a new worker based on capacity
		target, routeErr = r.mgr.RandomNodeForService("worker")
	}

	if routeErr != nil {
		log.Errorf("error assigning worker: %s", routeErr)
		return routeErr
	}

	if target == "" {
		return errors.New("unknown target for action")
	}

	queue := r.mgr.GetRemoteWorkerQueue(target)

	queueErr := EnqueueRequest(*queue, request)

	if queueErr != nil {
		log.Errorf("error sending to worker: %s", queueErr)
		return queueErr
	}

	log.Debugf("routed %s to %s", request.Action, target)

	return nil
}

func (r *router) GetQueue() *Queue {
	return &r.queue
}
