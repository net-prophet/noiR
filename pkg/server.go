package pkg

import (
	"github.com/gorilla/websocket"
	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"net"
	"net/http"
)
// server.go contains public API handlers


func PublicJSONRPC(n *NoirSFU, publicJrpcAddr string, key string, cert string) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	public := http.NewServeMux()
	public.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		pid := RandomString(32)

		p := NewClientJSONRPCBridge(
			NewNoirPeer(*n, pid, ""))

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	server := http.Server{
		Addr:    publicJrpcAddr,
		Handler: public,
	}

	var err error
	if key != "" && cert != "" {
		log.Infof("listening at https://[%s]", publicJrpcAddr)
		err = server.ListenAndServeTLS(cert, key)
	} else {
		log.Infof("listening at http://[%s]", publicJrpcAddr)
		err = server.ListenAndServe()
	}
	if err != nil {
		panic(err)
	}

}

func AdminJSONRPC(n *NoirSFU, adminJrpcAddr string) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	http.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		p := NewAdminJSONRPC(n)

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	log.Infof("listening at http://[%s]", adminJrpcAddr)
	err := http.ListenAndServe(adminJrpcAddr, nil)

	if err != nil {
		panic(err)
	}

}

func AdminGRPC(n *NoirSFU, grpcAddr string) {
	lis, _ := net.Listen("tcp", grpcAddr)
	log.Infof("listening at %s", grpcAddr)
	s := NewGRPCServer(n)
	if err := s.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}
