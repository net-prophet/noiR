package noir

import (
	log "github.com/pion/ion-log"
	"math/rand"
	"runtime"
	"sync"
	"time"

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
	nodeID string
	manager *Manager
}

type NoirSFU interface {
	sfu.SessionProvider
	AttachManager(*Manager)
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
		nodeID: id,
	}

}

func (s *noirSFU) AttachManager(manager *Manager) {
	s.manager = manager
}


func (s *noirSFU) ensureSession(sessionID string) *sfu.Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s, ok := s.sessions[sessionID]; ok {
		return s
	}

	log.Infof("creating session %s", sessionID)
	mgr := *s.manager
	session := sfu.NewSession(sessionID)
	session.OnClose(func() {
		log.Infof("closing session %s", sessionID)
		mgr.CloseRoom(sessionID)
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	})

	mgr.OpenRoom(sessionID, session)
	s.sessions[sessionID] = session
	return session
}

func (s *noirSFU) GetSession(sid string) (*sfu.Session, sfu.WebRTCTransportConfig) {
	if s.manager == nil {
		panic("manager not initialized")
	}
	session := s.ensureSession(sid)
	return session, s.webrtc
}
