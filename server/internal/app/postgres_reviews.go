package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) UpsertReview(ctx context.Context, review Review) (Review, []string, error) {
	if review.ImageKeys == nil {
		review.ImageKeys = []string{}
	}
	var previous []string
	err := r.pool.QueryRow(ctx, `WITH old AS (SELECT image_keys FROM reviews WHERE user_id=$1 AND target_type=$2 AND target_id=$3), saved AS (INSERT INTO reviews(user_id,target_type,target_id,title,rating,content,image_keys) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id,target_type,target_id) DO UPDATE SET title=EXCLUDED.title,rating=EXCLUDED.rating,content=EXCLUDED.content,image_keys=EXCLUDED.image_keys,updated_at=now() RETURNING id,created_at,updated_at) SELECT saved.id,(extract(epoch FROM saved.created_at)*1000)::bigint,(extract(epoch FROM saved.updated_at)*1000)::bigint,COALESCE((SELECT image_keys FROM old),'{}'::text[]) FROM saved`, review.UserID, review.TargetType, review.TargetID, review.Title, review.Rating, review.Content, review.ImageKeys).Scan(&review.ID, &review.CreatedAt, &review.UpdatedAt, &previous)
	if err != nil {
		return Review{}, nil, err
	}
	review.Author, err = r.GetSocialProfile(ctx, review.UserID, review.UserID)
	return review, removedReviewImageKeys(previous, review.ImageKeys), err
}
func (r *PostgresRepository) ListReviews(ctx context.Context, viewer, targetType, targetID string, before int64, limit int) ([]Review, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,user_id,target_type,target_id,title,rating,content,image_keys,(extract(epoch FROM created_at)*1000)::bigint,(extract(epoch FROM updated_at)*1000)::bigint FROM reviews rv WHERE target_type=$1 AND target_id=$2 AND ($3::bigint=0 OR id<$3) AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE (b.blocker_id=$4 AND b.blocked_id=rv.user_id) OR (b.blocked_id=$4 AND b.blocker_id=rv.user_id)) ORDER BY id DESC LIMIT $5`, targetType, targetID, before, viewer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		var v Review
		if err := rows.Scan(&v.ID, &v.UserID, &v.TargetType, &v.TargetID, &v.Title, &v.Rating, &v.Content, &v.ImageKeys, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Author, err = r.GetSocialProfile(ctx, viewer, v.UserID)
		if err == nil {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}
func (r *PostgresRepository) DeleteReview(ctx context.Context, user string, id int64) ([]string, error) {
	var keys []string
	err := r.pool.QueryRow(ctx, `DELETE FROM reviews WHERE id=$1 AND user_id=$2 RETURNING image_keys`, id, user).Scan(&keys)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return keys, err
}
func (r *PostgresRepository) AddReviewComment(ctx context.Context, c ReviewComment) (ReviewComment, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO review_comments(review_id,user_id,body) SELECT rv.id,$2,$3 FROM reviews rv WHERE rv.id=$1 AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE (b.blocker_id=$2 AND b.blocked_id=rv.user_id) OR (b.blocked_id=$2 AND b.blocker_id=rv.user_id)) RETURNING id,(extract(epoch FROM created_at)*1000)::bigint`, c.ReviewID, c.UserID, c.Body).Scan(&c.ID, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewComment{}, ErrNotFound
	}
	if err != nil {
		return ReviewComment{}, err
	}
	c.Author, err = r.GetSocialProfile(ctx, c.UserID, c.UserID)
	return c, err
}
func (r *PostgresRepository) ListReviewComments(ctx context.Context, viewer string, reviewID, before int64, limit int) ([]ReviewComment, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id,c.review_id,c.user_id,c.body,(extract(epoch FROM c.created_at)*1000)::bigint FROM review_comments c WHERE c.review_id=$1 AND ($2::bigint=0 OR c.id<$2) AND NOT EXISTS(SELECT 1 FROM user_blocks b WHERE (b.blocker_id=$3 AND b.blocked_id=c.user_id) OR (b.blocked_id=$3 AND b.blocker_id=c.user_id)) ORDER BY c.id DESC LIMIT $4`, reviewID, before, viewer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReviewComment{}
	for rows.Next() {
		var c ReviewComment
		if err := rows.Scan(&c.ID, &c.ReviewID, &c.UserID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Author, err = r.GetSocialProfile(ctx, viewer, c.UserID)
		if err == nil {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}
