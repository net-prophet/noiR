package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"testing"
)

func TestHealthCheckin(t *testing.T) {
	for _, driver := range MakeTestDrivers() {

		health, worker, _, _ := NewTestSetup(driver)
		err := health.Checkin(&worker)
		if err != nil {
			t.Errorf("error setting up health %s", err)
		}

		health.UpdateAvailableWorkers()

		count := health.WorkerCount()
		if count != 1 {
			t.Errorf("expected 1 worker, got %d", count)
		}
	}
}

func TestHealthAvailableWorkers(t *testing.T) {
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
		worker_queue := *health.GetWorkerQueue("test-worker")
		got, _ := worker_queue.Next()
		want, _ := MarshalRequest(request)

		if string(got) != string(want) {
			t.Errorf("worker got %s queue sent %s", got, want)
		}
	}
}
