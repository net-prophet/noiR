package noir

import (
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/go-redis/redis"
	sfu "github.com/pion/ion-sfu/pkg/sfu"
)

var (
	SESSION_TIMEOUT = 10 * time.Second
)

type noirSFU struct {
	webrtc   sfu.WebRTCTransportConfig
	router   sfu.RouterConfig
	mu       sync.RWMutex
	sessions map[string]*sfu.Session
	redis *redis.Client
	nodeID string
}

type NoirSFU interface {
	sfu.SessionProvider
}

// NewNoirSFU will create an object that represent the NoirSFU interface
func NewNoirSFU(c sfu.Config) NoirSFU {
	rand.Seed(time.Now().UnixNano())
	id := RandomString(8)
	// Init ballast
	ballast := make([]byte, c.SFU.Ballast*1024*1024)

	w := sfu.NewWebRTCTransportConfig(c)

	runtime.KeepAlive(ballast)

	return &noirSFU{
		webrtc: w,
		sessions: make(map[string]*sfu.Session),
		nodeID: id }

}

// NewSession creates a new session instance
func (s *noirSFU) newSession(id string) *sfu.Session {
	session := sfu.NewSession(id)
	session.OnClose(func() {
		s.mu.Lock()
		delete(s.sessions, id)
		s.mu.Unlock()
	})

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	return session
}

// GetSession by id
func (s *noirSFU) getSession(id string) *sfu.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

func (s *noirSFU) GetSession(sid string) (*sfu.Session, sfu.WebRTCTransportConfig) {
	session := s.getSession(sid)
	if session == nil {
		session = s.newSession(sid)
	}
	return session, s.webrtc
}
