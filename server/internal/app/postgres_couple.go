package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateCoupleRequest(ctx context.Context, requester, recipient string) (CoupleRequest, error) {
	if requester == recipient {
		return CoupleRequest{}, errors.New("cannot bind self")
	}
	if _, err := r.GetSocialProfile(ctx, requester, recipient); err != nil {
		return CoupleRequest{}, err
	}
	var occupied bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM couples WHERE status='active' AND (user_low IN($1,$2) OR user_high IN($1,$2)))`, requester, recipient).Scan(&occupied); err != nil {
		return CoupleRequest{}, err
	}
	if occupied {
		return CoupleRequest{}, errors.New("user already has a couple")
	}
	var q CoupleRequest
	q.RecipientID = recipient
	q.Status = "pending"
	err := r.pool.QueryRow(ctx, `INSERT INTO couple_requests(requester_id,recipient_id) VALUES($1,$2) ON CONFLICT(requester_id,recipient_id) WHERE status='pending' DO UPDATE SET requester_id=EXCLUDED.requester_id RETURNING id,(extract(epoch FROM created_at)*1000)::bigint`, requester, recipient).Scan(&q.ID, &q.CreatedAt)
	if err != nil {
		return CoupleRequest{}, err
	}
	q.Requester, err = r.GetSocialProfile(ctx, requester, requester)
	return q, err
}
func (r *PostgresRepository) ListCoupleRequests(ctx context.Context, user string) ([]CoupleRequest, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,requester_id,(extract(epoch FROM created_at)*1000)::bigint FROM couple_requests WHERE recipient_id=$1 AND status='pending' ORDER BY id DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CoupleRequest{}
	for rows.Next() {
		var q CoupleRequest
		var requester string
		if err := rows.Scan(&q.ID, &requester, &q.CreatedAt); err != nil {
			return nil, err
		}
		q.RecipientID = user
		q.Status = "pending"
		q.Requester, err = r.GetSocialProfile(ctx, user, requester)
		if err == nil {
			out = append(out, q)
		}
	}
	return out, rows.Err()
}
func (r *PostgresRepository) RespondCoupleRequest(ctx context.Context, user string, id int64, accept bool) (Couple, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Couple{}, err
	}
	defer tx.Rollback(ctx)
	var requester, recipient, status string
	err = tx.QueryRow(ctx, `SELECT requester_id,recipient_id,status FROM couple_requests WHERE id=$1 AND recipient_id=$2 FOR UPDATE`, id, user).Scan(&requester, &recipient, &status)
	if errors.Is(err, pgx.ErrNoRows) || status != "pending" {
		return Couple{}, ErrNotFound
	}
	if err != nil {
		return Couple{}, err
	}
	if !accept {
		_, err = tx.Exec(ctx, `UPDATE couple_requests SET status='rejected',reviewed_at=now() WHERE id=$1`, id)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return Couple{}, err
	}
	var vipActive bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(vip_expires_at>now(),false) FROM users WHERE id=$1`, requester).Scan(&vipActive); err != nil {
		return Couple{}, err
	}
	if !vipActive {
		return Couple{}, ErrForbidden
	}
	low, high := socialPair(requester, recipient)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)), pg_advisory_xact_lock(hashtext($2))`, low, high); err != nil {
		return Couple{}, err
	}
	var occupied bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM couples WHERE status='active' AND (user_low IN($1,$2) OR user_high IN($1,$2)))`, requester, recipient).Scan(&occupied); err != nil {
		return Couple{}, err
	}
	if occupied {
		return Couple{}, errors.New("user already has a couple")
	}
	var coupleID int64
	err = tx.QueryRow(ctx, `INSERT INTO couples(user_low,user_high,status,bound_at,separated_at,cooling_period_end,updated_at) VALUES($1,$2,'active',now(),NULL,NULL,now()) ON CONFLICT(user_low,user_high) DO UPDATE SET status='active',bound_at=now(),separated_at=NULL,cooling_period_end=NULL,updated_at=now() RETURNING id`, low, high).Scan(&coupleID)
	if err != nil {
		return Couple{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE couple_requests SET status='accepted',reviewed_at=now() WHERE id=$1`, id); err != nil {
		return Couple{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO couple_events(couple_id,event_type) VALUES($1,'bound')`, coupleID); err != nil {
		return Couple{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Couple{}, err
	}
	return r.GetCouple(ctx, user)
}
func (r *PostgresRepository) GetCouple(ctx context.Context, user string) (Couple, error) {
	var c Couple
	var low, high string
	err := r.pool.QueryRow(ctx, `SELECT id,user_low,user_high,status,(extract(epoch FROM bound_at)*1000)::bigint,COALESCE((extract(epoch FROM separated_at)*1000)::bigint,0),COALESCE((extract(epoch FROM cooling_period_end)*1000)::bigint,0) FROM couples WHERE (user_low=$1 OR user_high=$1) AND status<>'ended' ORDER BY updated_at DESC LIMIT 1`, user).Scan(&c.ID, &low, &high, &c.Status, &c.BoundAt, &c.SeparatedAt, &c.CoolingPeriodEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return Couple{}, ErrNotFound
	}
	if err != nil {
		return Couple{}, err
	}
	peer := low
	if peer == user {
		peer = high
	}
	c.Partner, err = r.GetSocialProfile(ctx, user, peer)
	return c, err
}
func (r *PostgresRepository) SeparateCouple(ctx context.Context, user string) (Couple, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `UPDATE couples SET status='separated',separated_at=now(),cooling_period_end=now()+interval '7 days',updated_at=now() WHERE (user_low=$1 OR user_high=$1) AND status='active' RETURNING id`, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Couple{}, ErrNotFound
	}
	if err != nil {
		return Couple{}, err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO couple_events(couple_id,event_type) VALUES($1,'separated')`, id)
	if err != nil {
		return Couple{}, err
	}
	return r.GetCouple(ctx, user)
}
func (r *PostgresRepository) RestoreCouple(ctx context.Context, user string) (Couple, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `UPDATE couples SET status='active',separated_at=NULL,cooling_period_end=NULL,updated_at=now() WHERE (user_low=$1 OR user_high=$1) AND status='separated' AND cooling_period_end>now() RETURNING id`, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Couple{}, ErrForbidden
	}
	if err != nil {
		return Couple{}, err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO couple_events(couple_id,event_type) VALUES($1,'restored')`, id)
	if err != nil {
		return Couple{}, err
	}
	return r.GetCouple(ctx, user)
}
func (r *PostgresRepository) AddCoupleMoment(ctx context.Context, user, body string) (CoupleMoment, error) {
	var m CoupleMoment
	var author string
	var coupleID int64
	err := r.pool.QueryRow(ctx, `INSERT INTO couple_moments(couple_id,author_id,body) SELECT id,$1,$2 FROM couples WHERE (user_low=$1 OR user_high=$1) AND status='active' RETURNING id,couple_id,author_id,(extract(epoch FROM created_at)*1000)::bigint`, user, body).Scan(&m.ID, &coupleID, &author, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CoupleMoment{}, ErrForbidden
	}
	if err != nil {
		return CoupleMoment{}, err
	}
	m.Body = body
	if _, eventErr := r.pool.Exec(ctx, `INSERT INTO couple_events(couple_id,event_type) VALUES($1,'moment')`, coupleID); eventErr != nil {
		return CoupleMoment{}, eventErr
	}
	m.Author, err = r.GetSocialProfile(ctx, user, author)
	return m, err
}
func (r *PostgresRepository) ListCoupleMoments(ctx context.Context, user string, before int64, limit int) ([]CoupleMoment, error) {
	couple, err := r.GetCouple(ctx, user)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id,author_id,body,(extract(epoch FROM created_at)*1000)::bigint FROM couple_moments WHERE couple_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT $3`, couple.ID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CoupleMoment{}
	for rows.Next() {
		var m CoupleMoment
		var author string
		if err := rows.Scan(&m.ID, &author, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Author, err = r.GetSocialProfile(ctx, user, author)
		if err == nil {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}
func (r *PostgresRepository) ListCoupleEvents(ctx context.Context, user string, limit int) ([]CoupleEvent, error) {
	couple, err := r.GetCouple(ctx, user)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id,event_type,(extract(epoch FROM created_at)*1000)::bigint FROM couple_events WHERE couple_id=$1 ORDER BY id DESC LIMIT $2`, couple.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CoupleEvent{}
	for rows.Next() {
		var e CoupleEvent
		if err := rows.Scan(&e.ID, &e.Type, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
