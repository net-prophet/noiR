package noir

import (
	"errors"
	pb "github.com/net-prophet/noir/pkg/proto"
	"github.com/pion/ion-sfu/pkg/sfu"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type Room struct {
	id          string
	manager     *Manager
	created     time.Time
	lastUpdated time.Time
	data        pb.RoomData
	session     *sfu.Session
}

/*
Rooms:
Create a room by specifying mandatory CLOSE_AFTER and EXPIRY_FACTOR>=0

if CLOSE_AFTER is 2h and EXPIRY_FACTOR is 2 (default), the room "123" will be open for 2h,
and nobody will be able to join the room for 4h after closure (6h total elapsed)

Setting EXPIRY_FACTOR to 0 is not recommended because when you close a room, users might try to immediately
reconnect, creating an empty ion session with insecure access parameters;
a room with many subscribers that had a password would suddenly be open for anyone to publish, etc
*/

func NewRoom(roomID string) Room {
	return Room{
		id:          roomID,
		created:     time.Now(),
		lastUpdated: time.Now(),
		data: pb.RoomData{
			Created: timestamppb.Now(),
			LastUpdate: timestamppb.Now(),
			Options:    &pb.RoomOptions{MaxAgeSeconds: -1, MaxPeers: 2},
		},
	}
}

func (r *Room) LatestData() *pb.RoomData {
	return &r.data
}
func (r *Room) UpdateData(remote *pb.RoomData) {
	r.data = *remote
}

func (r *Room) Bind(session *sfu.Session, manager *Manager) {
	r.session = session
	r.manager = manager
	r.data.WorkerID = manager.id
	r.Save()
}

func (r *Room) Session() *sfu.Session {
	return r.session
}

func (r *Room) Save() error {
	if r.manager != nil {
		return SaveRoomData(r.id, &r.data, r.manager)
	} else {
		return errors.New("manager not binded to room")
	}
}

func SaveRoomData(roomID string, data *pb.RoomData, m *Manager) error {
	expiry := data.Options.MaxAgeSeconds
	if expiry == -1 {
		expiry = 0
	}

	return m.SaveData(pb.KeyRoomData(roomID), &pb.NoirObject{
		Data: &pb.NoirObject_Room{
			Room: data,
		},
	}, time.Duration(expiry * (data.Options.KeyExpiryFactor+1)) * time.Second)
}

func (r *Room) SetOptions(options *pb.RoomOptions) {
	r.data.Options = options
	r.data.LastUpdate = timestamppb.Now()
}

func GetRoomEndTime(data *pb.RoomData) time.Time {
	seconds := data.Options.GetMaxAgeSeconds()
	return data.Created.AsTime().Add(time.Duration(seconds) * time.Second)
}

func GetRoomCleanupTime(data *pb.RoomData) time.Time {
	seconds := data.Options.GetMaxAgeSeconds() * (1 + data.Options.KeyExpiryFactor)
	return data.Created.AsTime().Add(time.Duration(seconds) * time.Second)
}
