package signal

import (
	"context"
	noir "github.com/net-prophet/noir/pkg"
	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
)

type adminJSONRPC struct {
	sfu *noir.NoirSFU
}

func NewAdminJSONRPC(s *noir.NoirSFU) *adminJSONRPC {
	return &adminJSONRPC{s}
}

func (a *adminJSONRPC) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	log.Infof("got method %s", req.Method)
}

func (s *adminJSONRPC) Close() {
}

func (s *adminJSONRPC) Listen(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	for {
	}
}