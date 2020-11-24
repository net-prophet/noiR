package pkg

import (
	"encoding/json"
	"github.com/go-redis/redis"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
)

// RedisSignal represent the signals
type RedisSignal interface {
	SFUPeerBus(p *sfu.Peer)
	Redis() *redis.Client
	PeerID() string
	SessionID() string
	Cleanup()
}

// redisSignal represent the redisSignal model
type redisSignal struct {
	server RedisSignalServer
	pid    string
	sid    string
}

// NewRedisSignal will create an object that represent the Signal interface
func NewRedisSignal(server RedisSignalServer, pid string, sid string) RedisSignal {
	return &redisSignal{server, pid, sid}
}

func (s *redisSignal) Redis() *redis.Client {
	return s.server.RedisClient()
}

func (s *redisSignal) SessionID() string {
	return s.sid
}

func (s *redisSignal) PeerID() string {
	return s.pid
}

func (s *redisSignal) Cleanup() {
	r := s.server.RedisClient()
	r.Del("peer-recv/" + s.pid)
	r.Del("peer-send/" + s.pid)
}

// SFUPeerBus watches on peer-send/{pid} for messages from peer
func (s *redisSignal) SFUPeerBus(p *sfu.Peer) {
	r := s.server.RedisClient()

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
