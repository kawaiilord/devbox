package app

import (
	"context"
	"errors"
	"sort"
	"time"
)

func (r *MemoryRepository) CreateAccountDeletionRequest(_ context.Context, userID, reason string) (AccountDeletionRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.usersByID[userID]
	if !ok {
		return AccountDeletionRequest{}, ErrNotFound
	}
	if account.AdminRole != "" {
		return AccountDeletionRequest{}, ErrForbidden
	}
	for _, item := range r.deletionRequests {
		if item.UserID == userID && item.Status == "pending" {
			return AccountDeletionRequest{}, errors.New("deletion request already pending")
		}
	}
	item := AccountDeletionRequest{ID: r.nextDeletionRequest, UserID: userID, Reason: reason, Status: "pending", RequestedAt: time.Now().UnixMilli()}
	r.nextDeletionRequest++
	r.deletionRequests = append(r.deletionRequests, item)
	return item, nil
}
func (r *MemoryRepository) GetAccountDeletionRequest(_ context.Context, userID string) (AccountDeletionRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := len(r.deletionRequests) - 1; i >= 0; i-- {
		if r.deletionRequests[i].UserID == userID {
			return r.deletionRequests[i], nil
		}
	}
	return AccountDeletionRequest{}, ErrNotFound
}
func (r *MemoryRepository) CancelAccountDeletionRequest(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.deletionRequests {
		if r.deletionRequests[i].UserID == userID && r.deletionRequests[i].Status == "pending" {
			r.deletionRequests[i].Status = "cancelled"
			return nil
		}
	}
	return ErrNotFound
}
func (r *MemoryRepository) ListAccountDeletionRequests(_ context.Context, status string, before int64, limit int) ([]AccountDeletionRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []AccountDeletionRequest{}
	for i := len(r.deletionRequests) - 1; i >= 0 && len(out) < limit; i-- {
		v := r.deletionRequests[i]
		if (status == "" || v.Status == status) && (before == 0 || v.ID < before) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *MemoryRepository) ResolveAccountDeletionRequest(_ context.Context, id int64, actor string, approve bool, resolution string) (AccountDeletionRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.deletionRequests {
		item := &r.deletionRequests[i]
		if item.ID != id || item.Status != "pending" {
			continue
		}
		item.ReviewedBy = actor
		item.ReviewedAt = time.Now().UnixMilli()
		item.Resolution = resolution
		if !approve {
			item.Status = "rejected"
			return *item, nil
		}
		account, ok := r.usersByID[item.UserID]
		if !ok {
			return AccountDeletionRequest{}, ErrNotFound
		}
		if account.AdminRole != "" {
			return AccountDeletionRequest{}, ErrForbidden
		}
		for key, order := range r.orders {
			if order.UserID == item.UserID {
				order.UserID = ""
				r.orders[key] = order
			}
		}
		delete(r.usersByMail, account.Email)
		delete(r.usersByID, item.UserID)
		delete(r.userDevices, item.UserID)
		delete(r.privacy, item.UserID)
		item.Status = "approved"
		item.ExecutedAt = time.Now().UnixMilli()
		return *item, nil
	}
	return AccountDeletionRequest{}, ErrNotFound
}

func (r *MemoryRepository) CreateCopyrightComplaint(_ context.Context, item CopyrightComplaint) (CopyrightComplaint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item.ID = r.nextCopyrightComplaint
	r.nextCopyrightComplaint++
	item.Status = "submitted"
	item.SubmittedAt = time.Now().UnixMilli()
	item.DueAt = time.Now().Add(24 * time.Hour).UnixMilli()
	item.Evidence = append([]string(nil), item.Evidence...)
	r.copyrightComplaints = append(r.copyrightComplaints, item)
	return item, nil
}
func (r *MemoryRepository) ListCopyrightComplaints(_ context.Context, status string, before int64, limit int) ([]CopyrightComplaint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []CopyrightComplaint{}
	for i := len(r.copyrightComplaints) - 1; i >= 0 && len(out) < limit; i-- {
		v := r.copyrightComplaints[i]
		if (status == "" || v.Status == status) && (before == 0 || v.ID < before) {
			v.Evidence = append([]string(nil), v.Evidence...)
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueAt < out[j].DueAt })
	return out, nil
}
func (r *MemoryRepository) ResolveCopyrightComplaint(_ context.Context, id int64, actor, status, resolution string) (CopyrightComplaint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.copyrightComplaints {
		item := &r.copyrightComplaints[i]
		if item.ID == id && (item.Status == "submitted" || item.Status == "triaged") {
			item.Status = status
			item.Resolution = resolution
			item.ReviewedBy = actor
			item.ReviewedAt = time.Now().UnixMilli()
			r.appendAuditLocked(AuditEvent{ActorID: actor, Action: "copyright_complaint.resolve", TargetType: "copyright_complaint", TargetID: strconvFormatCompat(id)})
			return *item, nil
		}
	}
	return CopyrightComplaint{}, ErrNotFound
}
func strconvFormatCompat(value int64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	out := make([]byte, 0, 20)
	for value > 0 {
		out = append(out, digits[value%10])
		value /= 10
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}
