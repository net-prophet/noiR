package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/ivfreader"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/go-redis/redis"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"
)

var (
	SESSION_TIMEOUT = 10 * time.Second
)


type noirSFU struct {
	sfu.SFU
	transport *redis.Client
	nodeID    string
}

type NoirSFU interface {
	sfu.TransportProvider
	Redis() *redis.Client
	SendToPeer(pid string, value interface{})
	Listen()
}

// NewNoirSFU will create an object that represent the NoirSFU interface
func NewNoirSFU(ion sfu.SFU, client *redis.Client, nodeID string) NoirSFU {
	return &noirSFU{ion, client, nodeID}
}

func (s *noirSFU) Redis() *redis.Client {
	return s.transport
}

func (s *noirSFU) ID() string {
	return s.nodeID
}

// Listen watches the redis topics `sfu/` and `sfu/{id}` for commands
func (s *noirSFU) Listen() {
	r := s.transport
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
			go s.handlePlay(message)
		}
	}
}

func (s *noirSFU) handleJoin(message []string) {
	r := s.transport
	var rpcJoin RPCJoin

	err := json.Unmarshal([]byte(message[1]), &rpcJoin)

	if err != nil {
		log.Errorf("unrecognized %s", message)
		return
	}

	locked_by, err := s.AttemptSessionLock(rpcJoin.Params.Sid)

	if err != nil {
		log.Errorf("error aquiring session lock %s", err)
	}
	if locked_by != s.nodeID {
		log.Infof("another node has session %s, forwarding join to sfu/%s", rpcJoin.Params.Sid, locked_by)
		r.LPush("sfu/"+locked_by, message[1])

		return // another node aquired the session lock
	}

	log.Infof("joining room %s", rpcJoin.Params.Sid)

	p := *sfu.NewPeer(&s.SFU)

	sig := NewNoirPeer(s, rpcJoin.Params.Pid, rpcJoin.Params.Sid)

	p.OnOffer = func(offer *webrtc.SessionDescription) {
		message, _ := json.Marshal(Notify{"offer", offer, "2.0"})
		s.SendToPeer(rpcJoin.Params.Pid, message)
	}

	p.OnIceCandidate = func(candidate *webrtc.ICECandidateInit, target int) {
		message, _ := json.Marshal(Notify{"trickle", Trickle{*candidate, target}, "2.0"})
		s.SendToPeer(rpcJoin.Params.Pid, message)
	}

	answer, err := p.Join(rpcJoin.Params.Sid, rpcJoin.Params.Offer)

	if err != nil {
		log.Errorf("error joining %s %s", err)
	} else {
		log.Infof("peer %s joined session %s", rpcJoin.Params.Pid, rpcJoin.Params.Sid)
	}

	reply, err := json.Marshal(Result{rpcJoin.ID, answer, "2.0"})

	// peer-recv/{id} channel is for peer to recieve messages
	s.SendToPeer(rpcJoin.Params.Pid, reply)

	go sig.Listen(&p)
}

func (s *noirSFU) handlePlay(message []string) {
	log.Infof("lets play %s", message[1])
	r := s.Redis()

	var rpcPlay RPCPlay

	err := json.Unmarshal([]byte(message[1]), &rpcPlay)

	if err != nil {
		log.Errorf("unrecognized %s", message)
		return
	}

	play := rpcPlay.Params

	mediaEngine := webrtc.MediaEngine{}
	mediaEngine.RegisterDefaultCodecs()

	// Create a new RTCPeerConnection
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	// Create a video track
	videoTrack, addTrackErr := peerConnection.NewTrack(getPayloadType(mediaEngine, webrtc.RTPCodecTypeVideo, "VP8"), rand.Uint32(), "video", "pion")
	if addTrackErr != nil {
		panic(addTrackErr)
	}
	if _, addTrackErr = peerConnection.AddTrack(videoTrack); err != nil {
		panic(addTrackErr)
	}

	go func() {
		// Open a IVF file and start reading using our IVFReader
		file, ivfErr := os.Open(play.Filename)
		if ivfErr != nil {
			panic(ivfErr)
		}

		ivf, header, ivfErr := ivfreader.NewWith(file)
		if ivfErr != nil {
			panic(ivfErr)
		}

		// Wait for connection established
		<-iceConnectedCtx.Done()

		// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
		// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
		sleepTime := time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000)
		for {
			frame, _, ivfErr := ivf.ParseNextFrame()
			if ivfErr == io.EOF {
				if play.Repeat {

					file, ivfErr = os.Open(play.Filename)
					if ivfErr != nil {
						panic(ivfErr)
					}

					ivf, header, ivfErr = ivfreader.NewWith(file)
					if ivfErr != nil {
						panic(ivfErr)
					}

				} else {
					fmt.Printf("All video frames parsed and sent")
					peerConnection.Close()
					return
				}
			}

			if ivfErr != nil {
				panic(ivfErr)
			}

			time.Sleep(sleepTime)
			if ivfErr = videoTrack.WriteSample(media.Sample{Data: frame, Samples: 90000}); ivfErr != nil {
				panic(ivfErr)
			}
		}
	}()

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
	})

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Infof("Error creating offer: %v", err)
	}

	if err = peerConnection.SetLocalDescription(offer); err != nil {
		log.Infof("Error setting local description: %v", err)
	}

	<-gatherComplete

	if err != nil {
		log.Infof("Error publishing stream: %v", err)
	}

	pid := play.Pid

	join, err := json.Marshal(RPCCall{
		ID:     pid,
		Method: "join",
		Params: Join{
			Sid: play.Sid,
			Offer: webrtc.SessionDescription{
				Type: offer.Type,
				SDP:  peerConnection.LocalDescription().SDP,
			},
			Pid: pid,
		},
	})

	if err != nil {
		log.Infof("marshaling: %v", err)
	}

	r.LPush("sfu/", join)

	if err != nil {
		log.Infof("Error sending publish request: %v", err)
	}
	topic := "peer-recv/" + pid

	log.Infof("playing %s as %s", play.Filename, pid)

	message, err = r.BRPop(0, topic).Result()
	var result Result
	if json.Unmarshal([]byte(message[1]), &result) != nil {
		log.Errorf("error parsing rpc", message[1], result)
	}

	log.Infof("Got answer from sfu. Starting streaming for pid %s!\n", pid)

	var reply webrtc.SessionDescription
	resultParams, _ := json.Marshal(result.Result)
	if err = json.Unmarshal(resultParams, &reply) ; err != nil {
		log.Errorf("error unmarshaling %s %s", err, resultParams)
		s.SendToPeer(pid, Notify{"error", err, "2.0"})
		return
	}

	err = peerConnection.SetRemoteDescription(reply)

	if err != nil {
		log.Errorf("error using answer %s %s", err, reply)
		s.SendToPeer(pid, Notify{"error", err, "2.0"})
		return
	}

	for {

		message, err := r.BRPop(0, topic).Result()

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}
		if message[1] == "kill" {
			log.Infof("peer %s disconnected, killing sfu bus", pid)
			peerConnection.Close()
			return
		}

		var rpc ResultOrNotify
		if json.Unmarshal([]byte(message[1]), &rpc) != nil {
			log.Errorf("error parsing rpc", message[1], rpc)
		}

		params, _ := json.Marshal(rpc.Params)

		if rpc.Method == "answer" {
			log.Infof("Got answer from sfu. Starting streaming for pid %s!\n", pid)
			var negotiation Negotiation
			if err := json.Unmarshal(params, &negotiation); err != nil {
				log.Errorf("error parsing answer %s %s", err, params)
			}
			err := peerConnection.SetRemoteDescription(negotiation.Desc)

			if err != nil {
				log.Errorf("error using answer %s %s", err, negotiation.Desc)
				s.SendToPeer(pid, Notify{"error", err, "2.0"})
				continue
			}
		} /*else if rpc.Method == "trickle" {
			var trickle Trickle
			if err := json.Unmarshal(params, &trickle); err != nil {
				log.Errorf("error parsing trickle %s %s", err, params)
			}

			err := peerConnection.Trickle(trickle.Candidate, trickle.Target)

			if err != nil {
				log.Errorf("error using trickle %s %s", err, trickle)
				s.SendToPeer(pid, Notify{"error", err, "2.0"})
				continue
			}
		}*/
	}

}

func getPayloadType(m webrtc.MediaEngine, codecType webrtc.RTPCodecType, codecName string) uint8 {
	for _, codec := range m.GetCodecsByKind(codecType) {
		if codec.Name == codecName {
			return codec.PayloadType
		}
	}
	panic(fmt.Sprintf("Remote peer does not support %s", codecName))
}

// SessionExists tells you if any other node has the session key locked
func (s *noirSFU) GetSessionNode(sid string) (string, error) {
	r := s.transport
	result, err := r.Get("session/" + sid).Result()
	return result, err
}

// AttemptSessionLock returns true if no other node has a session lock, and locks the session
func (s *noirSFU) AttemptSessionLock(sid string) (string, error) {
	r := s.transport

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
	r := s.transport
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
