package noir

import (
"github.com/go-redis/redis"
	"time"
)

type Queue interface {
	Add(value []byte) error
	Next() ([]byte, error)
	BlockUntilNext(timeout time.Duration) ([]byte, error)
	Count() (int64, error)
	Cleanup() error
}

type redisQueue struct {
	client *redis.Client
	topic string
	maxAge time.Duration
}

func NewRedisQueue(client *redis.Client, topic string, maxAge time.Duration) Queue {
	return &redisQueue{client, topic, maxAge}
}

func (q *redisQueue) Add(value []byte) error {
	err := q.client.LPush(q.topic, value).Err()
	if q.maxAge > 0 {
		q.client.Expire(q.topic, q.maxAge)
	}
	return err
}

func (q *redisQueue) Cleanup() error {
	return q.client.Del(q.topic).Err()
}

func (q *redisQueue) Next() ([]byte, error) {
	count, err := q.Count()
	if err != nil {
		return nil, err
	}
	if count > 0 {
		result, err := q.client.RPop(q.topic).Result()
		return []byte(result), err
	}
	return nil, nil
}

func (q *redisQueue) BlockUntilNext(timeout time.Duration) ([]byte, error) {
	result, err := q.client.BRPop(timeout, q.topic).Result()
	if err != nil {
		return nil, err
	}
	return []byte(result[1]), nil
}

func (q *redisQueue) Count() (int64, error) {
	return q.client.LLen(q.topic).Result()
}


