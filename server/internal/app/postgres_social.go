package app

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) SearchSocialProfiles(
	ctx context.Context, viewerID, query string, limit int,
) ([]SocialProfile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.display_name, u.signature,
		 EXISTS(SELECT 1 FROM user_follows f WHERE f.follower_id=$1 AND f.followed_id=u.id),
		 EXISTS(SELECT 1 FROM user_follows f WHERE f.follower_id=u.id AND f.followed_id=$1),
		 (SELECT count(*) FROM user_follows f WHERE f.followed_id=u.id),
		 (SELECT count(*) FROM user_follows f WHERE f.follower_id=u.id)
		FROM users u LEFT JOIN user_privacy p ON p.user_id=u.id
		WHERE u.id<>$1 AND COALESCE(p.allow_profile_find, true)
		 AND ($2='' OR lower(u.display_name) LIKE '%' || lower($2) || '%' OR u.email=lower($2))
		 AND NOT EXISTS (SELECT 1 FROM user_blocks b
		   WHERE (b.blocker_id=$1 AND b.blocked_id=u.id) OR (b.blocker_id=u.id AND b.blocked_id=$1))
		ORDER BY lower(u.display_name), u.id LIMIT $3`, viewerID, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SocialProfile, 0, limit)
	for rows.Next() {
		var profile SocialProfile
		if err := rows.Scan(&profile.ID, &profile.DisplayName, &profile.Signature,
			&profile.Following, &profile.FollowsViewer, &profile.FollowerCount, &profile.FollowingCount); err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetSocialProfile(ctx context.Context, viewerID, targetID string) (SocialProfile, error) {
	var profile SocialProfile
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.display_name, u.signature,
		 EXISTS(SELECT 1 FROM user_follows f WHERE f.follower_id=$1 AND f.followed_id=u.id),
		 EXISTS(SELECT 1 FROM user_follows f WHERE f.follower_id=u.id AND f.followed_id=$1),
		 (SELECT count(*) FROM user_follows f WHERE f.followed_id=u.id),
		 (SELECT count(*) FROM user_follows f WHERE f.follower_id=u.id)
		FROM users u WHERE u.id=$2
		 AND NOT EXISTS (SELECT 1 FROM user_blocks b
		   WHERE (b.blocker_id=$1 AND b.blocked_id=u.id) OR (b.blocker_id=u.id AND b.blocked_id=$1))`,
		viewerID, targetID).Scan(&profile.ID, &profile.DisplayName, &profile.Signature,
		&profile.Following, &profile.FollowsViewer, &profile.FollowerCount, &profile.FollowingCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return SocialProfile{}, ErrNotFound
	}
	return profile, err
}

func (r *PostgresRepository) FollowUser(ctx context.Context, followerID, followedID string) error {
	if followerID == followedID {
		return errors.New("cannot follow self")
	}
	command, err := r.pool.Exec(ctx, `INSERT INTO user_follows(follower_id, followed_id)
		SELECT $1,u.id FROM users u WHERE u.id=$2 AND NOT EXISTS(
		 SELECT 1 FROM user_blocks b WHERE (b.blocker_id=$1 AND b.blocked_id=$2) OR (b.blocker_id=$2 AND b.blocked_id=$1))
		ON CONFLICT DO NOTHING`, followerID, followedID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		var exists bool
		_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_follows WHERE follower_id=$1 AND followed_id=$2)`, followerID, followedID).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *PostgresRepository) UnfollowUser(ctx context.Context, followerID, followedID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_follows WHERE follower_id=$1 AND followed_id=$2`, followerID, followedID)
	return err
}

func (r *PostgresRepository) CreateConversation(ctx context.Context, userID, peerID string) (Conversation, error) {
	if userID == peerID {
		return Conversation{}, errors.New("cannot message self")
	}
	if _, err := r.GetSocialProfile(ctx, userID, peerID); err != nil {
		return Conversation{}, err
	}
	low, high := socialPair(userID, peerID)
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO direct_conversations(user_low,user_high) VALUES($1,$2)
		ON CONFLICT(user_low,user_high) DO UPDATE SET user_low=EXCLUDED.user_low RETURNING id`, low, high).Scan(&id)
	if err != nil {
		return Conversation{}, err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO conversation_reads(conversation_id,user_id) VALUES($1,$2),($1,$3) ON CONFLICT DO NOTHING`, id, low, high)
	if err != nil {
		return Conversation{}, err
	}
	profile, err := r.GetSocialProfile(ctx, userID, peerID)
	return Conversation{ID: id, Peer: profile}, err
}

func (r *PostgresRepository) ListConversations(ctx context.Context, userID string, limit int) ([]Conversation, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id,
		 CASE WHEN c.user_low=$1 THEN c.user_high ELSE c.user_low END peer_id,
		 COALESCE(last.body,''), COALESCE((extract(epoch FROM last.created_at)*1000)::bigint,0),
		 COALESCE((SELECT count(*) FROM direct_messages m2
		   WHERE m2.conversation_id=c.id AND m2.sender_id<>$1 AND m2.id>COALESCE(cr.last_read_message_id,0)),0)
		FROM direct_conversations c
		LEFT JOIN conversation_reads cr ON cr.conversation_id=c.id AND cr.user_id=$1
		LEFT JOIN LATERAL (SELECT body,created_at FROM direct_messages m WHERE m.conversation_id=c.id ORDER BY id DESC LIMIT 1) last ON true
		WHERE (c.user_low=$1 OR c.user_high=$1)
		 AND NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
		   (b.blocker_id=$1 AND b.blocked_id=CASE WHEN c.user_low=$1 THEN c.user_high ELSE c.user_low END) OR
		   (b.blocked_id=$1 AND b.blocker_id=CASE WHEN c.user_low=$1 THEN c.user_high ELSE c.user_low END))
		ORDER BY c.updated_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Conversation, 0, limit)
	for rows.Next() {
		var item Conversation
		var peerID string
		if err := rows.Scan(&item.ID, &peerID, &item.LastMessage, &item.LastMessageAt, &item.UnreadCount); err != nil {
			return nil, err
		}
		item.Peer, err = r.GetSocialProfile(ctx, userID, peerID)
		if err == nil {
			result = append(result, item)
		}
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ListDirectMessages(ctx context.Context, userID string, conversationID, before int64, limit int) ([]DirectMessage, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM direct_conversations c WHERE c.id=$1 AND (c.user_low=$2 OR c.user_high=$2)
		AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE
		 (b.blocker_id=$2 AND b.blocked_id=CASE WHEN c.user_low=$2 THEN c.user_high ELSE c.user_low END) OR
		 (b.blocked_id=$2 AND b.blocker_id=CASE WHEN c.user_low=$2 THEN c.user_high ELSE c.user_low END)))`, conversationID, userID).Scan(&allowed)
	if err != nil || !allowed {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT id,conversation_id,sender_id,body,(extract(epoch FROM created_at)*1000)::bigint
		FROM direct_messages WHERE conversation_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, conversationID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DirectMessage, 0, limit)
	for rows.Next() {
		var m DirectMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	for l, h := 0, len(result)-1; l < h; l, h = l+1, h-1 {
		result[l], result[h] = result[h], result[l]
	}
	return result, rows.Err()
}

func (r *PostgresRepository) AddDirectMessage(ctx context.Context, senderID string, conversationID int64, body string) (DirectMessage, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DirectMessage{}, "", err
	}
	defer tx.Rollback(ctx)
	var low, high string
	err = tx.QueryRow(ctx, `SELECT user_low,user_high FROM direct_conversations WHERE id=$1 AND (user_low=$2 OR user_high=$2) FOR UPDATE`, conversationID, senderID).Scan(&low, &high)
	if errors.Is(err, pgx.ErrNoRows) {
		return DirectMessage{}, "", ErrNotFound
	}
	if err != nil {
		return DirectMessage{}, "", err
	}
	recipient := low
	if recipient == senderID {
		recipient = high
	}
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1))`, senderID, recipient).Scan(&blocked); err != nil {
		return DirectMessage{}, "", err
	}
	if blocked {
		return DirectMessage{}, "", ErrForbidden
	}
	message := DirectMessage{ConversationID: conversationID, SenderID: senderID, Body: body}
	err = tx.QueryRow(ctx, `INSERT INTO direct_messages(conversation_id,sender_id,body) VALUES($1,$2,$3) RETURNING id,(extract(epoch FROM created_at)*1000)::bigint`, conversationID, senderID, body).Scan(&message.ID, &message.CreatedAt)
	if err != nil {
		return DirectMessage{}, "", err
	}
	_, err = tx.Exec(ctx, `UPDATE direct_conversations SET updated_at=now() WHERE id=$1`, conversationID)
	if err != nil {
		return DirectMessage{}, "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO conversation_reads(conversation_id,user_id,last_read_message_id)
		VALUES($1,$2,$3) ON CONFLICT(conversation_id,user_id) DO UPDATE SET
		last_read_message_id=GREATEST(conversation_reads.last_read_message_id,EXCLUDED.last_read_message_id),updated_at=now()`, conversationID, senderID, message.ID)
	if err != nil {
		return DirectMessage{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return DirectMessage{}, "", err
	}
	return message, recipient, nil
}

func (r *PostgresRepository) ConversationPeer(ctx context.Context, userID string, conversationID int64) (string, error) {
	var peer string
	err := r.pool.QueryRow(ctx, `SELECT CASE WHEN user_low=$2 THEN user_high ELSE user_low END
		FROM direct_conversations WHERE id=$1 AND (user_low=$2 OR user_high=$2)`, conversationID, userID).Scan(&peer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return peer, err
}

func (r *PostgresRepository) MarkConversationRead(ctx context.Context, userID string, conversationID, messageID int64) error {
	command, err := r.pool.Exec(ctx, `INSERT INTO conversation_reads(conversation_id,user_id,last_read_message_id)
		SELECT c.id,$2,$3 FROM direct_conversations c WHERE c.id=$1 AND (c.user_low=$2 OR c.user_high=$2)
		ON CONFLICT(conversation_id,user_id) DO UPDATE SET last_read_message_id=GREATEST(conversation_reads.last_read_message_id,EXCLUDED.last_read_message_id),updated_at=now()`, conversationID, userID, messageID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UnreadDirectCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM direct_messages m JOIN direct_conversations c ON c.id=m.conversation_id
		LEFT JOIN conversation_reads cr ON cr.conversation_id=c.id AND cr.user_id=$1
		WHERE (c.user_low=$1 OR c.user_high=$1) AND m.sender_id<>$1 AND m.id>COALESCE(cr.last_read_message_id,0)
		AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE
		 (b.blocker_id=$1 AND b.blocked_id=CASE WHEN c.user_low=$1 THEN c.user_high ELSE c.user_low END) OR
		 (b.blocked_id=$1 AND b.blocker_id=CASE WHEN c.user_low=$1 THEN c.user_high ELSE c.user_low END))`, userID).Scan(&count)
	return count, err
}
