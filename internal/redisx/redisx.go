// Package redisx wraps the Redis client with the primitives APAGE needs:
// the strong-consistency access-counter (spec §14 / P0-2), rate limiting
// (spec §19.6), the agent session registry (spec §19.4), idempotency keys, and
// link cache invalidation (spec §19.7).
package redisx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client with APAGE helpers.
type Client struct {
	rdb *redis.Client
}

// New parses a redis URL and connects.
func New(url string) (*Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Client{rdb: redis.NewClient(opt)}, nil
}

// Ping verifies connectivity (used by /readyz).
func (c *Client) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

// Raw exposes the underlying client for advanced use.
func (c *Client) Raw() *redis.Client { return c.rdb }

// Close closes the connection.
func (c *Client) Close() error { return c.rdb.Close() }

// ErrQuotaExceeded is returned when an atomic counter exceeds its cap.
var ErrQuotaExceeded = errors.New("access quota exceeded")

// consumeViewScript atomically increments a counter and rejects if it would
// exceed maxViews. KEYS[1]=counter key, ARGV[1]=maxViews (0=unlimited),
// ARGV[2]=ttl seconds. Returns the new count, or -1 if over the limit.
// Spec §14: single_use and maxViews are a strong-consistency admission gate;
// they must be consumed atomically before letting the request through.
var consumeViewScript = redis.NewScript(`
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local n = redis.call('INCR', KEYS[1])
if ttl > 0 and n == 1 then
  redis.call('EXPIRE', KEYS[1], ttl)
end
if max > 0 and n > max then
  return -1
end
return n
`)

// ConsumeView atomically consumes one view against maxViews. maxViews=0 means
// unlimited (counter still tracked for stats parity). Returns ErrQuotaExceeded
// when the cap is reached. single_use is expressed as maxViews=1.
func (c *Client) ConsumeView(ctx context.Context, linkID string, maxViews int, ttl time.Duration) (int64, error) {
	key := "view:" + linkID
	res, err := consumeViewScript.Run(ctx, c.rdb, []string{key}, maxViews, int(ttl.Seconds())).Int64()
	if err != nil {
		return 0, err
	}
	if res < 0 {
		return 0, ErrQuotaExceeded
	}
	return res, nil
}

// rateLimitScript implements a fixed-window counter. KEYS[1]=key,
// ARGV[1]=limit, ARGV[2]=window seconds. Returns {allowed(1/0), remaining, resetUnix}.
var rateLimitScript = redis.NewScript(`
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], window)
end
local ttl = redis.call('TTL', KEYS[1])
local reset = now + ttl
if n > limit then
  return {0, 0, reset}
end
return {1, limit - n, reset}
`)

// RateResult holds the outcome of a rate-limit check.
type RateResult struct {
	Allowed   bool
	Remaining int
	Limit     int
	ResetUnix int64
}

// RateLimit applies a fixed-window limit to key (spec §19.6).
func (c *Client) RateLimit(ctx context.Context, key string, limit int, window time.Duration) (RateResult, error) {
	now := time.Now().Unix()
	vals, err := rateLimitScript.Run(ctx, c.rdb, []string{"rl:" + key}, limit, int(window.Seconds()), now).Slice()
	if err != nil {
		return RateResult{}, err
	}
	allowed, _ := vals[0].(int64)
	remaining, _ := vals[1].(int64)
	reset, _ := vals[2].(int64)
	return RateResult{
		Allowed:   allowed == 1,
		Remaining: int(remaining),
		Limit:     limit,
		ResetUnix: reset,
	}, nil
}

// --- Agent session registry (spec §19.4) ---

// AgentReg is a live agent registration as seen by the API (spec §19.4).
type AgentReg struct {
	GatewayID       string
	GatewayURL      string
	SessionID       string
	ProtocolVersion string
	Allowlist       []string
}

// RegisterAgent records a live agent registration with a TTL strictly below the
// offline timeout (spec §19.4). gatewayURL routes previews to the owning gateway;
// protocolVersion/allowlist are the agent's actually-reported values.
func (c *Client) RegisterAgent(ctx context.Context, instanceID string, reg AgentReg, ttl time.Duration) error {
	key := "agent:" + instanceID
	allow, _ := json.Marshal(reg.Allowlist)
	if err := c.rdb.HSet(ctx, key,
		"gateway_id", reg.GatewayID,
		"gateway_url", reg.GatewayURL,
		"session_id", reg.SessionID,
		"protocol_version", reg.ProtocolVersion,
		"allowlist", string(allow),
		"updated_at", time.Now().Unix(),
	).Err(); err != nil {
		return err
	}
	// Set the TTL at registration so the key expires even if no refresh follows
	// (previously the key had no TTL until the first TouchAgent tick).
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// TouchAgent refreshes the TTL of an agent registration (heartbeat).
func (c *Client) TouchAgent(ctx context.Context, instanceID string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, "agent:"+instanceID, ttl).Err()
}

// LookupAgent returns the live registration for an instance, if online (§19.4).
func (c *Client) LookupAgent(ctx context.Context, instanceID string) (AgentReg, bool, error) {
	m, err := c.rdb.HGetAll(ctx, "agent:"+instanceID).Result()
	if err != nil {
		return AgentReg{}, false, err
	}
	if len(m) == 0 {
		return AgentReg{}, false, nil
	}
	reg := AgentReg{
		GatewayID: m["gateway_id"], GatewayURL: m["gateway_url"],
		SessionID: m["session_id"], ProtocolVersion: m["protocol_version"],
	}
	if a := m["allowlist"]; a != "" {
		_ = json.Unmarshal([]byte(a), &reg.Allowlist)
	}
	return reg, true, nil
}

// UnregisterAgent removes the mapping when a gateway loses the connection.
func (c *Client) UnregisterAgent(ctx context.Context, instanceID string) error {
	return c.rdb.Del(ctx, "agent:"+instanceID).Err()
}

// --- Idempotency (spec §"幂等") ---

// IdempotencyGet returns a cached response body for an idempotency key, if any.
func (c *Client) IdempotencyGet(ctx context.Context, scope string) (string, bool, error) {
	v, err := c.rdb.Get(ctx, "idem:"+scope).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// IdempotencySet stores a response body for an idempotency key (24h, spec §幂等).
func (c *Client) IdempotencySet(ctx context.Context, scope, body string) error {
	return c.rdb.Set(ctx, "idem:"+scope, body, 24*time.Hour).Err()
}

// --- Link cache invalidation (spec §19.7) ---

// InvalidateLink drops cached link data so revocation/freeze takes effect <=5s.
func (c *Client) InvalidateLink(ctx context.Context, linkID string) error {
	return c.rdb.Del(ctx, "link:"+linkID).Err()
}

// --- Usage metering buffer (spec §19.7: buffer in Redis, flush async) ---

// UsageDelta is one buffered usage increment awaiting flush to the DB.
type UsageDelta struct {
	TenantID string
	Dim      string
	N        int64
}

// AddUsage buffers a usage increment (egress/conversions) in Redis so the hot
// path does not write the main table (spec §19.7). The worker flushes these.
func (c *Client) AddUsage(ctx context.Context, tenantID, dim string, n int64) error {
	if n == 0 {
		return nil
	}
	if err := c.rdb.IncrBy(ctx, "usagebuf:"+tenantID+":"+dim, n).Err(); err != nil {
		return err
	}
	return c.rdb.SAdd(ctx, "usagebuf:keys", tenantID+"|"+dim).Err()
}

// DrainUsage atomically reads-and-clears all buffered usage counters (GETDEL) so
// the worker can flush them to the DB exactly once (spec §19.7).
func (c *Client) DrainUsage(ctx context.Context) ([]UsageDelta, error) {
	members, err := c.rdb.SMembers(ctx, "usagebuf:keys").Result()
	if err != nil {
		return nil, err
	}
	var out []UsageDelta
	for _, m := range members {
		parts := strings.SplitN(m, "|", 2)
		c.rdb.SRem(ctx, "usagebuf:keys", m)
		if len(parts) != 2 {
			continue
		}
		v, err := c.rdb.GetDel(ctx, "usagebuf:"+parts[0]+":"+parts[1]).Int64()
		if err != nil || v == 0 {
			continue
		}
		out = append(out, UsageDelta{TenantID: parts[0], Dim: parts[1], N: v})
	}
	return out, nil
}

// --- Task queue (spec §22.2: Asynq/Redis queue) ---

// Enqueue pushes a task payload onto a named queue (e.g. "scan", "delete").
func (c *Client) Enqueue(ctx context.Context, queue, payload string) error {
	return c.rdb.LPush(ctx, "queue:"+queue, payload).Err()
}

// Dequeue blocks up to timeout for the next task on a queue. Returns "" on timeout.
func (c *Client) Dequeue(ctx context.Context, queue string, timeout time.Duration) (string, error) {
	res, err := c.rdb.BRPop(ctx, timeout, "queue:"+queue).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(res) == 2 {
		return res[1], nil
	}
	return "", nil
}

// QueueLen returns the pending length of a queue (observability, spec §18).
func (c *Client) QueueLen(ctx context.Context, queue string) (int64, error) {
	return c.rdb.LLen(ctx, "queue:"+queue).Result()
}
