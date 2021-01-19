package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	noir "github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
	strings "strings"
	"time"
)

type clientJSONRPCBridge struct {
	pid     string
	manager *noir.Manager
}

// Trickle message sent when renegotiating the peer connection
type Trickle struct {
	Target    int                     `json:"target"`
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

func NewClientJSONRPCBridge(pid string, manager *noir.Manager) *clientJSONRPCBridge {
	return &clientJSONRPCBridge{pid: pid, manager: manager}
}

// Handle incoming RPC call events like join, answer, offer and trickle
func (s *clientJSONRPCBridge) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	replyError := func(err error) {
		_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{
			Code:    500,
			Message: fmt.Sprintf("%s", err),
		})
	}
	// TODO: why is this wrapped in quotes?

	requestId := strings.Replace(req.ID.String(), "\"", "", -1)

	router := (*s.manager).GetRouter()
	routerQueue := (*router).GetQueue()

	toPeerQueue := s.manager.GetQueue(pb.KeyTopicToPeer(s.pid))

	log.Debugf("from jsonrpc %s %s", s.pid, req.Method)

	switch req.Method {

	case "join":

		var join noir.Join
		err := json.Unmarshal(*req.Params, &join)

		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		command := &pb.NoirRequest{
			Command: &pb.NoirRequest_Signal{
				Signal: &pb.SignalRequest{
					// SignalRequest.id should be called pid but we are ion-sfu compatible
					Id:        s.pid,
					RequestId: requestId,
					Payload: &pb.SignalRequest_Join{&pb.JoinRequest{
						Sid:         join.Sid,
						Description: []byte(join.Offer.SDP),
					},
					},
				},
			}}

		noir.EnqueueRequest(*routerQueue, command)

		go s.Listen(ctx, conn, req)

	case "offer":
		var negotiation noir.Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		marshaled, _ := json.Marshal(negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		command := &pb.NoirRequest{
			Command: &pb.NoirRequest_Signal{
				Signal: &pb.SignalRequest{
					// SignalRequest.id should be called pid but we are ion-sfu compatible
					Id:        s.pid,
					RequestId: requestId,
					Payload: &pb.SignalRequest_Description{
						Description: marshaled,
					},
				},
			}}

		noir.EnqueueRequest(toPeerQueue, command)

	case "answer":
		var negotiation noir.Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		marshaled, _ := json.Marshal(negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		command := &pb.NoirRequest{
			Command: &pb.NoirRequest_Signal{
				Signal: &pb.SignalRequest{
					// SignalRequest.id should be called pid but we are ion-sfu compatible
					Id:        s.pid,
					RequestId: requestId,
					Payload: &pb.SignalRequest_Description{
						Description: marshaled,
					},
				},
			}}

		noir.EnqueueRequest(toPeerQueue, command)

	case "trickle":
		var trickle Trickle
		err := json.Unmarshal(*req.Params, &trickle)
		if err != nil {
			log.Errorf("connect: error parsing candidate: %v", err)
			replyError(err)
			break
		}
		var target pb.Trickle_Target
		if trickle.Target == 1 {
			target = pb.Trickle_PUBLISHER
		} else {
			target = pb.Trickle_SUBSCRIBER
		}
		marshaled, _ := json.Marshal(trickle.Candidate)

		command := &pb.NoirRequest{
			Command: &pb.NoirRequest_Signal{
				Signal: &pb.SignalRequest{
					// SignalRequest.id should be called pid but we are ion-sfu compatible
					Id:        s.pid,
					RequestId: requestId,
					Payload: &pb.SignalRequest_Trickle{Trickle: &pb.Trickle{
						Target: target,
						Init:   string(marshaled),
					}},
				},
			}}
		noir.EnqueueRequest(toPeerQueue, command)
	}
}

func (s *clientJSONRPCBridge) Close() {
	s.manager.DisconnectUser(s.pid)
}

func (s *clientJSONRPCBridge) Listen(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	//send := s.manager.GetQueue(pb.KeyTopicToPeer(s.pid), noir.PeerPingFrequency)
	recv := s.manager.GetQueue(pb.KeyTopicFromPeer(s.pid))

	log.Infof("peer bridge %s", s.pid)

	for {

		message, err := recv.BlockUntilNext(0)

		if err != nil {
			log.Errorf("message err: %s", err)
			time.Sleep(time.Second)
			continue
		}

		var reply pb.NoirReply

		err = proto.Unmarshal(message, &reply)
		if err != nil {
			log.Errorf("unmarshal err: %s", err)
			continue
		}

		switch reply.Command.(type) {
		case *pb.NoirReply_Signal:
			signal := reply.GetSignal()
			reqID := jsonrpc2.ID{Num: 0, Str: signal.RequestId, IsString: true}
			switch signal.Payload.(type) {
			case *pb.SignalReply_Kill:
				return
			case *pb.SignalReply_Join:
				var answer webrtc.SessionDescription
				json.Unmarshal(signal.GetJoin().Description, &answer)
				conn.Reply(ctx, reqID, answer)
				//log.Debugf("answer %s", answer)
			case *pb.SignalReply_Description:
				var desc webrtc.SessionDescription
				json.Unmarshal(signal.GetDescription(), &desc)
				var method string
				if desc.Type == webrtc.SDPTypeAnswer {
					method = "answer"
				} else {
					method = "offer"
				}
				if signal.RequestId == "" {
					conn.Notify(ctx, method, desc)
					//log.Debugf("notify %s %s", method, desc)
				} else {
					conn.Reply(ctx, reqID, desc)
					//log.Debugf("notify %s %s", method, desc)
				}
			case *pb.SignalReply_Trickle:
				trickle := signal.GetTrickle()
				var candidate webrtc.ICECandidateInit
				json.Unmarshal([]byte(trickle.GetInit()), &candidate)
				conn.Notify(ctx, "trickle", Trickle{
					Target:    int(trickle.Target.Number()),
					Candidate: candidate,
				})
				//log.Debugf("trickle %s", trickle)
			default:
				log.Errorf("unknown servers reply %s", signal)
			}
		default:
			log.Warnf("non-servers reply on client channel %s", &reply)
		}
	}
}
