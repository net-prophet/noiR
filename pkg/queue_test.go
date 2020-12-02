package pkg

import (
	"github.com/go-redis/redis"
	"os"
	"strconv"
	"testing"
	"time"
)
func MakeTestQueue(topic string) Queue {
	if os.Getenv("TEST_REDIS") != "" {
		rdb := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("TEST_REDIS"),
			Password: "",
			DB:       0,
		})
		return NewRedisQueue(rdb, topic, 60 * time.Second)
	} else {
		return NewListQueue()
	}
}

func TestAdd(t *testing.T) {
	addTests := []struct {
		add  []string
		want int64
	}{
		{[]string{"a"}, 1},
		{[]string{"a", "b", "c"}, 3},
		{[]string{}, 0},
	}

	for n, tt := range addTests {
		queue := MakeTestQueue("test-next/"+strconv.Itoa(n))

		for _, msg := range tt.add {
			if err := queue.Add(msg); err != nil {
				t.Errorf("Error adding %s to queue: %s", msg, err)

			}
		}
		got, err := queue.Count()
		if err != nil {
			t.Errorf("error getting count %s", err)
		}
		if got != tt.want {
			t.Errorf("got %d want %d", got, tt.want)
		}

		if err := queue.Cleanup(); err != nil {
			t.Errorf("Error cleaning up: %s", err)
		}

	}
}

func TestNext(t *testing.T) {

	addTests := []struct {
		add  []string
		want []string
	}{
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{}, []string{}},
	}

	for n, tt := range addTests {
		queue := MakeTestQueue("test-next/"+strconv.Itoa(n))
		for _, msg := range tt.add {
			if err := queue.Add(msg); err != nil {
				t.Errorf("Error adding %s to queue: %s", msg, err)
			}
		}

		for _, want := range tt.want {
			got, err := queue.Next()
			if err != nil {
				t.Errorf("Error getting next %s", err)
			}
			if got != want {
				t.Errorf("got %s want %s", got, want)
			}
		}

		if err := queue.Cleanup(); err != nil {
			t.Errorf("Error cleaning up: %s", err)
		}

	}
}

// listQueue is a queue for the tests!
type listQueue struct {
	messages []string
}

func NewListQueue() Queue {
	return &listQueue{}
}

func (q *listQueue) Add(value string) error {
	q.messages = append(q.messages, value)
	return nil
}

func (q *listQueue) Cleanup() error {
	q.messages = []string{}
	return nil
}

func (q *listQueue) Next() (string, error) {
	count, _ := q.Count()
	if count > 0 {
		next := q.messages[0]
		q.messages = q.messages[1:]
		return next, nil
	}
	return "", nil
}

func (q *listQueue) BlockUntilNext(timeout time.Duration) (string, error) {
	return q.Next()
}

func (q *listQueue) Count() (int64, error) {
	return int64(len(q.messages)), nil
}
