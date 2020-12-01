// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"

	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"

	noir "github.com/net-prophet/noir/pkg"

	"github.com/spf13/viper"

	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"
)

var (
	conf         = sfu.Config{}
	ctx            = context.Background()
	file           string
	redisURL       string
	demoAddr       string
	grpcAddr       string
	nodeID         string
	cert           string
	key            string
	publicJrpcAddr string
	adminJrpcAddr  string
	SFU            noir.NoirSFU
	seededRand     *rand.Rand = rand.New(
		rand.NewSource(time.Now().UnixNano()))
)

const (
	portRangeLimit = 100

	charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

func showHelp() {
	fmt.Printf("Usage:%s {params}\n", os.Args[0])
	fmt.Println("      -c {config file}")
	fmt.Println("      -u {redis url}")
	fmt.Println("      -n {node id}")
	fmt.Println("      -d {demo http addr}")
	fmt.Println("      -j {public jsonrpc addr}")
	fmt.Println("      -a {admin jsonrpc addr}")
	fmt.Println("      -g {admin grpc addr}")
	fmt.Println("      -h (show help info)")
}

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

type Command struct {
	Method string `json:"method"`
}

// Join message sent when initializing a peer connection
type Join struct {
	Sid   string                    `json:"sid"`
	Offer webrtc.SessionDescription `json:"offer"`
}

// Negotiation message sent when renegotiating the peer connection
type Negotiation struct {
	Desc webrtc.SessionDescription `json:"desc"`
}

// Trickle message sent when renegotiating the peer connection
type Trickle struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

func load() bool {
	_, err := os.Stat(file)
	if err != nil {
		return false
	}

	viper.SetConfigFile(file)
	viper.SetConfigType("toml")

	err = viper.ReadInConfig()
	if err != nil {
		fmt.Printf("config file %s read failed. %v\n", file, err)
		return false
	}
	err = viper.GetViper().Unmarshal(&conf)
	if err != nil {
		fmt.Printf("sfu config file %s loaded failed. %v\n", file, err)
		return false
	}

	if len(conf.WebRTC.ICEPortRange) > 2 {
		fmt.Printf("config file %s loaded failed. range port must be [min,max]\n", file)
		return false
	}

	if len(conf.WebRTC.ICEPortRange) != 0 && conf.WebRTC.ICEPortRange[1]-conf.WebRTC.ICEPortRange[0] < portRangeLimit {
		fmt.Printf("config file %s loaded failed. range port must be [min, max] and max - min >= %d\n", file, portRangeLimit)
		return false
	}

	fmt.Printf("config %s load ok!\n", file)
	return true
}

func parse() bool {
	flag.StringVar(&file, "c", "/configs/sfu.toml", "config file")
	flag.StringVar(&redisURL, "u", "localhost:6379", "redisURL to use")
	flag.StringVar(&demoAddr, "d", ":7070", "http addr to listen for demo")
	flag.StringVar(&publicJrpcAddr, "j", ":7000", "jsonrpc addr for public")
	flag.StringVar(&adminJrpcAddr, "a", ":7001", "jsonrpc addr for admin")
	flag.StringVar(&grpcAddr, "g", ":50051", "grpc addr for admin")
	flag.StringVar(&cert, "cert", "", "public jsonrpc https cert file")
	flag.StringVar(&key, "key", "", "public jsonrpc https key file")
	flag.StringVar(&nodeID, "n", StringWithCharset(8, charset), "node ID to subscribe to")
	help := flag.Bool("h", false, "help info")
	flag.Parse()
	if !load() {
		return false
	}

	if *help {
		return false
	}
	return true
}

func main() {

	if !parse() {
		showHelp()
		os.Exit(-1)
	}

	fixByFile := []string{"asm_amd64.s", "proc.go", "icegatherer.go", "jsonrpc2"}
	fixByFunc := []string{"Handle"}
	log.Init(conf.Log.Level, fixByFile, fixByFunc)

	log.Infof("--- noiR SFU ---")

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: "",
		DB:       0,
	})

	defer rdb.Close()
	// Test the connection
	_, err := rdb.Ping().Result()

	if err != nil {
		log.Infof("can't connect to the redis database at %s, got error:\n%v", redisURL, err)
	}

	ion := sfu.NewSFU(conf)

	SFU = noir.NewNoirSFU(*ion, rdb, nodeID)

	if publicJrpcAddr != "" {
		go publicJSONRPC(&SFU)
	}
	if adminJrpcAddr != "" {
		go adminJSONRPC(&SFU)
	}
	if grpcAddr != "" {
		go adminGRPC(&SFU)
	}

	go SFU.Listen()
	
	if(demoAddr != "") {

		log.Infof("demo http server running at %s", demoAddr)
		fs := http.FileServer(http.Dir("demo/"))
		http.Handle("/", fs)
	    http.ListenAndServe(demoAddr, nil)
	}

	for {
	}

}

func publicJSONRPC(n *noir.NoirSFU) {
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

		pid := StringWithCharset(32, charset)

		p := noir.NewClientJSONRPCBridge(
			noir.NewNoirPeer(*n, pid, ""))

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	server := http.Server {
		Addr: publicJrpcAddr,
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

func adminJSONRPC(n *noir.NoirSFU) {
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

		p := noir.NewAdminJSONRPC(n)

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

func adminGRPC(n *noir.NoirSFU) {
	lis, _ := net.Listen("tcp", grpcAddr)
	log.Infof("listening at %s", grpcAddr)
	s := noir.NewGRPCServer(n)
	if err := s.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}

