package noir

import (
	"encoding/json"
	"time"

	"github.com/go-redis/redis"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
)

var (
	SESSION_TIMEOUT = 10 * time.Second
)

type noirSFU struct {
	sfu.SFU
	redis *redis.Client
	nodeID string
}

type NoirSFU interface {
	sfu.TransportProvider
	Redis() *redis.Client
	SendToPeer(pid string, value interface{})
	Listen()
}

// NewNoirSFU will create an object that represent the NoirSFU interface
func NewNoirSFU(ion sfu.SFU, client *redis.Client, nodeID string) NoirSFU {
	id := nodeID
	if id == "" {
		id = RandomString(8)
	}
	return &noirSFU{ion, client, id }
}

func (s *noirSFU) Redis() *redis.Client {
	return s.redis
}

func (s *noirSFU) ID() string {
	return s.nodeID
}

// Listen watches the redis topics `sfu/` and `sfu/{id}` for commands
func (s *noirSFU) Listen() {
	r := s.redis
	topic := "sfu/" + s.nodeID
	log.Infof("Listening on topics 'sfu/' and '%s'", topic)

	for {

		message, err := r.BRPop(0, topic, "sfu/").Result()

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}

		var rpc RPCCall

		err = json.Unmarshal([]byte(message[1]), &rpc)

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}
		if rpc.Method == "join" {
			s.handleJoin(message)
		}

		if rpc.Method == "play" {
		}
	}
}

func (s *noirSFU) handleJoin(message []string) {
}

// SessionExists tells you if any other node has the session key locked
func (s *noirSFU) GetSessionNode(sid string) (string, error) {
	r := s.redis
	result, err := r.Get("session/" + sid).Result()
	return result, err
}

// AttemptSessionLock returns true if no other node has a session lock, and locks the session
func (s *noirSFU) AttemptSessionLock(sid string) (string, error) {
	r := s.redis

	sessionNode, err := s.GetSessionNode(sid)
	if sessionNode == "" {
		set, err := r.SetNX("session/"+sid, s.ID(), SESSION_TIMEOUT).Result()

		if err != nil {
			log.Errorf("error locking session: %s", err)
			return "", err
		}
		if set {
			s.RefreshSessionExpiry(sid)
			return s.ID(), nil
		} else {
			return "", nil
		}
	}

	if sessionNode == s.ID() {
		s.RefreshSessionExpiry(sid)
	}
	return sessionNode, err
}

func (s *noirSFU) RefreshSessionExpiry(sid string) {
	r := s.redis
	r.Expire("node/"+s.ID(), SESSION_TIMEOUT)
	r.Expire("session/"+sid, SESSION_TIMEOUT)
}

func (s *noirSFU) SendToPeer(pid string, value interface{}) {
	r := s.Redis()
	if pid != "" {
		r.LPush("peer-recv/"+pid, value)
		r.Expire("peer-recv/"+pid, 10*time.Second)
		r.Publish("peer-recv/", pid)
	} else {
		log.Errorf("cannot push to empty peer")
	}
}
