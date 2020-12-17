package noir

import (
	"github.com/net-prophet/noir/pkg/proto"
	"github.com/pion/webrtc/v3"
	"testing"
)

const EXAMPLE_EMPTY_SDP = "v=0\r\no=- 8158248220666482328 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\na=msid-semantic: WMS\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=ice-ufrag:1vfB\r\na=ice-pwd:C2aiDmz9WfYOyF93NC36kqaU\r\na=ice-options:trickle\r\na=fingerprint:sha-256 52:37:6D:70:E6:AA:52:08:0F:3F:00:3E:FA:39:3D:F2:BE:7B:20:1D:56:79:B6:E4:5E:59:25:99:3C:D0:14:C1\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\na=max-message-size:262144\r\n"

func TestWorkerJoin(t *testing.T) {
	mgr := NewTestSetup()

	desc := webrtc.SessionDescription{SDP: EXAMPLE_EMPTY_SDP}

	request := &proto.NoirRequest{
		Command: &proto.NoirRequest_Signal{
			Signal: &proto.SignalRequest{
				Id: "123",
				Payload: &proto.SignalRequest_Join{
					Join: &proto.JoinRequest{
						Sid:         "test",
						Description: []byte(desc.SDP),
					},
				},
			},
		},
	}
	worker := *mgr.GetWorker()
	queue := worker.GetQueue()
	EnqueueRequest(*queue, request)
	worker.HandleNext(0)
}
