package pkg

import (
	"encoding/json"
	"github.com/go-redis/redis"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
)

// NoirPeer represent the signals
type NoirPeer interface {
	Listen(p *sfu.Peer)
	Redis() *redis.Client
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

func (s *noirPeer) Redis() *redis.Client {
	return s.server.Redis()
}

func (s *noirPeer) SessionID() string {
	return s.sid
}

func (s *noirPeer) PeerID() string {
	return s.pid
}

func (s *noirPeer) Cleanup() {
	r := s.Redis()
	r.Del("peer-recv/" + s.pid)
	r.Del("peer-send/" + s.pid)
}

// SFUPeerBus watches on peer-send/{pid} for messages from peer
func (s *noirPeer) Listen(p *sfu.Peer) {
	r := s.Redis()

	topic := "peer-send/" + s.pid

	log.Infof("watching topic %s", topic)

	for {

		message, err := r.BRPop(0, topic).Result()

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}
		if message[1] == "kill" {
			log.Infof("peer %s disconnected, killing sfu bus", s.pid)
			p.Close()
			return
		}

		var rpc ResultOrNotify
		if json.Unmarshal([]byte(message[1]), &rpc) != nil {
			log.Errorf("error parsing rpc", message[1], rpc)
		}

		params, _ := json.Marshal(rpc.Params)

		if rpc.Method == "offer" {
			var negotiation Negotiation
			if err := json.Unmarshal(params, &negotiation); err != nil {
				log.Errorf("error parsing offer %s %s", err, params)
			}
			answer, err := p.Answer(negotiation.Desc)
			if err != nil {
				log.Errorf("error accepting offer %s %s", err, params)
				s.server.SendToPeer(s.pid, Notify{"error", err, "2.0"})
				continue
			}
			message, err := json.Marshal(Result{rpc.ID, answer, "2.0"})
			s.server.SendToPeer(s.pid, message)

		} else if rpc.Method == "answer" {
			var negotiation Negotiation
			if err := json.Unmarshal(params, &negotiation); err != nil {
				log.Errorf("error parsing answer %s %s", err, params)
			}
			err := p.SetRemoteDescription(negotiation.Desc)

			if err != nil {
				log.Errorf("error using answer %s %s", err, negotiation.Desc)
				s.server.SendToPeer(s.pid, Notify{"error", err, "2.0"})
				continue
			}

		} else if rpc.Method == "trickle" {
			var trickle Trickle
			if err := json.Unmarshal(params, &trickle); err != nil {
				log.Errorf("error parsing trickle %s %s", err, params)
			}

			err := p.Trickle(trickle.Candidate, trickle.Target)
			if err != nil {
				log.Errorf("error in candidate %s %s", err, params)
				s.server.SendToPeer(s.pid, Notify{"error", err, "2.0"})
				continue
			}
		} else {
			log.Errorf("Unrecognized rpc %s", rpc)
		}
	}
}
