package noir

import (
	"github.com/net-prophet/noir/pkg/proto"
	"testing"
)

func TestOpenRoom(t *testing.T) {
	mgr, _ := NewTestSetup()

	request := &proto.NoirRequest{
		Command: &proto.NoirRequest_RoomAdmin{
			RoomAdmin: &proto.RoomAdminRequest{
				RoomID: "test room",
				Payload: &proto.RoomAdminRequest_OpenRoom{
					OpenRoom: &proto.OpenRoomRequest{
						Options: &proto.RoomOptions{
							MaxAgeSeconds:   10,
							JoinPassword:    "secret",
							PublishPassword: "publish",
							MaxPeers:        2,
						},
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
