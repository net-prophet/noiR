package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"github.com/pion/webrtc/v3"
	"time"
)

// Client represent the Client model
type Client struct {
	sfu.Peer
	clientID string
	roomID   string
	status   *pb.PeerStatus
}

// NewClient will create an object that represent the Signal interface
func NewClient(provider *sfu.SFU, clientID string, roomID string) Client {
	peer := *sfu.NewPeer(provider)
	return Client{Peer: peer, clientID: clientID, roomID: roomID, status: &pb.PeerStatus{Id: clientID, LastUpdate: time.Now().String()}}
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
