package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

func writeApprovalPending(w http.ResponseWriter, err error) bool {
	var pending *ApprovalPendingError
	if !errors.As(err, &pending) {
		return false
	}
	writeJSON(w, 202, apiResponse{Code: 202, Msg: pending.Error(), Data: map[string]any{"pending_approval": true, "request_id": pending.RequestID, "kind": pending.Kind, "room_code": pending.RoomCode}})
	return true
}
func (r *MemoryRepository) ReadApproval(_ context.Context, id string) (ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.approvals[id]
	if !ok {
		return item, ErrNotFound
	}
	return item, nil
}
func (r *PostgresRepository) ReadApproval(ctx context.Context, id string) (ApprovalRequest, error) {
	return scanApproval(r.pool.QueryRow(ctx, `SELECT `+approvalColumns+approvalFrom+` WHERE a.id=$1`, id))
}
func (s *Server) approvalList(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	scope := "mine"
	code := strings.ToUpper(r.PathValue("code"))
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
		scope = "admin"
	} else if code != "" {
		scope = "room"
	}
	q := adminQuery(r)
	items, total, err := s.repo.FindApprovals(r.Context(), user.ID, scope, code, q)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"requests": items, "total": total, "offset": q.Offset, "limit": q.Limit, "has_more": q.Offset+len(items) < total}})
}
func (s *Server) approvalResolve(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var in struct {
		Version int64  `json:"version"`
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err = decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Version < 1 || !safeModerationText(in.Reason, 500, in.Approve) {
		writeError(w, 400, errors.New("请填写有效的审核说明"))
		return
	}
	item, err := s.repo.ReadApproval(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	resolve := func(ctx context.Context) error {
		var err error
		item, err = s.repo.DecideApproval(ctx, item.ID, user.ID, in.Version, in.Approve, strings.TrimSpace(in.Reason))
		if err != nil {
			return err
		}
		if item.RoomCode != "" {
			record, err := s.repo.AdminReadRoom(ctx, item.RoomCode)
			if err != nil {
				return err
			}
			return s.publishManagedRoom(ctx, record.Room)
		}
		return nil
	}
	if item.RoomCode != "" {
		err = s.withRoomMutation(r.Context(), item.RoomCode, resolve)
	} else {
		err = resolve(r.Context())
	}
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Data: item})
}
