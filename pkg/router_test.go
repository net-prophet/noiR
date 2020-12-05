package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"testing"
)

func TestRouterEnqueueRequest(t *testing.T) {
	for _, driver := range MakeTestDrivers() {

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

		queue := MakeTestQueue(driver, "tests/router/enqueue")
		EnqueueRequest(queue, request)

		got, err := queue.Next()
		if err != nil {
			t.Errorf("error getting count %s", err)
			continue
		}

		want, err := MarshalRequest(request)

		if err != nil {
			t.Errorf("error marshaling request %s", err)
			continue
		}

		if string(got) != string(want) {
			t.Errorf("got %s want %s", got, want)
			continue
		}

		if err := queue.Cleanup(); err != nil {
			t.Errorf("Error cleaning up: %s", err)
			continue
		}
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
			continue
		}
		if got != tt.want {
			t.Errorf("got %s want %s", got, tt.want)
			continue
		}
	}
}

func TestRouterRouteAction(t *testing.T) {
	for _, driver := range MakeTestDrivers() {

		health, _, router, _ := NewTestSetup(driver)

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
			continue
		}

		id, err := health.FirstAvailableWorkerID(next.Action)

		if err != nil {
			t.Errorf("no target for action %s %s", next.Action, err)
			continue
		}

		if id != "test-worker" {
			t.Errorf("unexpected worker id %s expected 'test-worker'", id)
			continue
		}

		err = router.Handle(next)

		if err != nil {
			t.Errorf("error routing request %s", err)
			continue
		}
		worker_queue := *health.GetWorkerQueue("test-worker")
		got, _ := worker_queue.Next()
		want, _ := MarshalRequest(request)

		if string(got) != string(want) {
			t.Errorf("worker got %s queue sent %s", got, want)
			continue
		}
	}
}

func TestRouterHandleNext(t *testing.T) {
	for _, driver := range MakeTestDrivers() {

		health, _, router, _ := NewTestSetup(driver)

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

		err := router.HandleNext()
		if err != nil {
			t.Errorf("error with HandleNext in %s: %s", driver, err)
			continue
		}

		worker_queue := *health.GetWorkerQueue("test-worker")
		got, _ := worker_queue.Next()
		want, _ := MarshalRequest(request)

		if string(got) != string(want) {
			t.Errorf("worker got %s queue from %s sent %s", got, driver, want)
		}
	}
}
