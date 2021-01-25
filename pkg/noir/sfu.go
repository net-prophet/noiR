package noir

import (
	"github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	//"github.com/pion/ion-sfu/pkg/middlewares/datachannel"
	"math/rand"
	"sync"
	"time"

	sfu "github.com/pion/ion-sfu/pkg/sfu"
)

var (
	SESSION_TIMEOUT = 10 * time.Second
)

type noirSFU struct {
	sfu.SFU
	webrtc       sfu.WebRTCTransportConfig
	router       sfu.RouterConfig
	mu           sync.RWMutex
	sessions     map[string]*sfu.Session
	//datachannels []*sfu.Datachannel
	nodeID       string
	manager      *Manager
}

type NoirSFU interface {
	sfu.SessionProvider
	AttachManager(*Manager)
}

// NewNoirSFU will create an object that represent the NoirSFU interface
func NewNoirSFU(c Config) NoirSFU {
	rand.Seed(time.Now().UnixNano())
	id := RandomString(8)

	w := sfu.NewWebRTCTransportConfig(c.Ion)

	ion := sfu.NewSFU(c.Ion)
	//dc := ion.NewDatachannel(sfu.APIChannelLabel)
	//dc.Use(datachannel.SubscriberAPI)

	return &noirSFU{
		SFU:      *ion,
		webrtc:   w,
		sessions: make(map[string]*sfu.Session),
		nodeID:   id,
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

	mgr.redis.Del(proto.KeyRoomUsers(sessionID))

	session.OnClose(func() {
		log.Infof("closing session %s", sessionID)
		room, err := mgr.GetRemoteRoomData(sessionID)
		defer mgr.UpdateRoomScore(sessionID)

		if room != nil && err == nil {
			if room.Options.MaxAgeSeconds == -1 {
				log.Infof("closing empty room %s with expiry=-1", sessionID)
				mgr.CloseRoom(sessionID)
				if room.Options.Debug == 0 {
					mgr.redis.Del(proto.KeyRoomData(sessionID))
				}
			}
		}

		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	})

	room := NewRoom(sessionID)
	data, _ := mgr.GetRemoteRoomData(sessionID)
	room.data = *data

	mgr.BindRoomSession(room, session)

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
