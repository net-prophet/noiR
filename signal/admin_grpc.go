package signal

import (
	noir "github.com/net-prophet/noir/pkg"
	"io"

	log "github.com/pion/ion-log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/net-prophet/noir/pkg/proto"
)

func NewGRPCServer(sfu *noir.NoirSFU) *grpc.Server {
	s := grpc.NewServer()
	pb.RegisterNoirSFUServer(s, &SFUServer{SFU: sfu})
	return s
}

type SFUServer struct {
	pb.UnimplementedNoirSFUServer
	SFU *noir.NoirSFU
}

// TODO: implement admin grpc

func (s *SFUServer) Command(stream pb.NoirSFU_AdminServer) error {
	//peer := sfu.NewPeer(s.Ion().SFU)
	for {
		in, err := stream.Recv()

		if err != nil {
			//peer.Close()

			if err == io.EOF {
				return nil
			}

			errStatus, _ := status.FromError(err)
			if errStatus.Code() == codes.Canceled {
				return nil
			}

			log.Errorf("signal error %v %v", errStatus.Message(), errStatus.Code())
			return err
		}

		log.Debugf("got command:\n%v", in.Command)
		switch payload := in.Command.(type) {
		case *pb.NoirRequest_Signal:
			log.Debugf("signal called:\n%v", payload)

		}
	}
}
