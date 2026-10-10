package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func (s *Server) withRoomMutation(ctx context.Context, code string, work func(context.Context) error) error {
	if len(code) != 6 {
		return ErrNotFound
	}
	if s.redis == nil {
		s.roomMutationMu.Lock()
		defer s.roomMutationMu.Unlock()
		return work(ctx)
	}
	key := s.redis.prefix + ":mutation:" + strings.ToUpper(code)
	token := mustRandomString(18)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		ok, err := s.redis.client.SetNX(ctx, key, token, 15*time.Second).Result()
		if err != nil {
			return errors.New("房间状态服务暂不可用")
		}
		if ok {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return ErrRoomConflict
		case <-time.After(25 * time.Millisecond):
		}
	}
	locked, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-locked.Done():
				return
			case <-ticker.C:
				ok, err := redis.NewScript(`if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('PEXPIRE',KEYS[1],15000) end return 0`).Run(locked, s.redis.client, []string{key}, token).Int()
				if err != nil || ok != 1 {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		close(done)
		cancel()
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_, _ = redis.NewScript(`if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`).Run(cleanup, s.redis.client, []string{key}, token).Result()
	}()
	return work(locked)
}

func (s *Server) authorizedRoom(ctx context.Context, code string, user User) (Room, RoomFeatures, error) {
	code = strings.ToUpper(code)
	if err := s.refreshRedisRoom(ctx, code); err != nil && !errors.Is(err, ErrNotFound) {
		return Room{}, RoomFeatures{}, err
	}
	room, err := s.store.GetRoom(code, user.ID)
	if err != nil {
		return Room{}, RoomFeatures{}, err
	}
	f, err := s.repo.GetRoomFeatures(ctx, code)
	if err != nil {
		return Room{}, RoomFeatures{}, err
	}
	if f.blocked(user.ID) || (f.role(room, user.ID) == "guest" && !f.AllowGuests) {
		return Room{}, RoomFeatures{}, ErrForbidden
	}
	if user.GuestRoomCode != "" && (user.GuestRoomCode != code || user.GuestExpiresAt <= time.Now().UnixMilli()) {
		return Room{}, RoomFeatures{}, ErrForbidden
	}
	return room, f, nil
}

func (s *Server) roomForViewer(ctx context.Context, room Room, userID string) (Room, error) {
	f, err := s.repo.GetRoomFeatures(ctx, room.Code)
	if err != nil {
		return Room{}, err
	}
	if f.blocked(userID) {
		return Room{}, ErrForbidden
	}
	if f.role(room, userID) == "guest" && !f.AllowGuests {
		return Room{}, ErrForbidden
	}
	view := f.view(room, userID)
	room.Features = &view
	room.Members = append([]Member(nil), room.Members...)
	for i := range room.Members {
		room.Members[i].Role = f.role(room, room.Members[i].UserID)
	}
	return room, nil
}

func (s *Server) processPlaybackControl(ctx context.Context, code string, user User, sequence int64, control Control) error {
	return s.withRoomMutation(ctx, code, func(ctx context.Context) error {
		room, f, err := s.authorizedRoom(ctx, code, user)
		if err != nil {
			return err
		}
		if !f.grants(room, user.ID).Playback {
			return ErrForbidden
		}
		if source, _, ok := f.activeSource(); ok && source.IsLive && (control.Action == "seek" || control.Action == "speed") {
			return errors.New("直播片源不支持拖动和倍速")
		}
		if control.SourceVersion != nil && *control.SourceVersion != room.Playback.SourceVersion {
			return ErrRoomConflict
		}
		if control.Action == "episode" && len(f.Playlist) > 0 {
			return errors.New("请从房间片单切换剧集")
		}
		var updated Room
		var serverSequence int64
		if s.redis != nil {
			var playback Playback
			playback, serverSequence, err = s.redis.ApplyControl(ctx, code, user.ID, sequence, control)
			if err == nil {
				updated, err = s.store.ApplyAuthoritativePlayback(code, playback, serverSequence)
			}
		} else {
			updated, serverSequence, err = s.store.applyControl(code, user, sequence, control, true)
		}
		if err != nil {
			return err
		}
		if err = s.repo.UpdatePlayback(ctx, code, updated.Playback); err != nil {
			return errors.New("播放状态保存失败")
		}
		s.broadcastSnapshot(ctx, updated, serverSequence, user.ID)
		return nil
	})
}

func (s *Server) clientRoom(ctx context.Context, room Room, viewer string) (Room, error) {
	var err error
	room, err = s.roomForViewer(ctx, room, viewer)
	if err != nil {
		return Room{}, err
	}
	if room.MediaSourceID != "" {
		room, _, err = s.issueTicketForRoom(ctx, room, viewer)
		if err != nil {
			return Room{}, err
		}
	}
	return room, nil
}

func (s *Server) publishFeatureChange(ctx context.Context, room Room) error {
	room.Features = nil
	if s.redis != nil {
		if err := s.redis.CacheRoom(ctx, room); err != nil {
			return err
		}
	}
	state, sequence, err := s.roomStateForBroadcast(ctx, room.Code)
	if err != nil {
		return err
	}
	s.broadcastRoomState(ctx, state, sequence)
	return nil
}

func (s *Server) saveSelectedMedia(ctx context.Context, room Room, f RoomFeatures, selection RoomMediaSelection) error {
	previous := f.Version
	f.Version++
	if err := s.repo.SaveRoomFeatures(ctx, room.Code, previous, f, &selection); err != nil {
		return err
	}
	room.MediaSourceID = selection.MediaSourceID
	room.MediaPath = selection.MediaPath
	room.SourceURL = selection.SourceURL
	room.Playback = selection.Playback
	room.Features = nil
	var sequence int64
	if s.redis != nil {
		encoded, _ := json.Marshal(room)
		result, err := redis.NewScript(`local seq=redis.call('HINCRBY',KEYS[1],'seq',1)
		redis.call('HSET',KEYS[1],'position',ARGV[1],'playing',ARGV[2],'speed',ARGV[3],'episode',ARGV[4],'position_ts',ARGV[5],'source_version',ARGV[6])
		redis.call('SET',KEYS[2],ARGV[7],'PX',ARGV[8]); return seq`).Run(ctx, s.redis.client, []string{s.redis.playbackKey(room.Code), s.redis.roomKey(room.Code)},
			selection.Playback.Position, boolInt(selection.Playback.Playing), selection.Playback.Speed, selection.Playback.Episode, selection.Playback.PositionTS, selection.Playback.SourceVersion,
			encoded, max(int64(time.Until(time.UnixMilli(room.ExpiresAt).Add(time.Hour))/time.Millisecond), 1000)).Int64()
		if err != nil {
			return errors.New("影片已保存，实时状态暂不可用，请重新进入房间")
		}
		sequence = result
	} else {
		_, sequence, _ = s.store.Snapshot(room.Code)
	}
	s.store.UpsertAuthoritativeRoom(room, sequence)
	s.broadcastRoomState(ctx, room, sequence)
	return nil
}

func writeRoomFeatureError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRoomConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrNotFound), errors.Is(err, ErrRoomClosed), errors.Is(err, ErrExpired), errors.Is(err, ErrRoomFull):
		writeStoreError(w, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
