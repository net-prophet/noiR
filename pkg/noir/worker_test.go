package noir

import (
	"github.com/net-prophet/noir/pkg/proto"
	"testing"
)

func TestWorkerJoin(t *testing.T) {
	mgr := NewTestSetup()

	request := &proto.NoirRequest{
		Command: &proto.NoirRequest_Signal{
			Signal: &proto.SignalRequest{
				Id: "123",
				Payload: &proto.SignalRequest_Join{
					Join: &proto.JoinRequest{
						Sid:         "test",
						Description: []byte{},
					},
				},
			},
		},
	}
	worker := *mgr.GetWorker()
	queue := worker.GetQueue()
	EnqueueRequest(*queue, request)
	worker.HandleNext(0)
}
