package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	"github.com/pion/ion-sfu/pkg/sfu"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type Room struct {
	id          string
	lastUpdated time.Time
	data        pb.RoomData
	session     *sfu.Session
}

func NewRoom(roomID string, workerID string, session *sfu.Session) Room {
	return Room{id: roomID, lastUpdated: time.Now(), data: pb.RoomData{LastUpdate: timestamppb.Now(), WorkerID: workerID}, session: session}
}

func (r *Room) LatestData() *pb.RoomData {
	return &r.data
}
func (r *Room) UpdateData(remote *pb.RoomData) {
	r.data = *remote
}

func (r *Room) Session() *sfu.Session {
	return r.session
}
