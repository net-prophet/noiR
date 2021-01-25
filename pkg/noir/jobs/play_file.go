package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/randutil"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/ivfreader"
	"io"
	"os"
	"time"
)

type PlayFileOptions struct {
	Filename string `json:"filename"`
	Repeat bool `json:"repeat"`
}

type PlayFileJob struct {
	noir.PeerJob
	options *PlayFileOptions
}

const handler = "PlayFile"

func NewPlayFileJob(manager *noir.Manager, roomID string, filename string, repeat bool) *PlayFileJob {
	return &PlayFileJob{
		PeerJob: *noir.NewPeerJob(manager, handler, roomID, noir.RandomString(16)),
		options: &PlayFileOptions{Filename: filename, Repeat: repeat},
	}
}

func (j *PlayFileJob) Handle() {
	// Assert that we have an audio or video file
	filename := j.options.Filename
	_, err := os.Stat(filename)
	haveVideoFile := !os.IsNotExist(err)

	if !haveVideoFile {
		panic("Could not find `" + filename + "`")
	}

	// We make our own mediaEngine so we can place the sender's codecs in it.  This because we must use the
	// dynamic media type from the sender in our answer. This is not required if we are the offerer
	mediaEngine := webrtc.MediaEngine{}
	mediaEngine.RegisterDefaultCodecs()

	// Create a new RTCPeerConnection
	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		j.KillWithError(err)
		return
	}
	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: "video/vp8"},
		fmt.Sprintf("video-%d", randutil.NewMathRandomGenerator().Uint32()),
		fmt.Sprintf("video-%d", randutil.NewMathRandomGenerator().Uint32()),
	)

	// Create a video track
	_, err = peerConnection.AddTrack(videoTrack)
	if err != nil {
		j.KillWithError(err)
		return
	}

	go func() {
		defer peerConnection.Close()

		// Open a IVF file and start reading using our IVFReader
		file, ivfErr := os.Open(filename)
		if ivfErr != nil {
			j.KillWithError(ivfErr)
			return
		}

		ivf, header, ivfErr := ivfreader.NewWith(file)
		if ivfErr != nil {
			j.KillWithError(ivfErr)
			return
		}

		log.Infof("waiting for connection...")
		// Wait for connection established
		<-iceConnectedCtx.Done()
		log.Infof("done waiting for connection...")

		// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
		// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
		sleepTime := time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000)
		for {
			frame, _, ivfErr := ivf.ParseNextFrame()
			if ivfErr == io.EOF {
				fmt.Printf("All video frames parsed and sent")
				j.Kill(0)
				return
			}

			if ivfErr != nil {
				j.KillWithError(ivfErr)
				return
			}

			time.Sleep(sleepTime)
			if ivfErr = videoTrack.WriteSample(media.Sample{Data: frame}); ivfErr != nil {
				j.KillWithError(ivfErr)
				return
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
		log.Errorf("Error creating offer: %v", err)
		j.KillWithError(err)
	}

	if err = peerConnection.SetLocalDescription(offer); err != nil {
		log.Errorf("Error setting local description: %v", err)
		j.KillWithError(err)
	}

	<-gatherComplete

	if err != nil {
		log.Errorf("Error publishing stream: %v", err)
		j.KillWithError(err)
	}

	router := j.GetManager().GetRouter()
	queue := (*router).GetQueue()
	peerID := j.GetPeerData().PeerID
	roomID := j.GetPeerData().RoomID
	log.Infof("joining room=%s peer=%s", roomID, peerID)
	err = noir.EnqueueRequest(*queue, &pb.NoirRequest{
		Command: &pb.NoirRequest_Signal{
			Signal: &pb.SignalRequest{
				Id: peerID,
				Payload: &pb.SignalRequest_Join{
					Join: &pb.JoinRequest{
						Sid:         roomID,
						Description: []byte(peerConnection.LocalDescription().SDP),
					},
				},
			},
		},
	})

	if err != nil {
		log.Errorf("Error sending publish request: %v", err)
		j.KillWithError(err)
	}

	peerQueue := j.GetPeerQueue()

	for {
		message, err := peerQueue.BlockUntilNext(noir.QueueMessageTimeout)

		if err == io.EOF {
			// WebRTC Transport closed
			fmt.Println("WebRTC Transport Closed")
			j.Kill(0)
			return
		}

		if err != nil {
			continue
		}

		var reply pb.NoirReply

		err = proto.Unmarshal(message, &reply)

		if signal, ok := reply.Command.(*pb.NoirReply_Signal); ok {
			if join := signal.Signal.GetJoin() ; join != nil {
				log.Debugf("playfile connected %s => %s!\n", peerID)
				// Set the remote SessionDescription
				desc := &webrtc.SessionDescription{}
				json.Unmarshal(join.Description, desc)
				if err = peerConnection.SetRemoteDescription(*desc); err != nil {
					j.KillWithError(err)
					return
				}
			}
			if signal.Signal.GetKill() {
				log.Debugf("signal killed room=%s peer=%s", roomID, peerID)
				j.Kill(0)
				return
			}
		}
	}
}

// Search for Codec PayloadType
//
// Since we are answering we need to match the remote PayloadType
/*
func getPayloadType(m webrtc.MediaEngine, codecType webrtc.RTPCodecType, codecName string) uint8 {
	for _, codec := range m.GetCodecsByKind(codecType) {
		if codec.Name == codecName {
			return codec.PayloadType
		}
	}
	panic(fmt.Sprintf("Remote peer does not support %s", codecName))
}
*/