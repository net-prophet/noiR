// Package cmd contains an entrypoint for running an ion-sfu instance.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	noir "github.com/net-prophet/noir/pkg/noir"
	"github.com/net-prophet/noir/pkg/noir/jobs"
	"github.com/net-prophet/noir/pkg/noir/servers"
	"github.com/spf13/viper"
	"net/http"
	"os"

	log "github.com/pion/ion-log"
)

var (
	conf            = noir.Config{}
	ctx             = context.Background()
	file            string
	nodeServices    string
	redisURL        string
	demoAddr        string
	grpcAddr        string
	cert            string
	key             string
	publicJrpcAddr  string
	adminJrpcAddr   string
	webGrpcAddr     string
	SFU             noir.NoirSFU
)

const (
	portRangeLimit = 100
)

func showHelp() {
	fmt.Printf("Usage:%s {params}\n", os.Args[0])
	fmt.Println("      -c {config file}")
	fmt.Println("      -u {redis url}")
	fmt.Println("      -d {demo http addr}")
	fmt.Println("      -j {public jsonrpc addr}")
	fmt.Println("      -a {admin jsonrpc addr}")
	fmt.Println("      -g {admin-grpc addr}")
	fmt.Println("      -w {web admin-grpc addr}")
	fmt.Println("      -h (show help info)")
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

	if len(conf.Ion.WebRTC.ICEPortRange) > 2 {
		fmt.Printf("config file %s loaded failed. range port must be [min,max]\n", file)
		return false
	}

	if len(conf.Ion.WebRTC.ICEPortRange) != 0 && conf.Ion.WebRTC.ICEPortRange[1]-conf.Ion.WebRTC.ICEPortRange[0] < portRangeLimit {
		fmt.Printf("config file %s loaded failed. range port must be [min, max] and max - min >= %d\n", file, portRangeLimit)
		return false
	}

	fmt.Printf("config %s load ok!\n", file)
	return true
}

func parse() bool {
	flag.StringVar(&file, "c", "/configs/sfu.toml", "config file")
	flag.StringVar(&nodeServices, "n", "*", "node services to launch")
	flag.StringVar(&redisURL, "u", "localhost:6379", "redisURL to use")
	flag.StringVar(&demoAddr, "d", "", "http addr to listen for demo")
	flag.StringVar(&publicJrpcAddr, "j", "", "jsonrpc addr for public")
	flag.StringVar(&adminJrpcAddr, "a", "", "jsonrpc addr for admin")
	flag.StringVar(&webGrpcAddr, "w", "", "web grpc addr for admin")
	flag.StringVar(&grpcAddr, "g", "", "grpc addr for admin")
	flag.StringVar(&cert, "cert", "", "public jsonrpc https cert file")
	flag.StringVar(&key, "key", "", "public jsonrpc https key file")
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

	id := noir.RandomString(8)

	log.Infof("--- noiR SFU %s [services: %s]---", id, nodeServices)

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: "",
		DB:       0,
	})

	// Test the connection
	_, err := rdb.Ping().Result()

	if err != nil {
		log.Infof("can't connect to the redis database at %s, got error:\n%v", redisURL, err)
	}
	sfu := noir.NewNoirSFU(conf)

	mgr := noir.SetupNoir(&sfu, rdb, id, nodeServices)

	worker := *(mgr.GetWorker())
	worker.RegisterHandler(jobs.LabelPlayFile, jobs.NewPlayFileHandler(&mgr))
	// worker.RegisterHandler(jobs.LabelRTMPSend, jobs.NewRTMPSendHandler(&mgr))

	go mgr.Noir()
	defer mgr.Cleanup()

	if publicJrpcAddr != "" {
		go servers.PublicJSONRPC(&mgr, publicJrpcAddr, key, cert)
	}
	if adminJrpcAddr != "" {
		go servers.AdminJSONRPC(&mgr, adminJrpcAddr, key, cert)
	}
	if grpcAddr != "" {
		go servers.AdminGRPC(&mgr, grpcAddr)
	}

	if webGrpcAddr != "" {
		go servers.AdminGRPCWeb(&mgr, webGrpcAddr)
	}

	if demoAddr != "" {
		log.Infof("demo http server running at %s", demoAddr)
		fs := http.FileServer(http.Dir("demo/"))
		http.Handle("/", fs)
		go http.ListenAndServe(demoAddr, nil)
	}
	for {
	}

}
