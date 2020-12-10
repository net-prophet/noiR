package signal

import (
	"context"
	"encoding/json"
	"fmt"
	noir "github.com/net-prophet/noir/pkg/noir"
	"github.com/net-prophet/noir/pkg/proto"
	strings "strings"

	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
)

type clientJSONRPCBridge struct {
	pid string
	manager *noir.Manager
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

	switch req.Method {

	case "join":

		var join noir.Join
		err := json.Unmarshal(*req.Params, &join)

		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		command := &proto.NoirRequest{
			Command: &proto.NoirRequest_Signal{
				Signal: &proto.SignalRequest{
					// SignalRequest.id should be called pid but we are ion-sfu compatible
					Id: s.pid,
					Payload: &proto.SignalRequest_Join{&proto.JoinRequest{
						Sid:         join.Sid,
						Description: []byte(join.Offer.SDP),
					},
					},
				},
			}}
		router := (*s.manager).GetRouter()
		queue := (*router).GetQueue()

		noir.EnqueueRequest(*queue, command)

		go s.Listen(ctx, conn, req)

	case "offer":
		var negotiation noir.Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		json.Marshal(noir.RPCCall{requestId, "offer", negotiation})
		//r.LPush("peer-send/"+s.PeerID(), message)

	case "answer":
		var negotiation noir.Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}
		json.Marshal(noir.Notify{"answer", negotiation, "2.0"})
		//r.LPush("peer-send/"+s.PeerID(), message)

	case "trickle":
		var trickle noir.Trickle
		err := json.Unmarshal(*req.Params, &trickle)
		if err != nil {
			log.Errorf("connect: error parsing candidate: %v", err)
			replyError(err)
			break
		}
		if _, err := json.Marshal(noir.Notify{"trickle", trickle, "2.0"}); err != nil {
			log.Errorf("error parsing message")
		} else {
			//r.LPush("peer-send/"+s.PeerID(), message)
		}
	}

	//r.Expire("peer-send/"+s.PeerID(), 10*time.Second)
}

func (s *clientJSONRPCBridge) Close() {
	s.manager.CloseClient(s.pid)
}

func (s *clientJSONRPCBridge) Listen(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	/*
	r := s.Redis()
	topic := "peer-recv/" + s.PeerID()
	log.Infof("watch[%s] started", topic)

	for {

		message, err := r.BRPop(0, topic).Result()

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}
		if message[1] == "kill" {
			s.Cleanup()
			return
		}

		var rpc noir.ResultOrNotify

		if err := json.Unmarshal([]byte(message[1]), &rpc); err != nil {
			log.Errorf("failed to unmarshal rpc %s", message[1])
			continue
		}

		log.Infof("RPC: %s:%s/%s", rpc.ID, rpc.ResultType, rpc.Method)

		if rpc.ID != "" {
			conn.Reply(ctx, jsonrpc2.ID{Num: 0, Str: rpc.ID, IsString: true}, rpc.Result)
			continue
		}

		packed, err := json.Marshal(rpc.Params)
		if err != nil {
			log.Errorf("failed to marshal params %s", rpc.Params)
			continue
		}

		if rpc.ResultType == "answer" {
			var answer webrtc.SessionDescription
			if err := json.Unmarshal([]byte(message[1]), &answer); err != nil {
				log.Errorf("failed to unmarshal answer %s %s", err, message[1])
				continue
			}
			if err := conn.Reply(ctx, req.ID, answer); err != nil {
				log.Errorf("failed to send reply %s", err)
				continue
			}
		}

		if rpc.Method == "offer" {
			var offer webrtc.SessionDescription
			if err := json.Unmarshal(packed, &offer); err != nil {
				log.Errorf("failed to unmarshal answer %s %s", err, packed)
				continue
			}
			if err := conn.Notify(ctx, "offer", offer); err != nil {
				log.Errorf("error sending offer %s", err)
				continue
			}
		}

		if rpc.Method == "trickle" {
			var trickle noir.Trickle
			if err := json.Unmarshal(packed, &trickle); err != nil {
				log.Errorf("failed to unmarshal trickle %s %s", err, packed)
				continue
			}

			if err := conn.Notify(ctx, "trickle", trickle); err != nil {
				log.Errorf("error sending ice candidate %s", err)
				continue
			}
		}

	}
	 */
}
