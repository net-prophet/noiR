package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	"github.com/pion/ion-sfu/pkg/sfu"
	"time"
)

type Room struct {
	id string
	lastUpdated time.Time
	status pb.RoomStatus
	session *sfu.Session
}

func NewRoom(roomID string, workerID string, session *sfu.Session) Room {
	status := pb.RoomStatus{LastUpdate: time.Now().String(), WorkerID: workerID}
	return Room{id: roomID, lastUpdated: time.Now(), status: status, session: session}
}

func (r *Room) LatestStatus() *pb.RoomStatus {
	return &r.status
}
func (r *Room) UpdateStatus(remote *pb.RoomStatus) {
	r.status = *remote
}

func (r *Room) Session() *sfu.Session {
	return r.session
}
