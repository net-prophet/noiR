package noir

import (
	pb "github.com/net-prophet/noir/pkg/proto"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
	"time"
)

// NoirPeer represent the signals
type NoirPeer interface {
	Listen(p *sfu.Peer)
	ID() string
	SessionID() string
	Cleanup()
}

// noirPeer represent the noirPeer model
type noirPeer struct {
	sfu.Peer
	id    string
	sid    string
	status *pb.PeerStatus
}

// NewNoirPeer will create an object that represent the Signal interface
func NewNoirPeer(ion sfu.SessionProvider, pid string, sid string) NoirPeer {
	peer := sfu.NewPeer(ion)
	return &noirPeer{Peer: *peer, id: pid, sid: sid, status: &pb.PeerStatus{Id: pid, LastUpdate: time.Now().String()}}
}


func (s *noirPeer) SessionID() string {
	return s.sid
}

func (s *noirPeer) ID() string {
	return s.id
}

func (s *noirPeer) Cleanup() {
}

// SFUPeerBus watches on peer-send/{pid} for messages from peer
func (s *noirPeer) Listen(p *sfu.Peer) {

}
