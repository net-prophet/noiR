package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
	"strings"
)

type adminJSONRPC struct {
	sfu      *noir.NoirSFU
	manager  *noir.Manager
	clientID string
}

func NewAdminJSONRPC(s *noir.NoirSFU, manager *noir.Manager) *adminJSONRPC {
	return &adminJSONRPC{s, manager, "admin-" + noir.RandomString(24)}
}

type JSONableSlice []uint8

func (u JSONableSlice) MarshalJSON() ([]byte, error) {
	var result string
	if u == nil {
		result = "null"
	} else {
		result = strings.Join(strings.Fields(fmt.Sprintf("%d", u)), ",")
	}
	return []byte(result), nil
}

type RelayMessage struct {
	Encoded JSONableSlice `json:"encoded"`
}

func (a *adminJSONRPC) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	switch req.Method {
	case "relay":
		router := a.manager.GetRouter()
		routerQueue := (*router).GetQueue()

		cmd := &pb.NoirRequest{}
		relayed := &RelayMessage{}
		original, _ := req.Params.MarshalJSON()
		err := json.Unmarshal(original, relayed)

		if err != nil {
			log.Errorf("unexpected client message %s\n%s", err, req.Params)
			return
		}

		err = proto.Unmarshal(relayed.Encoded, cmd)

		if err != nil {
			log.Errorf("unexpected protobuf %s\n%s", err, string(relayed.Encoded))
			return
		}

		log.Infof("got admin cmd: %s", cmd)

		cmd.AdminID = a.clientID

		noir.EnqueueRequest(*routerQueue, cmd)
	case "subscribe":
		go a.Listen(ctx, conn, req)
	}

}

func (a *adminJSONRPC) Close() {

}

func (a *adminJSONRPC) Listen(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	topic := pb.KeyTopicToAdmin(a.clientID)
	recv := a.manager.GetQueue(topic)

	log.Infof("admin bridge %s", topic)

	for {
		message, err := recv.BlockUntilNext(0)
		var reply pb.NoirReply

		err = proto.Unmarshal(message, &reply)
		if err != nil {
			log.Errorf("unmarshal err: %s", err)
			continue
		}

		encoded, _ := proto.Marshal(&reply)
		relay := &RelayMessage{Encoded: encoded}

		if reply.GetId() != "" {
			reqID := jsonrpc2.ID{Num: 0, Str: reply.GetId(), IsString: true}
			conn.Reply(ctx, reqID, relay)
		} else {
			conn.Notify(ctx, "noir", relay)
		}

	}
}
