package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	"time"
)

type Room interface {
	LatestStatus() *pb.RoomStatus
	UpdateStatus(status *pb.RoomStatus)
}

type room struct {
	id string
	lastUpdated time.Time
	status pb.RoomStatus
}

func NewRoom(roomID string, workerID string) Room {
	status := pb.RoomStatus{LastUpdate: time.Now().String(), WorkerID: workerID}
	return &room{id: roomID, lastUpdated: time.Now(), status: status}
}

func (r *room) LatestStatus() *pb.RoomStatus {
	return &r.status
}
func (r *room) UpdateStatus(remote *pb.RoomStatus) {
	r.status = *remote
}
