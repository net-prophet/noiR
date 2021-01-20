package servers

import (
	"context"
	"github.com/golang/protobuf/proto"
	"github.com/net-prophet/noir/pkg/noir"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
)

type SFUServer struct {
	pb.UnimplementedNoirServer
	manager *noir.Manager
}

func NewGRPCServer(manager *noir.Manager) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterNoirServer(s, &SFUServer{manager: manager})
	return s
}

func (s *SFUServer) Admin(stream pb.Noir_AdminServer) error {
	id := noir.RandomString(8)
	go s.AdminBridge(id, stream)

	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			errStatus, _ := status.FromError(err)
			log.Errorf("admin error %v %v", errStatus.Message(), errStatus.Code())
			return err
		}
		log.Infof("Got admin command %s", in)
		s.Handle(in, id)
	}
	return nil
}

func (s *SFUServer) Send(ctx context.Context, message *pb.NoirRequest) (*pb.Empty, error) {
	return s.Handle(message, message.GetAdminID())
}
func (s *SFUServer) Handle(message *pb.NoirRequest, clientID string) (*pb.Empty, error) {
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
		message.AdminID = clientID
		noir.EnqueueRequest(*routerQueue, message)
	}

	return &pb.Empty{}, nil
}

func (s *SFUServer) Subscribe(client *pb.AdminClient, stream pb.Noir_SubscribeServer) error {
	//peer := noir.NewUser(, "123")
	for {
	}
}

func (s *SFUServer) AdminBridge(clientID string, stream pb.Noir_AdminServer) error {
	topic := pb.KeyTopicToAdmin(clientID)
	recv := s.manager.GetQueue(topic)

	log.Infof("admin bridge %s", topic)

	for {
		message, err := recv.BlockUntilNext(0)
		var reply pb.NoirReply

		err = proto.Unmarshal(message, &reply)
		if err != nil {
			log.Errorf("unmarshal err: %s", err)
			continue
		}
		err = stream.Send(&reply)
		if err != nil {
			log.Errorf("grpc send error %v", err)
			return status.Errorf(codes.Internal, err.Error())
		}
	}

	}
