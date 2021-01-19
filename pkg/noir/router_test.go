package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"testing"
)

func TestRouterEnqueueRequest(t *testing.T) {
	request := &pb.NoirRequest{
		Id: "abc",
		At: "now",
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: "123",
				Payload: &pb.SignalRequest_Join{
					Join: &pb.JoinRequest{
						Sid:         "test",
						Description: nil,
					},
				},
			},
		},
	}

	queue := NewTestQueue("tests/router/enqueue")
	EnqueueRequest(queue, request)

	got, err := queue.Next()
	if err != nil {
		t.Errorf("error getting count %s", err)
		return
	}

	want, err := MarshalRequest(request)

	if err != nil {
		t.Errorf("error marshaling request %s", err)
		return
	}

	if string(got) != string(want) {
		t.Errorf("got %s want %s", got, want)
		return
	}

	if err := queue.Cleanup(); err != nil {
		t.Errorf("Error cleaning up: %s", err)
		return
	}
}

func TestRouterReadAction(t *testing.T) {
	tests := []struct {
		request *pb.NoirRequest
		want    string
	}{
		{&pb.NoirRequest{
			Command: &pb.NoirRequest_Signal{
				Signal: &pb.SignalRequest{
					Id: "123",
					Payload: &pb.SignalRequest_Join{
						Join: &pb.JoinRequest{
							Sid:         "test",
							Description: nil,
						},
					},
				},
			},
		}, "request.servers.join"},
	}

	for _, tt := range tests {
		got, err := ReadAction(tt.request)

		if err != nil {
			t.Errorf("error getting count %s", err)
			continue
		}
		if got != tt.want {
			t.Errorf("got %s want %s", got, tt.want)
			continue
		}
	}
}

func TestRouterRouteAction(t *testing.T) {
	mgr, _ := NewTestSetup()
	router := *mgr.GetRouter()

	request := &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: "123",
				Payload: &pb.SignalRequest_Join{
					Join: &pb.JoinRequest{
						Sid:         "test",
						Description: []byte{},
					},
				},
			},
		},
	}
	router_queue := *router.GetQueue()

	EnqueueRequest(router_queue, request)
	msg, err := router_queue.Next()
	log.Infof("next %s", msg)

	next := &pb.NoirRequest{}
	err = UnmarshalRequest(msg, next)

	if err != nil {
		t.Errorf("error parsing request %s", err)
		return
	}

	id, err := mgr.FirstAvailableWorkerID(next.Action)

	if err != nil {
		t.Errorf("no target for action %s %s", next.Action, err)
		return
	}

	if id != "test-worker" {
		t.Errorf("unexpected worker userID %s expected 'test-worker'", id)
		return
	}

	err = router.Handle(next)

	if err != nil {
		t.Errorf("error routing request: %s", err)
		return
	}

	workerQueue := *mgr.GetRemoteWorkerQueue("test-worker")
	got, _ := workerQueue.Next()
	want, _ := MarshalRequest(request)

	if string(got) != string(want) {
		t.Errorf("worker got %s queue sent %s", got, want)
		return
	}
}

func TestRouterHandleNext(t *testing.T) {
	mgr, _ := NewTestSetup()
	router := *mgr.GetRouter()

	request := &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: "123",
				Payload: &pb.SignalRequest_Join{
					Join: &pb.JoinRequest{
						Sid:         "test",
						Description: []byte{},
					},
				},
			},
		},
	}

	routerQueue := *router.GetQueue()
	EnqueueRequest(routerQueue, request)

	err := router.HandleNext()
	if err != nil {
		t.Errorf("error with HandleNext queue: %s", err)
		return
	}

	workerQueue := *mgr.GetRemoteWorkerQueue("test-worker")
	got, _ := workerQueue.Next()
	want, _ := MarshalRequest(request)

	if string(got) != string(want) {
		t.Errorf("worker got %s queue sent %s", got, want)
	}
}
