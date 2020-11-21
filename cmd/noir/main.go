// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
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
	ctx          = context.Background()
	file         string
	redisURL     string
	nodeID       string
	cert         string
	key          string
	rpcAddr      string
	SFU          *sfu.SFU
	signalserver noir.RedisSignalServer
	seededRand   *rand.Rand = rand.New(
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
	fmt.Println("      -j {jsonrpc addr}")
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
	flag.StringVar(&file, "c", "config.toml", "config file")
	flag.StringVar(&redisURL, "u", "localhost:6379", "redisURL to use")
	flag.StringVar(&rpcAddr, "j", "", "jsonrpc addr to listen")
	flag.StringVar(&cert, "cert", "", "jsonrpc https cert file")
	flag.StringVar(&key, "key", "", "jsonrpc https key file")
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
	SFU = sfu.NewSFU(conf)

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

	signalserver = noir.NewRedisSignalServer(*SFU, rdb, nodeID)

	if rpcAddr != "" {
		go JSONRPC(signalserver)
	}

	go signalserver.SFUBus()

	for {
	}

}

func JSONRPC(signalserver noir.RedisSignalServer) {
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

		pid := StringWithCharset(32, charset)

		p := noir.NewJSONRedisSignal(
			noir.NewRedisSignal(signalserver, pid, ""))

		defer p.Close()

		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(c), p)
		<-jc.DisconnectNotify()
	}))

	var err error
	if key != "" && cert != "" {
		log.Infof("Listening at https://[%s]", rpcAddr)
		err = http.ListenAndServeTLS(rpcAddr, cert, key, nil)
	} else {
		log.Infof("Listening at http://[%s]", rpcAddr)
		err = http.ListenAndServe(rpcAddr, nil)
	}
	if err != nil {
		panic(err)
	}
}
