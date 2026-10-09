package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisPrefix = "sameframe"
	presenceTimeout    = 45 * time.Second
)

type PresenceSnapshot struct {
	OnlineUserIDs []string `json:"online_user_ids"`
	OnlineCount   int      `json:"online_count"`
}

type redisTicket struct {
	Room string `json:"room"`
	User User   `json:"user"`
}

type RedisCoordinator struct {
	client *redis.Client
	prefix string
	nodeID string
}

func OpenRedis(ctx context.Context, redisURL, nodeID string) (*RedisCoordinator, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	options.Protocol = 2
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}
	if nodeID == "" {
		nodeID = mustRandomString(8)
	}
	return &RedisCoordinator{client: client, prefix: defaultRedisPrefix, nodeID: nodeID}, nil
}

func (r *RedisCoordinator) Close() error { return r.client.Close() }

func (r *RedisCoordinator) InitializeRoom(ctx context.Context, room Room) error {
	key := r.playbackKey(room.Code)
	result, err := initializeRoomScript.Run(
		ctx,
		r.client,
		[]string{key},
		room.Playback.Position,
		boolInt(room.Playback.Playing),
		room.Playback.Speed,
		room.Playback.Episode,
		room.Playback.PositionTS,
		room.Playback.SourceVersion,
		room.ExpiresAt+int64(time.Hour/time.Millisecond),
	).Result()
	if err != nil {
		return err
	}
	_ = result
	return r.CacheRoom(ctx, room)
}

func (r *RedisCoordinator) CacheRoom(ctx context.Context, room Room) error {
	encoded, err := json.Marshal(room)
	if err != nil {
		return err
	}
	ttl := time.Until(time.UnixMilli(room.ExpiresAt).Add(time.Hour))
	if ttl <= 0 {
		ttl = time.Minute
	}
	return r.client.Set(ctx, r.roomKey(room.Code), encoded, ttl).Err()
}

func (r *RedisCoordinator) Room(ctx context.Context, code string) (Room, int64, error) {
	encoded, err := r.client.Get(ctx, r.roomKey(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Room{}, 0, ErrNotFound
	}
	if err != nil {
		return Room{}, 0, err
	}
	var room Room
	if err := json.Unmarshal(encoded, &room); err != nil {
		return Room{}, 0, err
	}
	playback, sequence, err := r.Playback(ctx, code)
	if err != nil {
		return Room{}, 0, err
	}
	room.Playback = playback
	return room, sequence, nil
}

func (r *RedisCoordinator) ApplyControl(
	ctx context.Context,
	code, userID string,
	clientSequence int64,
	control Control,
) (Playback, int64, error) {
	value, err := validateControl(control)
	if err != nil {
		return Playback{}, 0, err
	}
	result, err := applyControlScript.Run(
		ctx,
		r.client,
		[]string{r.playbackKey(code)},
		time.Now().UnixMilli(),
		clientSequence,
		userID,
		control.Action,
		value,
	).Result()
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "STALE_CONTROL"):
			return Playback{}, 0, ErrStaleControl
		case strings.Contains(err.Error(), "ROOM_NOT_INITIALIZED"):
			return Playback{}, 0, ErrNotFound
		default:
			return Playback{}, 0, err
		}
	}
	return parsePlaybackResult(result)
}

func (r *RedisCoordinator) Snapshot(ctx context.Context, code string) (Playback, int64, bool, error) {
	acquired, err := r.client.SetNX(
		ctx,
		r.snapshotLockKey(code),
		r.nodeID,
		2500*time.Millisecond,
	).Result()
	if err != nil || !acquired {
		return Playback{}, 0, false, err
	}
	result, err := snapshotScript.Run(
		ctx,
		r.client,
		[]string{r.playbackKey(code)},
		time.Now().UnixMilli(),
	).Result()
	if err != nil {
		if strings.Contains(err.Error(), "ROOM_NOT_INITIALIZED") {
			return Playback{}, 0, false, ErrNotFound
		}
		return Playback{}, 0, false, err
	}
	playback, sequence, err := parsePlaybackResult(result)
	return playback, sequence, true, err
}

func (r *RedisCoordinator) Playback(ctx context.Context, code string) (Playback, int64, error) {
	values, err := r.client.HMGet(
		ctx,
		r.playbackKey(code),
		"position",
		"playing",
		"speed",
		"episode",
		"position_ts",
		"source_version",
		"seq",
	).Result()
	if err != nil {
		return Playback{}, 0, err
	}
	for _, value := range values {
		if value == nil {
			return Playback{}, 0, ErrNotFound
		}
	}
	return parsePlaybackValues(values)
}

func (r *RedisCoordinator) NextSequence(ctx context.Context, code string) (int64, error) {
	return r.client.HIncrBy(ctx, r.playbackKey(code), "seq", 1).Result()
}

func (r *RedisCoordinator) Publish(ctx context.Context, envelope Envelope) error {
	message, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, r.eventChannel(), message).Err()
}

func (r *RedisCoordinator) Subscribe(ctx context.Context) (*redis.PubSub, error) {
	pubsub := r.client.Subscribe(ctx, r.eventChannel())
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return nil, err
	}
	return pubsub, nil
}

func (r *RedisCoordinator) IssueTicket(ctx context.Context, code string, user User) (string, error) {
	ticket := mustRandomString(24)
	encoded, err := json.Marshal(redisTicket{Room: strings.ToUpper(code), User: user})
	if err != nil {
		return "", err
	}
	if err := r.client.Set(ctx, r.ticketKey(ticket), encoded, 30*time.Second).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

func (r *RedisCoordinator) ConsumeTicket(ctx context.Context, ticket, code string) (User, error) {
	encoded, err := r.client.GetDel(ctx, r.ticketKey(ticket)).Bytes()
	if errors.Is(err, redis.Nil) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	var payload redisTicket
	if json.Unmarshal(encoded, &payload) != nil || payload.Room != strings.ToUpper(code) {
		return User{}, ErrUnauthorized
	}
	return payload.User, nil
}

func (r *RedisCoordinator) IssueMediaTicket(
	ctx context.Context,
	ticket MediaTicket,
	lifetime time.Duration,
) (string, time.Time, error) {
	raw := mustRandomString(24)
	encoded, err := json.Marshal(ticket)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(lifetime)
	if err := r.client.Set(ctx, r.mediaTicketKey(raw), encoded, lifetime).Err(); err != nil {
		return "", time.Time{}, err
	}
	return raw, expiresAt, nil
}

func (r *RedisCoordinator) MediaTicket(ctx context.Context, raw string) (MediaTicket, error) {
	encoded, err := r.client.Get(ctx, r.mediaTicketKey(raw)).Bytes()
	if errors.Is(err, redis.Nil) {
		return MediaTicket{}, ErrUnauthorized
	}
	if err != nil {
		return MediaTicket{}, err
	}
	var ticket MediaTicket
	if json.Unmarshal(encoded, &ticket) != nil || ticket.UserID == "" ||
		ticket.SourceID == "" || ticket.Path == "" {
		return MediaTicket{}, ErrUnauthorized
	}
	return ticket, nil
}

func (r *RedisCoordinator) TouchPresence(
	ctx context.Context,
	code, connectionID string,
	user User,
) error {
	encoded, err := json.Marshal(user)
	if err != nil {
		return err
	}
	zset := r.presenceKey(code)
	users := r.presenceUsersKey(code)
	pipeline := r.client.TxPipeline()
	pipeline.ZAdd(ctx, zset, redis.Z{Score: float64(time.Now().UnixMilli()), Member: connectionID})
	pipeline.HSet(ctx, users, connectionID, encoded)
	pipeline.Expire(ctx, zset, 2*time.Hour)
	pipeline.Expire(ctx, users, 2*time.Hour)
	_, err = pipeline.Exec(ctx)
	return err
}

func (r *RedisCoordinator) RemovePresence(ctx context.Context, code, connectionID string) error {
	pipeline := r.client.TxPipeline()
	pipeline.ZRem(ctx, r.presenceKey(code), connectionID)
	pipeline.HDel(ctx, r.presenceUsersKey(code), connectionID)
	_, err := pipeline.Exec(ctx)
	return err
}

func (r *RedisCoordinator) Presence(ctx context.Context, code string) (PresenceSnapshot, error) {
	cutoff := time.Now().Add(-presenceTimeout).UnixMilli()
	zset := r.presenceKey(code)
	usersKey := r.presenceUsersKey(code)
	if err := r.client.ZRemRangeByScore(ctx, zset, "-inf", strconv.FormatInt(cutoff, 10)).Err(); err != nil {
		return PresenceSnapshot{}, err
	}
	connections, err := r.client.ZRange(ctx, zset, 0, -1).Result()
	if err != nil || len(connections) == 0 {
		return PresenceSnapshot{}, err
	}
	values, err := r.client.HMGet(ctx, usersKey, connections...).Result()
	if err != nil {
		return PresenceSnapshot{}, err
	}
	unique := make(map[string]struct{})
	for _, value := range values {
		encoded, ok := value.(string)
		if !ok {
			continue
		}
		var user User
		if json.Unmarshal([]byte(encoded), &user) == nil && user.ID != "" {
			unique[user.ID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return PresenceSnapshot{OnlineUserIDs: ids, OnlineCount: len(ids)}, nil
}

func (r *RedisCoordinator) FlushTestNamespace(ctx context.Context) error {
	iterator := r.client.Scan(ctx, 0, r.prefix+":*", 100).Iterator()
	keys := make([]string, 0)
	for iterator.Next(ctx) {
		keys = append(keys, iterator.Val())
	}
	if err := iterator.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

func validateControl(control Control) (string, error) {
	switch control.Action {
	case "play", "pause":
		return "", nil
	case "seek":
		if control.Position == nil || *control.Position < 0 {
			return "", fmt.Errorf("seek requires a non-negative position")
		}
		return strconv.FormatFloat(*control.Position, 'f', -1, 64), nil
	case "speed":
		if control.Speed == nil || *control.Speed < 0.5 || *control.Speed > 2 {
			return "", fmt.Errorf("speed must be between 0.5 and 2")
		}
		return strconv.FormatFloat(*control.Speed, 'f', -1, 64), nil
	case "episode":
		if control.Episode == nil || *control.Episode < 0 {
			return "", fmt.Errorf("episode requires a non-negative index")
		}
		return strconv.Itoa(*control.Episode), nil
	default:
		return "", fmt.Errorf("unsupported action %q", control.Action)
	}
}

func parsePlaybackResult(result any) (Playback, int64, error) {
	values, ok := result.([]any)
	if !ok {
		return Playback{}, 0, fmt.Errorf("unexpected Redis script response %T", result)
	}
	return parsePlaybackValues(values)
}

func parsePlaybackValues(values []any) (Playback, int64, error) {
	if len(values) != 7 {
		return Playback{}, 0, fmt.Errorf("unexpected playback field count %d", len(values))
	}
	position, err := strconv.ParseFloat(fmt.Sprint(values[0]), 64)
	if err != nil {
		return Playback{}, 0, err
	}
	playing := fmt.Sprint(values[1]) == "1"
	speed, err := strconv.ParseFloat(fmt.Sprint(values[2]), 64)
	if err != nil {
		return Playback{}, 0, err
	}
	episode, err := strconv.Atoi(fmt.Sprint(values[3]))
	if err != nil {
		return Playback{}, 0, err
	}
	positionTimestamp, err := strconv.ParseInt(fmt.Sprint(values[4]), 10, 64)
	if err != nil {
		return Playback{}, 0, err
	}
	sourceVersion, err := strconv.ParseInt(fmt.Sprint(values[5]), 10, 64)
	if err != nil {
		return Playback{}, 0, err
	}
	sequence, err := strconv.ParseInt(fmt.Sprint(values[6]), 10, 64)
	if err != nil {
		return Playback{}, 0, err
	}
	return Playback{
		Position: position, Playing: playing, Speed: speed, Episode: episode,
		PositionTS: positionTimestamp, SourceVersion: sourceVersion,
	}, sequence, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *RedisCoordinator) playbackKey(code string) string {
	return r.prefix + ":room:" + strings.ToUpper(code) + ":playback"
}

func (r *RedisCoordinator) roomKey(code string) string {
	return r.prefix + ":room:" + strings.ToUpper(code) + ":state"
}

func (r *RedisCoordinator) snapshotLockKey(code string) string {
	return r.prefix + ":room:" + strings.ToUpper(code) + ":snapshot-lock"
}

func (r *RedisCoordinator) presenceKey(code string) string {
	return r.prefix + ":room:" + strings.ToUpper(code) + ":presence"
}

func (r *RedisCoordinator) presenceUsersKey(code string) string {
	return r.prefix + ":room:" + strings.ToUpper(code) + ":presence-users"
}

func (r *RedisCoordinator) ticketKey(ticket string) string {
	return r.prefix + ":ticket:" + ticket
}

func (r *RedisCoordinator) mediaTicketKey(ticket string) string {
	return r.prefix + ":media-ticket:" + ticket
}

func (r *RedisCoordinator) eventChannel() string { return r.prefix + ":events" }

var initializeRoomScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  redis.call('HSET', KEYS[1],
    'position', ARGV[1], 'playing', ARGV[2], 'speed', ARGV[3],
    'episode', ARGV[4], 'position_ts', ARGV[5],
    'source_version', ARGV[6], 'seq', 0)
  redis.call('PEXPIREAT', KEYS[1], ARGV[7])
  return 1
end
return 0
`)

var applyControlScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return redis.error_reply('ROOM_NOT_INITIALIZED')
end
local control_field = 'control:' .. ARGV[3]
local previous = tonumber(redis.call('HGET', KEYS[1], control_field) or '-1')
local incoming = tonumber(ARGV[2])
if incoming <= previous then
  return redis.error_reply('STALE_CONTROL')
end
local now = tonumber(ARGV[1])
local position = tonumber(redis.call('HGET', KEYS[1], 'position') or '0')
local playing = tonumber(redis.call('HGET', KEYS[1], 'playing') or '0')
local speed = tonumber(redis.call('HGET', KEYS[1], 'speed') or '1')
local episode = tonumber(redis.call('HGET', KEYS[1], 'episode') or '0')
local position_ts = tonumber(redis.call('HGET', KEYS[1], 'position_ts') or ARGV[1])
local source_version = tonumber(redis.call('HGET', KEYS[1], 'source_version') or '1')
if playing == 1 and now > position_ts then
  position = position + ((now - position_ts) / 1000.0 * speed)
end
local action = ARGV[4]
if action == 'play' then
  playing = 1
elseif action == 'pause' then
  playing = 0
elseif action == 'seek' then
  position = tonumber(ARGV[5])
elseif action == 'speed' then
  speed = tonumber(ARGV[5])
elseif action == 'episode' then
  episode = tonumber(ARGV[5])
  position = 0
end
redis.call('HSET', KEYS[1],
  'position', tostring(position), 'playing', playing, 'speed', tostring(speed),
  'episode', episode, 'position_ts', now, 'source_version', source_version,
  control_field, incoming)
local seq = redis.call('HINCRBY', KEYS[1], 'seq', 1)
return {tostring(position), playing, tostring(speed), episode, now, source_version, seq}
`)

var snapshotScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return redis.error_reply('ROOM_NOT_INITIALIZED')
end
local now = tonumber(ARGV[1])
local position = tonumber(redis.call('HGET', KEYS[1], 'position') or '0')
local playing = tonumber(redis.call('HGET', KEYS[1], 'playing') or '0')
local speed = tonumber(redis.call('HGET', KEYS[1], 'speed') or '1')
local episode = tonumber(redis.call('HGET', KEYS[1], 'episode') or '0')
local position_ts = tonumber(redis.call('HGET', KEYS[1], 'position_ts') or ARGV[1])
local source_version = tonumber(redis.call('HGET', KEYS[1], 'source_version') or '1')
if playing == 1 and now > position_ts then
  position = position + ((now - position_ts) / 1000.0 * speed)
end
local seq = redis.call('HINCRBY', KEYS[1], 'seq', 1)
return {tostring(position), playing, tostring(speed), episode, now, source_version, seq}
`)
