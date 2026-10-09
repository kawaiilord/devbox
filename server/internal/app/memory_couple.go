package app

import (
	"context"
	"errors"
	"sort"
	"time"
)

func (r *MemoryRepository) CreateCoupleRequest(_ context.Context, requester, recipient string) (CoupleRequest, error) {
	if requester == recipient {
		return CoupleRequest{}, errors.New("cannot bind self")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.usersByID[recipient]; !ok || memoryUsersBlocked(r.blocks, requester, recipient) {
		return CoupleRequest{}, ErrNotFound
	}
	for _, c := range r.couples {
		if (c.low == requester || c.high == requester || c.low == recipient || c.high == recipient) && c.status == "active" {
			return CoupleRequest{}, errors.New("user already has a couple")
		}
	}
	for _, q := range r.coupleRequests {
		if q.requester == requester && q.recipient == recipient && q.status == "pending" {
			return r.coupleRequestViewLocked(requester, q), nil
		}
	}
	q := memoryCoupleRequest{id: r.nextCoupleRequest, requester: requester, recipient: recipient, status: "pending", created: time.Now()}
	r.nextCoupleRequest++
	r.coupleRequests[q.id] = q
	return r.coupleRequestViewLocked(requester, q), nil
}
func (r *MemoryRepository) ListCoupleRequests(_ context.Context, userID string) ([]CoupleRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []CoupleRequest{}
	for _, q := range r.coupleRequests {
		if q.recipient == userID && q.status == "pending" {
			out = append(out, r.coupleRequestViewLocked(userID, q))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}
func (r *MemoryRepository) RespondCoupleRequest(_ context.Context, userID string, id int64, accept bool) (Couple, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.coupleRequests[id]
	if !ok || q.recipient != userID || q.status != "pending" {
		return Couple{}, ErrNotFound
	}
	if !accept {
		q.status = "rejected"
		r.coupleRequests[id] = q
		return Couple{}, nil
	}
	if r.usersByID[q.requester].VIPExpiresAt <= time.Now().UnixMilli() {
		return Couple{}, ErrForbidden
	}
	for _, c := range r.couples {
		if c.status == "active" && (c.low == q.requester || c.high == q.requester || c.low == q.recipient || c.high == q.recipient) {
			return Couple{}, errors.New("user already has a couple")
		}
	}
	low, high := socialPair(q.requester, q.recipient)
	now := time.Now()
	var c memoryCouple
	for cid, existing := range r.couples {
		if existing.low == low && existing.high == high {
			c = existing
			c.id = cid
		}
	}
	if c.id == 0 {
		c = memoryCouple{id: r.nextCouple, low: low, high: high}
		r.nextCouple++
	}
	c.status = "active"
	c.bound = now
	c.separated = time.Time{}
	c.cooling = time.Time{}
	r.couples[c.id] = c
	q.status = "accepted"
	r.coupleRequests[id] = q
	r.addCoupleEventLocked(c.id, "bound", now)
	return r.coupleViewLocked(userID, c), nil
}
func (r *MemoryRepository) GetCouple(_ context.Context, userID string) (Couple, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok {
		return Couple{}, ErrNotFound
	}
	return r.coupleViewLocked(userID, c), nil
}
func (r *MemoryRepository) SeparateCouple(_ context.Context, userID string) (Couple, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok || c.status != "active" {
		return Couple{}, ErrNotFound
	}
	now := time.Now()
	c.status = "separated"
	c.separated = now
	c.cooling = now.Add(7 * 24 * time.Hour)
	r.couples[c.id] = c
	r.addCoupleEventLocked(c.id, "separated", now)
	return r.coupleViewLocked(userID, c), nil
}
func (r *MemoryRepository) RestoreCouple(_ context.Context, userID string) (Couple, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok || c.status != "separated" || time.Now().After(c.cooling) {
		return Couple{}, ErrForbidden
	}
	now := time.Now()
	c.status = "active"
	c.separated = time.Time{}
	c.cooling = time.Time{}
	r.couples[c.id] = c
	r.addCoupleEventLocked(c.id, "restored", now)
	return r.coupleViewLocked(userID, c), nil
}
func (r *MemoryRepository) AddCoupleMoment(_ context.Context, userID, body string) (CoupleMoment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok || c.status != "active" {
		return CoupleMoment{}, ErrForbidden
	}
	m := memoryCoupleMoment{id: r.nextCoupleMoment, author: userID, body: body, created: time.Now()}
	r.nextCoupleMoment++
	r.coupleMoments[c.id] = append(r.coupleMoments[c.id], m)
	r.addCoupleEventLocked(c.id, "moment", m.created)
	return r.coupleMomentViewLocked(userID, m), nil
}
func (r *MemoryRepository) ListCoupleMoments(_ context.Context, userID string, before int64, limit int) ([]CoupleMoment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok {
		return nil, ErrNotFound
	}
	items := r.coupleMoments[c.id]
	out := []CoupleMoment{}
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		if before == 0 || items[i].id < before {
			out = append(out, r.coupleMomentViewLocked(userID, items[i]))
		}
	}
	return out, nil
}
func (r *MemoryRepository) ListCoupleEvents(_ context.Context, userID string, limit int) ([]CoupleEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.findCoupleLocked(userID)
	if !ok {
		return nil, ErrNotFound
	}
	items := r.coupleEvents[c.id]
	out := []CoupleEvent{}
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, items[i])
	}
	return out, nil
}
func (r *MemoryRepository) findCoupleLocked(user string) (memoryCouple, bool) {
	for _, c := range r.couples {
		if (c.low == user || c.high == user) && c.status != "ended" {
			return c, true
		}
	}
	return memoryCouple{}, false
}
func (r *MemoryRepository) coupleViewLocked(user string, c memoryCouple) Couple {
	peer := c.low
	if peer == user {
		peer = c.high
	}
	return Couple{ID: c.id, Partner: r.socialProfileLocked(user, peer), Status: c.status, BoundAt: c.bound.UnixMilli(), SeparatedAt: millisOrZero(c.separated), CoolingPeriodEnd: millisOrZero(c.cooling)}
}
func (r *MemoryRepository) coupleRequestViewLocked(user string, q memoryCoupleRequest) CoupleRequest {
	return CoupleRequest{ID: q.id, Requester: r.socialProfileLocked(user, q.requester), RecipientID: q.recipient, Status: q.status, CreatedAt: q.created.UnixMilli()}
}
func (r *MemoryRepository) coupleMomentViewLocked(user string, m memoryCoupleMoment) CoupleMoment {
	return CoupleMoment{ID: m.id, Author: r.socialProfileLocked(user, m.author), Body: m.body, CreatedAt: m.created.UnixMilli()}
}
func (r *MemoryRepository) addCoupleEventLocked(id int64, kind string, at time.Time) {
	event := CoupleEvent{ID: r.nextCoupleEvent, Type: kind, CreatedAt: at.UnixMilli()}
	r.nextCoupleEvent++
	r.coupleEvents[id] = append(r.coupleEvents[id], event)
}
func millisOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}
