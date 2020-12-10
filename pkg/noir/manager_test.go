package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"testing"
)

func TestHealth_Checkin(t *testing.T) {
		mgr := NewTestSetup()
		worker := mgr.GetWorker()
		err := mgr.Checkin(worker)
		if err != nil {
			t.Errorf("error setting up mgr %s", err)
		}

		mgr.UpdateAvailableWorkers()

		count := mgr.WorkerCount()
		if count != 1 {
			t.Errorf("expected 1 worker, got %d", count)
		}
}

func TestHealth_AvailableWorkers(t *testing.T) {
		mgr := NewTestSetup()
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
		}

		id, err := mgr.FirstAvailableWorkerID(next.Action)

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
		workerQueue := *mgr.GetRemoteWorkerQueue("test-worker")
		got, _ := workerQueue.Next()
		want, _ := MarshalRequest(request)

		if string(got) != string(want) {
			t.Errorf("worker got %s queue sent %s", got, want)
		}
}

func TestHealth_Rooms(t *testing.T) {
		mgr := NewTestSetup()

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

		got, _ := mgr.LookupSignalRoomID(request.GetSignal())
		want := request.GetSignal().GetJoin().Sid
		if got !=  want {
			t.Errorf("unexpected room for join: got %s want %s", got, want)
		}

}
