package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
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

	queue := MakeTestQueue("tests/router/enqueue")
	EnqueueRequest(queue, request)

	got, err := queue.Next()
	if err != nil {
		t.Errorf("error getting count %s", err)
	}

	want, err := MarshalRequest(request)

	if err != nil {
		t.Errorf("error marshaling request %s", err)
	}

	if string(got) != string(want) {
		t.Errorf("got %s want %s", got, want)
	}

	if err := queue.Cleanup(); err != nil {
		t.Errorf("Error cleaning up: %s", err)
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
		}, "request.signal.join"},
	}

	for _, tt := range tests {
		got, err := ReadAction(tt.request)

		if err != nil {
			t.Errorf("error getting count %s", err)
		}
		if got != tt.want {
			t.Errorf("got %s want %s", got, tt.want)
		}
	}
}

func TestRouterRouteAction(t *testing.T) {
	ion := sfu.NewSFU(sfu.Config{Log: log.Config{Level: "trace"}})
	router_queue := MakeTestQueue("router/route/router")
	router_queue.Cleanup()
	worker_queue := MakeTestQueue("noir/test-worker")
	worker_queue.Cleanup()
	worker := NewWorker("test-worker", ion, worker_queue)
	health := NewTestHealth()
	health.Cleanup()
	err := health.Checkin(&worker)
	if err != nil {
		t.Errorf("error setting up worker %s", err)
	}
	router := MakeRouter(router_queue, health)

	health.UpdateAvailableWorkers()

	count, _ := health.WorkerCount()
	if count != 1 {
		t.Errorf("expected 1 worker, got %d", count)
	}


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

	EnqueueRequest(router_queue, request)
	msg, err := router_queue.Next()
	log.Infof("next %s", msg)

	next := &pb.NoirRequest{}
	err = UnmarshalRequest(msg, next)

	if err != nil {
		t.Errorf("error parsing request %s", err)
	}

	id, err := health.FirstAvailableWorkerID(next.Action)

	if err != nil {
		t.Errorf("no target for action %s %s", next.Action, err)
	}

	if id != "test-worker" {
		t.Errorf("unexpected worker id %s expected 'test-worker'", id)
	}

	err = router.Handle(next)

	if err != nil {
		t.Errorf("error routing request %s", err)
	}
	got, _ := worker_queue.Next()
	want, _ := MarshalRequest(request)

	if string(got) != string(want) {
		t.Errorf("worker got %s queue sent %s", got, want)
	}
}
