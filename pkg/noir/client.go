package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// User represent the User model
type User struct {
	sfu.Peer
	userID string
	roomID string
	data   *pb.UserData
}

// NewClient will create an object that represent the Signal interface
func NewClient(provider *sfu.SFU, userID string, roomID string) User {
	return User{Peer: *sfu.NewPeer(provider), userID: userID, roomID: roomID, data: &pb.UserData{Id: userID, LastUpdate: timestamppb.Now()}}
}

func (s *User) Join(roomID string, sdp webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	return s.Peer.Join(roomID, sdp)
}

func (s *User) PeerID() string {
	return ""
}

func (s *User) RoomID() string {
	return s.roomID
}

func (s *User) ID() string {
	return s.userID
}

func (s *User) Cleanup() {
	s.Close()
}
