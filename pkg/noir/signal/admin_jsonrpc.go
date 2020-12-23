package signal

import (
	"context"
	"encoding/json"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
)

type adminJSONRPC struct {
	sfu *noir.NoirSFU
	manager *noir.Manager
}

func NewAdminJSONRPC(s *noir.NoirSFU, manager *noir.Manager) *adminJSONRPC {
	return &adminJSONRPC{s, manager}
}

func (a *adminJSONRPC) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	log.Infof("got method %s", req.Method)

	router := a.manager.GetRouter()
	routerQueue := (*router).GetQueue()

	original, _ := req.MarshalJSON()

	var cmd *pb.NoirRequest
	json.Unmarshal(original, cmd)

	noir.EnqueueRequest(*routerQueue, cmd)

}

func (s *adminJSONRPC) Close() {

}

func (s *adminJSONRPC) Listen(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	for {
	}
}
