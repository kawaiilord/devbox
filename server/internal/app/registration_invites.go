package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrRegistrationInvite = errors.New("邀请码无效、已使用、已撤销或已过期，请联系管理员获取")

type RegistrationInvite struct {
	ID        string `json:"id"`
	CodeHash  string `json:"-"`
	Hint      string `json:"hint"`
	BatchID   string `json:"batch_id"`
	Label     string `json:"label"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UsedBy    string `json:"used_by,omitempty"`
	UsedAt    int64  `json:"used_at"`
	RevokedAt int64  `json:"revoked_at"`
	Status    string `json:"status"`
}

type RegistrationInviteRepository interface {
	StoreRegistrationInvites(context.Context, string, []RegistrationInvite) error
	FindRegistrationInvites(context.Context, AdminQuery) ([]RegistrationInvite, int, error)
	RevokeRegistrationInvite(context.Context, string, string) error
}

func registrationInviteHash(value string) string {
	code := strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(value)))
	if len(code) != 24 || !strings.HasPrefix(code, "JOIN") {
		return ""
	}
	for _, char := range code[4:] {
		if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", char) {
			return ""
		}
	}
	sum := sha256.Sum256([]byte("sameframe-registration-invite:" + code))
	return hex.EncodeToString(sum[:])
}

func inviteStatus(code RegistrationInvite, now int64) string {
	if code.UsedAt > 0 {
		return "used"
	}
	if code.RevokedAt > 0 {
		return "revoked"
	}
	if code.ExpiresAt > 0 && code.ExpiresAt <= now {
		return "expired"
	}
	return "active"
}

func (s *Server) adminRegistrationInvites(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		q := adminQuery(r)
		if q.Status != "" && q.Status != "active" && q.Status != "used" && q.Status != "expired" && q.Status != "revoked" {
			writeError(w, 400, errors.New("邀请码状态无效"))
			return
		}
		codes, total, err := s.repo.FindRegistrationInvites(r.Context(), q)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		writeJSON(w, 200, apiResponse{Code: 0, Data: map[string]any{"codes": codes, "total": total, "offset": q.Offset, "limit": q.Limit, "has_more": q.Offset+len(codes) < total}})
		return
	}
	var in struct {
		Count       int    `json:"count"`
		ExpiresDays int    `json:"expires_days"`
		Label       string `json:"label"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Count < 1 || in.Count > 100 || in.ExpiresDays < 0 || in.ExpiresDays > 365 || len([]rune(in.Label)) > 80 {
		writeError(w, 400, errors.New("每批 1–100 个，有效期 0–365 天，批次备注最多 80 字"))
		return
	}
	if !s.enforceRateLimit(w, r, "registration-invite-batch", actor.ID, 10, time.Minute) {
		return
	}
	now := time.Now()
	batch := mustRandomString(12)
	expiry := int64(0)
	if in.ExpiresDays > 0 {
		expiry = now.Add(time.Duration(in.ExpiresDays) * 24 * time.Hour).UnixMilli()
	}
	codes := make([]RegistrationInvite, 0, in.Count)
	issued := make([]map[string]any, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		raw, err := randomActivationCode(20)
		if err != nil {
			writeError(w, 500, errors.New("邀请码生成失败"))
			return
		}
		plain := "JOIN-" + raw[:5] + "-" + raw[5:10] + "-" + raw[10:15] + "-" + raw[15:]
		code := RegistrationInvite{ID: mustRandomString(12), CodeHash: registrationInviteHash(plain), Hint: "JOIN-…-" + raw[16:], BatchID: batch, Label: in.Label, CreatedBy: actor.ID, CreatedAt: now.UnixMilli(), ExpiresAt: expiry}
		codes = append(codes, code)
		issued = append(issued, map[string]any{"id": code.ID, "code": plain, "expires_at": expiry})
	}
	if err := s.repo.StoreRegistrationInvites(r.Context(), actor.ID, codes); err != nil {
		writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, apiResponse{Code: 0, Data: map[string]any{"batch_id": batch, "label": in.Label, "codes": issued}, Msg: "邀请码已生成，明文仅显示一次，请及时保存"})
}

func (s *Server) adminRevokeRegistrationInvite(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, "config.write")
	if !ok {
		return
	}
	if err := s.repo.RevokeRegistrationInvite(r.Context(), actor.ID, r.PathValue("id")); err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, apiResponse{Code: 0, Msg: "邀请码已撤销"})
}
