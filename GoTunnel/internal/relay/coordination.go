package relay

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Coordinator interface {
	Heartbeat(context.Context, string, string, int) error
}

type noopCoordinator struct{}

func (noopCoordinator) Heartbeat(context.Context, string, string, int) error { return nil }

type redisCoordinator struct {
	client *redis.Client
}

func NewRedisCoordinator(addr, password string, db int) (Coordinator, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &redisCoordinator{client: client}, nil
}

func (c *redisCoordinator) Heartbeat(ctx context.Context, relayID, region string, assigned int) error {
	key := fmt.Sprintf("gotunnel:relay:%s", relayID)
	return c.client.HSet(ctx, key, map[string]interface{}{
		"region":           region,
		"assigned_tunnels": assigned,
		"updated_at":       time.Now().UTC().Format(time.RFC3339),
	}).Err()
}
