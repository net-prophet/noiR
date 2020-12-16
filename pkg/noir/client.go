package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Client represent the Client model
type Client struct {
	sfu.Peer
	clientID string
	roomID   string
	data     *pb.PeerData
}

// NewClient will create an object that represent the Signal interface
func NewClient(provider *sfu.SFU, clientID string, roomID string) Client {
	return Client{Peer: *sfu.NewPeer(provider), clientID: clientID, roomID: roomID, data: &pb.PeerData{Id: clientID, LastUpdate: timestamppb.Now()}}
}

func (s *Client) Join(roomID string, sdp webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	return s.Peer.Join(roomID, sdp)
}

func (s *Client) PeerID() string {
	return ""
}

func (s *Client) SessionID() string {
	return s.roomID
}

func (s *Client) ClientID() string {
	return s.clientID
}

func (s *Client) Cleanup() {
	s.Close()
}

// SFUPeerBus watches on peer-send/{pid} for messages from peer
func (s *Client) Listen(p *sfu.Peer) {

}
