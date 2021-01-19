package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	"testing"
)

func TestOpenRoom(t *testing.T) {
	mgr, _ := NewTestSetup()

	request := &pb.NoirRequest{
		Command: &pb.NoirRequest_Admin{
			Admin: &pb.AdminRequest{
				Payload: &pb.AdminRequest_RoomAdmin{
					RoomAdmin: &pb.RoomAdminRequest{
						RoomID: "test room",
						Method: &pb.RoomAdminRequest_CreateRoom{
							CreateRoom: &pb.CreateRoomRequest{
								Options: &pb.RoomOptions{
									MaxAgeSeconds:   10,
									JoinPassword:    "secret",
									PublishPassword: "publish",
									MaxPeers:        2,
								},
							},
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
