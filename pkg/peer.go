package noir

import (
	sfu "github.com/pion/ion-sfu/pkg"
)

// NoirPeer represent the signals
type NoirPeer interface {
	Listen(p *sfu.Peer)
	PeerID() string
	SessionID() string
	Cleanup()
}

// noirPeer represent the noirPeer model
type noirPeer struct {
	server NoirSFU
	pid    string
	sid    string
}

// NewNoirPeer will create an object that represent the Signal interface
func NewNoirPeer(server NoirSFU, pid string, sid string) NoirPeer {
	return &noirPeer{server, pid, sid}
}


func (s *noirPeer) SessionID() string {
	return s.sid
}

func (s *noirPeer) PeerID() string {
	return s.pid
}

func (s *noirPeer) Cleanup() {
}

// SFUPeerBus watches on peer-send/{pid} for messages from peer
func (s *noirPeer) Listen(p *sfu.Peer) {

}
