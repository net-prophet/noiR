// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"os"

	"github.com/go-redis/redis"
	noir "github.com/net-prophet/noir/pkg"

	"github.com/spf13/viper"

	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"
)

var (
	conf           = sfu.Config{}
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
)

const (
	portRangeLimit = 100
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
	flag.StringVar(&nodeID, "n", "", "node ID to subscribe to")
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
		go noir.PublicJSONRPC(&SFU, publicJrpcAddr, key, cert)
	}
	if adminJrpcAddr != "" {
		go noir.AdminJSONRPC(&SFU, adminJrpcAddr)
	}
	if grpcAddr != "" {
		go noir.AdminGRPC(&SFU, grpcAddr)
	}

	go SFU.Listen()

	if demoAddr != "" {

		log.Infof("demo http server running at %s", demoAddr)
		fs := http.FileServer(http.Dir("demo/"))
		http.Handle("/", fs)
		http.ListenAndServe(demoAddr, nil)
	}

	for {
	}

}
