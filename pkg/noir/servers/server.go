package servers

import (
	"github.com/gorilla/websocket"
	"github.com/net-prophet/noir/pkg/noir"
	log "github.com/pion/ion-log"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"net"
	"net/http"
)

// server.go contains public API handlers

func PublicJSONRPC(mgr *noir.Manager, publicJrpcAddr string, key string, cert string) {
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

		pid := noir.RandomString(32)

		p := NewClientJSONRPCBridge(pid, mgr)

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	server := http.Server{
		Addr:    publicJrpcAddr,
		Handler: public,
	}

	var err error
	if false && key != "" && cert != "" {
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

func AdminJSONRPC(mgr *noir.Manager, adminJrpcAddr string, key string, cert string) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	admin := http.NewServeMux()
	admin.Handle("/admin/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close()

		p := NewAdminJSONRPC(mgr.SFU(), mgr)
		log.Infof("admin client connected %s", p.clientID)

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	server := http.Server{
		Addr:    adminJrpcAddr,
		Handler: admin,
	}

	var err error;
	if key != "" && cert != "" {
		log.Infof("listening at https://[%s]", adminJrpcAddr)
		err = server.ListenAndServeTLS(cert, key)
	} else {
		log.Infof("listening at http://[%s]", adminJrpcAddr)
		err = server.ListenAndServe()
	}

	if err != nil {
		panic(err)
	}

}

func AdminGRPC(m *noir.Manager, grpcAddr string) {
	lis, _ := net.Listen("tcp", grpcAddr)
	grpc := NewGRPCServer(m)

	log.Infof("grpc listening at %s", grpcAddr)

	if err := grpc.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
	select {}
}

func AdminGRPCWeb(m *noir.Manager, webAddr string) {
	options := DefaultWrapperedServerOptions()
	options.EnableTLS = false
	options.Addr = webAddr
	options.AllowAllOrigins = true
	options.UseWebSocket = true
	web := NewWrapperedGRPCWebServer(options, m)
	// serve() prints more info than this itself
	//log.Infof("grpc-web listening at %s", webAddr)
	if err := web.Serve(); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
	select {}
}
