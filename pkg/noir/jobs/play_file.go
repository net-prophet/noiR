package jobs

import (
	"context"
	"encoding/json"
	"fmt"
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
	Repeat   int    `json:"repeat"`
}

type PlayFileJob struct {
	noir.PeerJob
	options *PlayFileOptions
}

const LabelPlayFile = "PlayFile"

func NewPlayFileJob(manager *noir.Manager, roomID string, filename string, repeat int) *PlayFileJob {
	return &PlayFileJob{
		PeerJob: *noir.NewPeerJob(manager, LabelPlayFile, roomID, noir.RandomString(16)),
		options: &PlayFileOptions{Filename: filename, Repeat: repeat},
	}
}

func NewPlayFileHandler(manager *noir.Manager) noir.JobHandler {
	return func(request *pb.NoirRequest) noir.RunnableJob {
		admin := request.GetAdmin()
		roomAdmin := admin.GetRoomAdmin()
		options := &PlayFileOptions{}
		packed := roomAdmin.GetRoomJob().GetOptions()
		if len(packed) > 0 {
			err := json.Unmarshal(packed, options)
			if err != nil {
				log.Errorf("error unmarshalling job options")
				return nil
			}
		} else {
			options.Filename = "test.video"
			options.Repeat = 0
		}

		return NewPlayFileJob(manager, roomAdmin.GetRoomID(), options.Filename, options.Repeat)
	}
}

func (j *PlayFileJob) Handle() {
	// Assert that we have an audio or video file
	filename := j.options.Filename
	_, err := os.Stat(filename)

	if err != nil {
		j.KillWithError(err)
		return
	}

	err = j.GetMediaEngine().RegisterDefaultCodecs()
	if err != nil {
		j.KillWithError(err)
		return
	}

	peerConnection, err := j.GetPeerConnection()

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

		// A positive repeat will play the file N times, a negative repeat will loop forever
		repeat := j.options.Repeat

		// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
		// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
		sleepTime := time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000)
		for {
			frame, _, ivfErr := ivf.ParseNextFrame()
			if ivfErr == io.EOF {
				if repeat == -1 || repeat > 0 {
					file.Seek(0, 0)
					ivf, header, ivfErr = ivfreader.NewWith(file)
					frame, _, ivfErr = ivf.ParseNextFrame()
					if ivfErr != nil {
						j.KillWithError(ivfErr)
						return
					}
					if repeat > 0 {
						log.Debugf("repeating %s %d more times", filename, repeat)
						repeat = repeat - 1
					}

				} else {
					fmt.Printf("All video frames parsed and sent")
					j.Kill(0)
					return
				}
			}

			if ivfErr != nil {
				j.KillWithError(ivfErr)
				return
			}

			time.Sleep(sleepTime)
			if ivfErr = videoTrack.WriteSample(media.Sample{Data: frame, Duration: time.Second}); ivfErr != nil {
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

	err = j.SendJoin()

	if err != nil {
		log.Errorf("Error publishing stream: %v", err)
		j.KillWithError(err)
	}

	if err != nil {
		log.Errorf("Error sending publish request: %v", err)
		j.KillWithError(err)
	}

	go j.PeerBridge()

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
