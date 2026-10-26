package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/rueidis"
)

// Redis is the Cache for a deployment with more than one process: the api
// and the worker share it, and so do two api replicas. Tags are Redis sets
// of the keys that carry them; invalidating a tag deletes its members.
//
// The ONLY place the Redis client is constructed. A raw client in a module
// is refused by the platformservice analyzer.
type Redis struct {
	c      rueidis.Client
	prefix string
}

// NewRedis connects to addr (host:port). prefix namespaces every key and tag,
// so two projects or two test binaries can share one server.
func NewRedis(addr, password, prefix string) (*Redis, error) {
	c, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{addr}, Password: password, DisableCache: true})
	if err != nil {
		return nil, fmt.Errorf("cache: redis: %w", err)
	}
	return &Redis{c: c, prefix: prefix}, nil
}

// Close releases the connection.
func (r *Redis) Close() { r.c.Close() }

func (r *Redis) k(key string) string { return r.prefix + "k:" + key }
func (r *Redis) t(tag string) string { return r.prefix + "t:" + tag }

// Get returns the value, or false on a miss.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := r.c.Do(ctx, r.c.B().Get().Key(r.k(key)).Build()).AsBytes()
	if rueidis.IsRedisNil(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: redis get: %w", err)
	}
	return v, true, nil
}

// Set stores the value with its TTL and registers the key under each tag.
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration, tags []string) error {
	set := r.c.B().Set().Key(r.k(key)).Value(rueidis.BinaryString(value))
	var cmds []rueidis.Completed
	if ttl > 0 {
		cmds = append(cmds, set.Px(ttl).Build())
	} else {
		cmds = append(cmds, set.Build())
	}
	for _, res := range r.c.DoMulti(ctx, cmds...) {
		if err := res.Error(); err != nil {
			return fmt.Errorf("cache: redis set: %w", err)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	ms := strconv.FormatInt(ttl.Milliseconds(), 10)
	if ttl > 0 && ms == "0" {
		ms = "1" // a sub-millisecond TTL is still a TTL, not "never expires"
	}
	execs := make([]rueidis.LuaExec, 0, len(tags))
	for _, tag := range tags {
		execs = append(execs, rueidis.LuaExec{Keys: []string{r.t(tag)}, Args: []string{r.k(key), ms}})
	}
	for _, res := range tagMember.ExecMulti(ctx, r.c, execs...) {
		if err := res.Error(); err != nil {
			return fmt.Errorf("cache: redis tag: %w", err)
		}
	}
	return nil
}

// tagMember adds a key to a tag's set and keeps the set alive exactly as
// long as its longest-lived member, so a tag nobody invalidates does not
// keep every key it ever saw. A member that never expires (ttl 0) makes
// the set permanent; a new set takes its first member's TTL; an existing
// one is only ever extended (GT treats a permanent set as infinite, so it
// stays permanent). A member that expired before its tag is a harmless DEL
// of a missing key at invalidation time. One script, so the EXISTS and the
// SADD cannot be split by a concurrent writer.
var tagMember = rueidis.NewLuaScript(`
local fresh = redis.call('EXISTS', KEYS[1]) == 0
redis.call('SADD', KEYS[1], ARGV[1])
local ttl = tonumber(ARGV[2])
if ttl == 0 then
  redis.call('PERSIST', KEYS[1])
elseif fresh then
  redis.call('PEXPIRE', KEYS[1], ttl)
else
  redis.call('PEXPIRE', KEYS[1], ttl, 'GT')
end
return 1
`)

// Delete drops one key.
func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := r.c.Do(ctx, r.c.B().Del().Key(r.k(key)).Build()).Error(); err != nil {
		return fmt.Errorf("cache: redis del: %w", err)
	}
	return nil
}

// InvalidateTags deletes every key registered under any of the tags, and
// the tag sets themselves.
func (r *Redis) InvalidateTags(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		members, err := r.c.Do(ctx, r.c.B().Smembers().Key(r.t(tag)).Build()).AsStrSlice()
		if err != nil && !rueidis.IsRedisNil(err) {
			return fmt.Errorf("cache: redis smembers: %w", err)
		}
		keys := append(members, r.t(tag))
		if err := r.c.Do(ctx, r.c.B().Del().Key(keys...).Build()).Error(); err != nil {
			return fmt.Errorf("cache: redis del: %w", err)
		}
	}
	return nil
}
