package noir

import (
	"errors"
	"github.com/go-redis/redis"
	pb "github.com/net-prophet/noir/pkg/proto"
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
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
func NewTestQueue(topic string) Queue {
	redisUrl := os.Getenv("TEST_REDIS")
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisUrl,
		Password: "",
		DB:       0,
	})
	return NewRedisQueue(rdb, topic, 60*time.Second)
}

func NewTestSetup() Manager {
	driver := os.Getenv("TEST_REDIS")
	rdb := redis.NewClient(&redis.Options{
		Addr:     driver,
		Password: "",
		DB:       0,
	})
	config := sfu.Config{}
	sfu := NewNoirSFU(config)
	return NewManager(&sfu, rdb, "test-worker")
}

// listQueue is a queue for the tests!
type listQueue struct {
	messages [][]byte
	topic string
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
