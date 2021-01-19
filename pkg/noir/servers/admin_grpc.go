package servers

import (
	"context"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"google.golang.org/grpc"
)

func NewGRPCServer(manager *noir.Manager) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterNoirServer(s, &SFUServer{manager: manager})
	return s
}

// TODO: implement admin grpc

func (s *SFUServer) Send(ctx context.Context, message *pb.NoirRequest) (*pb.Empty, error) {
	router := s.manager.GetRouter()
	routerQueue := (*router).GetQueue()

	if message != nil {
		log.Infof("Send requested: message=%v", *message)
	} else {
		log.Infof("Send requested: message=<empty>")
	}
	log.Debugf("got command:\n%v", message.Command)
	switch payload := message.Command.(type) {
	case *pb.NoirRequest_Signal:
		log.Infof("servers called:\n%v", payload)
		noir.EnqueueRequest(*routerQueue, message)

	case *pb.NoirRequest_Admin:
		log.Infof("admin called:\n%v", payload)
		noir.EnqueueRequest(*routerQueue, message)
	}

	return &pb.Empty{}, nil
}

func (s *SFUServer) Subscribe(client *pb.AdminClient, stream pb.Noir_SubscribeServer) error {
	//peer := noir.NewUser(, "123")
	for {
	}
}
