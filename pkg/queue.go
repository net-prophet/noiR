package pkg

import (
"github.com/go-redis/redis"
	"time"
)

type Queue interface {
	Add(value string) error
	Next() (string, error)
	BlockUntilNext(timeout time.Duration) (string, error)
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

func (q *redisQueue) Add(value string) error {
	err := q.client.LPush(q.topic, value).Err()
	if q.maxAge > 0 {
		q.client.Expire(q.topic, q.maxAge)
	}
	return err
}

func (q *redisQueue) Cleanup() error {
	return q.client.Del(q.topic).Err()
}

func (q *redisQueue) Next() (string, error) {
	count, err := q.Count()
	if err != nil {
		return "", err
	}
	if count > 0 {
		return q.client.RPop(q.topic).Result()
	}
	return "", nil
}

func (q *redisQueue) BlockUntilNext(timeout time.Duration) (string, error) {
	result, err := q.client.BRPop(timeout, q.topic).Result()
	if err != nil {
		return "", err
	}
	return result[1], nil
}

func (q *redisQueue) Count() (int64, error) {
	return q.client.LLen(q.topic).Result()
}


