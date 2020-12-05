package pkg

import (
	"github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"testing"
)

func TestWorkerJoin(t *testing.T) {
	ion := sfu.NewSFU(sfu.Config{Log: log.Config{Level: "trace"}})
	queue := MakeTestQueue("worker/")
	worker := &worker{"test", ion, queue}
	request := &proto.NoirRequest{
		Command: &proto.NoirRequest_Signal{
			Signal: &proto.SignalRequest{
				Id: "123",
				Payload: &proto.SignalRequest_Join{
					Join: &proto.JoinRequest{
						Sid: "test",
						Description: []byte{},
					},
				},
			},
		},
	}
	EnqueueRequest(queue, request)
	worker.HandleNext()
}
