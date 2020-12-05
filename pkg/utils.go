package noir

import (
	"errors"
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	proto "google.golang.org/protobuf/proto"
	"os"
	"time"
	"math/rand"
)

const (
	charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
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

// ROUTER UTILS

func ReadAction(request *pb.NoirRequest) (string, error) {
	switch request.Command.(type) {
	case *pb.NoirRequest_Signal:
		return ReadSignalAction(request.GetSignal())
	}
	return "", errors.New("unhandled action")
}

func ReadSignalAction(signal *pb.SignalRequest) (string, error) {
	action := "request.signal."
	switch signal.Payload.(type) {
	case *pb.SignalRequest_Join:
		return action + "join", nil
	}
	return action, errors.New("unhandled action")
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

// TEST UTILS
func MakeTestDrivers() []string {
	drivers := []string{"locmem",}
	if os.Getenv("TEST_REDIS") != "" {
		drivers = append(drivers, os.Getenv("TEST_REDIS"))
	}
	return drivers
}

func MakeTestQueue(redisUrl string, topic string) Queue {
	if redisUrl != "" && redisUrl != "locmem" {
		rdb := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("TEST_REDIS"),
			Password: "",
			DB:       0,
		})
		return NewRedisQueue(rdb, topic, 60*time.Second)
	} else {
		return NewListQueue()
	}
}

// listQueue is a queue for the tests!
type listQueue struct {
	messages [][]byte
}

func NewListQueue() Queue {
	return &listQueue{}
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
