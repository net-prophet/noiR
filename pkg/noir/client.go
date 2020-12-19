package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

// User represent the User model
type User struct {
	sfu.Peer
	userID string
	currentRoomID string
	data   *pb.UserData
}

// NewUser will create an object that represent the Signal interface
func NewUser(provider *sfu.SFU, userID string) User {
	return User{Peer: *sfu.NewPeer(provider), userID: userID, data: &pb.UserData{Id: userID, LastUpdate: timestamppb.Now()}}
}

func (s *User) Join(roomID string, sdp webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	return s.Peer.Join(roomID, sdp)
}

func (s *User) PeerID() string {
	return ""
}

func (s *User) RoomID() string {
	return s.currentRoomID
}

func (s *User) ID() string {
	return s.userID
}

func (s *User) Cleanup() {
	s.Close()
}

func GetUserEndTime(data *pb.UserData) time.Time {
	seconds := data.Options.GetMaxAgeSeconds()
	return data.Created.AsTime().Add(time.Duration(seconds) * time.Second)
}

func GetUserCleanupTime(data *pb.UserData) time.Time {
	seconds := data.Options.GetMaxAgeSeconds() * (1 + data.Options.KeyExpiryFactor)
	return data.Created.AsTime().Add(time.Duration(seconds) * time.Second)
}
