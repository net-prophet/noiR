package noir

import (
	"errors"
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
)

func (w *worker) Reply(request *pb.NoirRequest, reply *pb.NoirReply) error {
	topic := pb.KeyTopicToAdmin(request.GetAdminID())
	queue := w.manager.GetQueue(topic)
	reply.Id = request.Id
	if err := EnqueueReply(queue, reply) ; err != nil {
		log.Errorf("error replying to admin %s", err)
		return err
	}
	return nil
}

func (w *worker) HandleAdmin(request *pb.NoirRequest) error {
	admin := request.GetAdmin()
	if roomAdmin := admin.GetRoomAdmin() ; roomAdmin != nil {
		if createRoom := roomAdmin.GetCreateRoom() ; createRoom != nil {
			_, err := w.manager.GetRemoteRoomData(roomAdmin.RoomID)
			if err == nil {
				return errors.New("room already exists") // Room exists
			}

			log.Infof("creating room %s", roomAdmin.RoomID)
			room := NewRoom(roomAdmin.RoomID)
			room.SetOptions(createRoom.GetOptions())
			return SaveRoomData(roomAdmin.RoomID, &room.data, w.manager)
		}
	} else if list := admin.GetRoomList() ; list != nil {
		keys := w.manager.redis.ZCount(pb.KeyRoomScores(), "1", "+inf").Val()
		rooms := []*pb.RoomListEntry{}
		for _, z := range w.manager.redis.ZRangeByScoreWithScores(pb.KeyRoomScores(),
			redis.ZRangeBy{
				Min:    "0",
				Max:    "+inf",
				Offset: 0,
				Count:  100,
			}).Val() {
			rooms = append(rooms, &pb.RoomListEntry{
				Id:    z.Member.(string),
				Score: int64(z.Score),
			})
			log.Infof("reporting %s %s", z.Member.(string), z.Score)
		}


		return w.Reply(request, &pb.NoirReply{
			Command: &pb.NoirReply_Admin{
				Admin: &pb.AdminReply{
					Payload: &pb.AdminReply_RoomList{
						RoomList: &pb.RoomListReply{
							Count: keys,
							Result: rooms,
						},
					},
				},
			},
		})
	} else {
		log.Errorf("no handler for admin command: %s", request.Action)
		return errors.New("no handler for command")
	}
	return nil
}
