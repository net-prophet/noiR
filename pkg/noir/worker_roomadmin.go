package noir

import pb "github.com/net-prophet/noir/pkg/proto"

func (w *worker) HandleRoomAdmin(request *pb.NoirRequest) error {
	admin := request.GetRoomAdmin()
	if request.Action == "request.room.open" {
		w.manager.OpenRoomFromRequest(admin)
	}
	return nil
}
