package jobs

import (
	"bufio"
	"encoding/json"
	"github.com/at-wat/ebml-go/webm"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"io"
	"os/exec"
	"time"
)

type RTMPSendOptions struct {
	Destination   string `json:"destination"`
	SourceUserID  string `json:"source_user_id"`
	SourceTrackID string `json:"source_track_id"`
}

type RTMPSendJob struct {
	noir.PeerJob
	options *RTMPSendOptions
	audioWriter, videoWriter       webm.BlockWriteCloser
	audioBuilder, videoBuilder     *samplebuilder.SampleBuilder
	audioTimestamp, videoTimestamp time.Duration
}

const LabelRTMPSend = "RTMPSend"

func NewRTMPSendJob(manager *noir.Manager, roomID string, destination string, sourceUserID string, sourceTrackID string) *RTMPSendJob {
	return &RTMPSendJob{
		PeerJob: *noir.NewPeerJob(manager, "RTMPSend", roomID, noir.RandomString(16)),
		options: &RTMPSendOptions{Destination: destination, SourceUserID: sourceUserID, SourceTrackID: sourceTrackID},
	}
}

func NewRTMPSendHandler(manager *noir.Manager) noir.JobHandler {
	return func(request *pb.NoirRequest) noir.RunnableJob {
		admin := request.GetAdmin()
		roomAdmin := admin.GetRoomAdmin()
		options := &RTMPSendOptions{}
		packed := roomAdmin.GetRoomJob().GetOptions()
		if len(packed) > 0 {
			err := json.Unmarshal(packed, options)
			if err != nil {
				log.Errorf("error unmarshalling job options")
				return nil
			}
		} else {
			options.Destination = ""
			options.SourceTrackID = ""
			options.SourceUserID = ""
		}
		return NewRTMPSendJob(manager, roomAdmin.GetRoomID(), options.Destination, options.SourceUserID, options.SourceTrackID)
	}
}

func (j *RTMPSendJob) Handle() {
	log.Infof("started rtmp")
	m := webrtc.MediaEngine{}

	// Setup the codecs you want to use.
	// Only support VP8 and OPUS, this makes our WebM muxer code simpler
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "video/VP8", ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		j.KillWithError(err)
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/opus", ClockRate: 48000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		j.KillWithError(err)
	}

	j.audioBuilder = samplebuilder.New(10, &codecs.OpusPacket{}, 48000)
	j.videoBuilder = samplebuilder.New(10, &codecs.VP8Packet{}, 90000)

	j.SetMediaEngine(&m)

	peerConnection, err := j.GetPeerConnection()
	if err != nil {
		log.Errorf("error getting peer connection", err)
	}

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		j.KillWithError(err)
	} else if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		j.KillWithError(err)
	}

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		j.KillWithError(err)
	}
	if err = peerConnection.SetLocalDescription(offer); err != nil {
		j.KillWithError(err)
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Infof("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().RTPCodecCapability.MimeType)

		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				rtcpSendErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if rtcpSendErr != nil {
					log.Errorf("err: %s ", rtcpSendErr)
				}
			}
		}()
		for {
			// Read RTP packets being sent to Pion
			rtp, readErr := track.ReadRTP()
			if readErr != nil {
				if readErr == io.EOF {
					return
				}
				j.KillWithError(readErr)
			}
			switch track.Kind() {
			case webrtc.RTPCodecTypeAudio:
				j.pushOpus(rtp)
			case webrtc.RTPCodecTypeVideo:
				j.pushVP8(rtp)
			}
		}
	})

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Infof("Connection State has changed %s \n", connectionState.String())
	})

	j.SendJoin()

	go j.PeerBridge()
}

// Parse Opus audio and Write to WebM
func (j *RTMPSendJob) pushOpus(rtpPacket *rtp.Packet) {
	j.audioBuilder.Push(rtpPacket)

	for {
		sample := j.audioBuilder.Pop()
		if sample == nil {
			return
		}
		if j.audioWriter != nil {
			j.audioTimestamp += sample.Duration
			if _, err := j.audioWriter.Write(true, int64(j.audioTimestamp/time.Millisecond), sample.Data); err != nil {
				j.KillWithError(err)
			}
		}
	}
}

// Parse VP8 video and Write to WebM
func (j *RTMPSendJob) pushVP8(rtpPacket *rtp.Packet) {
	j.videoBuilder.Push(rtpPacket)

	for {
		sample := j.videoBuilder.Pop()
		if sample == nil {
			return
		}
		// Read VP8 header.
		videoKeyframe := (sample.Data[0]&0x1 == 0)
		if videoKeyframe {
			// Keyframe has frame information.
			raw := uint(sample.Data[6]) | uint(sample.Data[7])<<8 | uint(sample.Data[8])<<16 | uint(sample.Data[9])<<24
			width := int(raw & 0x3FFF)
			height := int((raw >> 16) & 0x3FFF)

			if j.videoWriter == nil || j.audioWriter == nil {
				// Initialize WebM saver using received frame size.
				j.startFFmpeg(width, height)
			}
		}
		if j.videoWriter != nil {
			j.videoTimestamp += sample.Duration
			if _, err := j.videoWriter.Write(videoKeyframe, int64(j.audioTimestamp/time.Millisecond), sample.Data); err != nil {
				j.KillWithError(err)
			}
		}
	}
}


func (j *RTMPSendJob) startFFmpeg(width, height int) {
	// Create a ffmpeg process that consumes MKV via stdin, and broadcasts out to Twitch
	log.Infof("STARTING FFMPEG")
	ffmpeg := exec.Command("ffmpeg", "-re", "-i", "pipe:0", "-c:v", "libx264", "-preset", "veryfast", "-maxrate", "3000k", "-bufsize", "6000k", "-pix_fmt", "yuv420p", "-g", "50", "-c:a", "aac", "-b:a", "160k", "-ac", "2", "-ar", "44100", "-f", "flv", j.options.Destination) //nolint
	ffmpegIn, _ := ffmpeg.StdinPipe()
	ffmpegOut, _ := ffmpeg.StderrPipe()
	if err := ffmpeg.Start(); err != nil {
		j.KillWithError(err)
	}

	go func() {
		scanner := bufio.NewScanner(ffmpegOut)
		for scanner.Scan() {
			log.Debugf(scanner.Text())
		}
	}()

	ws, err := webm.NewSimpleBlockWriter(ffmpegIn,
		[]webm.TrackEntry{
			{
				Name:            "Audio",
				TrackNumber:     1,
				TrackUID:        12345,
				CodecID:         "A_OPUS",
				TrackType:       2,
				DefaultDuration: 20000000,
				Audio: &webm.Audio{
					SamplingFrequency: 48000.0,
					Channels:          2,
				},
			}, {
				Name:            "Video",
				TrackNumber:     2,
				TrackUID:        67890,
				CodecID:         "V_VP8",
				TrackType:       1,
				DefaultDuration: 33333333,
				Video: &webm.Video{
					PixelWidth:  uint64(width),
					PixelHeight: uint64(height),
				},
			},
		})
	if err != nil {
		j.KillWithError(err)
	}

	log.Infof("RTMPSend job has started with video width=%d, height=%d\n", width, height)
	j.audioWriter = ws[0]
	j.videoWriter = ws[1]
}
