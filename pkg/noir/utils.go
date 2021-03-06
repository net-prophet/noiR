package noir

import (
	"errors"
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	proto "google.golang.org/protobuf/proto"
	"math/rand"
	"os"
	"sync/atomic"
	"time"
)

const (
	charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	publisher  = 0
	subscriber = 1
)

var (
	seededRand *rand.Rand = rand.New(
		rand.NewSource(time.Now().UnixNano()))
)

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func RandomString(length int) string {
	return StringWithCharset(length, charset)
}

type atomicBool struct {
	val int32
}

func (b *atomicBool) set(value bool) { // nolint: unparam
	var i int32
	if value {
		i = 1
	}

	atomic.StoreInt32(&(b.val), i)
}

func (b *atomicBool) get() bool {
	return atomic.LoadInt32(&(b.val)) != 0
}

// ROUTER UTILS

func ReadAction(request *pb.NoirRequest) (string, error) {
	switch request.Command.(type) {
	case *pb.NoirRequest_Signal:
		return ReadSignalAction(request.GetSignal())
	case *pb.NoirRequest_Admin:
		return ReadAdminAction(request.GetAdmin())
	}
	return "", errors.New("unhandled action")
}

func ReadSignalAction(signal *pb.SignalRequest) (string, error) {
	action := "request.servers."
	switch signal.Payload.(type) {
	case *pb.SignalRequest_Join:
		return action + "join", nil
	case *pb.SignalRequest_Description:
		return action + "description", nil
	case *pb.SignalRequest_Trickle:
		return action + "trickle", nil
	case *pb.SignalRequest_Kill:
		return action + "kill", nil
	}
	return action, errors.New("unhandled servers")
}

func ReadAdminAction(admin *pb.AdminRequest) (string, error) {
	action := "request.admin."
	switch admin.Payload.(type) {
	case *pb.AdminRequest_RoomList:
		return action + "list_rooms", nil
	case *pb.AdminRequest_RoomAdmin:
			roomAdmin := admin.GetRoomAdmin()
			switch roomAdmin.Method.(type) {
				case *pb.RoomAdminRequest_CreateRoom:
					return action + "room.create", nil
				case *pb.RoomAdminRequest_RoomJob:
					return action + "room.runjob", nil
				default:
					return action, errors.New("unhandled roomadmin")
			}
	default:
		return action, errors.New("unhandled admin")
	}
}

func ParseSDP(offer webrtc.SessionDescription) (*sdp.SessionDescription, error) {
	desc, err := offer.Unmarshal()
	if err != nil {
		return nil, err
	}
	numTracks := len(desc.MediaDescriptions)

	if numTracks == 0 {
		return nil, errors.New("offer must include at least an empty datachannel")
	}
	return desc, nil
}


func FillDefaults(value *pb.NoirRequest) {
	if value.At == "" {
		now, _ := time.Now().MarshalText()
		value.At = string(now)
	}

	if value.Id == "" {
		value.Id = "notify-" + RandomString(24)
	}

	if value.Action == "" {
		action, action_err := ReadAction(value)
		if action_err != nil {
			log.Errorf("error getting action %s", action_err)
		}
		value.Action = action
	}
}

func MarshalRequest(value *pb.NoirRequest) ([]byte, error) {
	FillDefaults(value)
	return proto.Marshal(value)
}

func MarshalReply(value *pb.NoirReply) ([]byte, error) {
	return proto.Marshal(value)
}

func UnmarshalRequest(message []byte, destination *pb.NoirRequest) error {
	return proto.Unmarshal(message, destination)
}

func EnqueueRequest(queue Queue, value *pb.NoirRequest) error {
	command, err := MarshalRequest(value)
	if err != nil {
		return err
	}
	err = queue.Add(command)
	return err
}

func EnqueueReply(queue Queue, value *pb.NoirReply) error {
	command, err := MarshalReply(value)
	if err != nil {
		return err
	}
	err = queue.Add(command)
	return err
}

// TEST UTILS
func NewTestQueue(topic string) Queue {
	redisUrl := os.Getenv("TEST_REDIS")
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisUrl,
		Password: "",
		DB:       0,
	})
	return NewRedisQueue(rdb, topic, 60*time.Second)
}

func NewTestSetup() (Manager, *redis.Client) {
	driver := os.Getenv("TEST_REDIS")
	rdb := redis.NewClient(&redis.Options{
		Addr:     driver,
		Password: "",
		DB:       0,
	})
	config := Config{}
	sfu := NewNoirSFU(config)
	return SetupNoir(&sfu, rdb, "test-worker", "*"), rdb
}

// listQueue is a queue for the tests!
type listQueue struct {
	messages [][]byte
	topic    string
}

func (q *listQueue) Topic() string {
	return q.topic
}

func NewListQueue(topic string) Queue {
	return &listQueue{messages: [][]byte{}, topic: topic}
}

func (q *listQueue) Add(value []byte) error {
	q.messages = append(q.messages, value)
	return nil
}

func (q *listQueue) Cleanup() error {
	q.messages = [][]byte{}
	return nil
}

func (q *listQueue) Next() ([]byte, error) {
	count, _ := q.Count()
	if count > 0 {
		next := q.messages[0]
		q.messages = q.messages[1:]
		return next, nil
	}
	return nil, nil
}

func (q *listQueue) BlockUntilNext(timeout time.Duration) ([]byte, error) {
	return q.Next()
}

func (q *listQueue) Count() (int64, error) {
	return int64(len(q.messages)), nil
}
