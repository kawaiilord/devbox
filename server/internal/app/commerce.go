package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var defaultVIPBenefits = []VIPBenefit{
	{Title: "长期房间", Description: "会员房间在会员有效期内持续可用", Icon: "meeting_room"},
	{Title: "情侣空间", Description: "绑定伴侣并共享观影记忆", Icon: "favorite"},
	{Title: "多端同步", Description: "同步收藏、历史与资源配置", Icon: "sync"},
	{Title: "语音聊天", Description: "在房间内使用实时语音", Icon: "mic"},
}

func (s *Server) vipInfo(w http.ResponseWriter, r *http.Request) {
	plans, err := s.repo.ListVIPPlans(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	method := "disabled"
	if s.options.Payment != nil {
		method = s.options.Payment.Name()
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: VIPInfo{Announcement: s.options.VIPAnnouncement, PaymentMethod: method, Benefits: defaultVIPBenefits, Plans: plans}, Msg: "ok"})
}

func (s *Server) membershipStatus(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	account, err := s.repo.UserByID(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"active": account.VIPExpiresAt > time.Now().UnixMilli(), "vip_expires_at": account.VIPExpiresAt}, Msg: "ok"})
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	if s.options.Payment == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("payment unavailable"))
		return
	}
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		PlanID string `json:"plan_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := s.repo.GetVIPPlan(r.Context(), strings.TrimSpace(request.PlanID))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	now := time.Now()
	order := Order{OrderNo: "SF" + strings.ToUpper(mustRandomString(12)), UserID: user.ID, PlanID: plan.ID, PlanTitle: plan.Title, AmountMinor: plan.PriceMinor, Currency: "CNY", Status: "pending", PayChannel: s.options.Payment.Name(), QRExpiresAt: now.Add(3 * time.Minute).UnixMilli(), CreatedAt: now.UnixMilli()}
	intent, err := s.options.Payment.CreateIntent(r.Context(), order)
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("could not create payment"))
		return
	}
	order.CheckoutURL, order.QRExpiresAt = intent.CheckoutURL, intent.ExpiresAt
	order, err = s.repo.CreateOrder(r.Context(), order)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("could not create order"))
		return
	}
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: order, Msg: "created"})
}

func (s *Server) queryOrder(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	order, err := s.repo.GetOrder(r.Context(), user.ID, r.PathValue("orderNo"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: order, Msg: "ok"})
}

func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	orders, err := s.repo.ListUserOrders(r.Context(), user.ID, before, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"orders": orders}, Msg: "ok"})
}

func (s *Server) paymentCallback(w http.ResponseWriter, r *http.Request) {
	if s.options.Payment == nil {
		writeError(w, http.StatusNotFound, ErrNotFound)
		return
	}
	var request struct {
		OrderNo     string `json:"order_no"`
		TradeNo     string `json:"trade_no"`
		AmountMinor int64  `json:"amount_minor"`
		Timestamp   int64  `json:"timestamp"`
		Signature   string `json:"signature"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.options.Payment.VerifyCallback(request.OrderNo, request.TradeNo, request.AmountMinor, request.Timestamp, request.Signature) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid payment signature"))
		return
	}
	order, activated, err := s.repo.ActivatePaidOrder(r.Context(), request.OrderNo, s.options.Payment.Name(), request.TradeNo, request.AmountMinor)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"order": order, "activated": activated}, Msg: "ok"})
}

func (s *Server) redeemActivationCode(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hash, ok := activationCodeHash(request.Code)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("invalid activation code"))
		return
	}
	expires, err := s.repo.RedeemActivationCode(r.Context(), user.ID, hash)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int64{"vip_expires_at": expires}, Msg: "activated"})
}

func (s *Server) checkInStatus(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	status, err := s.repo.GetCheckInStatus(r.Context(), user.ID, utcDate(time.Now()), s.options.PointsPerCheckIn, s.options.PointsPerVIPDay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: status, Msg: "ok"})
}

func (s *Server) dailyCheckIn(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	status, err := s.repo.DailyCheckIn(r.Context(), user.ID, utcDate(time.Now()), s.options.PointsPerCheckIn)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	status.PointsPerVIPDay = s.options.PointsPerVIPDay
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: status, Msg: "checked in"})
}

func (s *Server) redeemPoints(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var request struct {
		Days int `json:"days"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	expires, err := s.repo.RedeemPointsForVIP(r.Context(), user.ID, request.Days, s.options.PointsPerVIPDay)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int64{"vip_expires_at": expires}, Msg: "redeemed"})
}

func (s *Server) pointsTransactions(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromRequest(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	items, err := s.repo.ListPointsTransactions(r.Context(), user.ID, before, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"transactions": items}, Msg: "ok"})
}

func (s *Server) pointsLeaderboard(w http.ResponseWriter, r *http.Request) {
	date := ""
	if r.URL.Query().Get("today") == "true" {
		date = utcDate(time.Now())
	}
	items, err := s.repo.PointsLeaderboard(r.Context(), date, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"entries": items}, Msg: "ok"})
}

func (s *Server) adminCreateActivationCodes(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var request struct {
		PlanID       string `json:"plan_id"`
		DurationDays int    `json:"duration_days"`
		Count        int    `json:"count"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Count < 1 || request.Count > 100 || (request.PlanID == "" && (request.DurationDays < 1 || request.DurationDays > 3650)) {
		writeError(w, http.StatusBadRequest, errors.New("invalid activation code batch"))
		return
	}
	if request.PlanID != "" {
		if _, err := s.repo.GetVIPPlan(r.Context(), request.PlanID); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	batch := "B" + strings.ToUpper(mustRandomString(10))
	stored := make([]ActivationCode, 0, request.Count)
	issued := make([]IssuedActivationCode, 0, request.Count)
	for i := 0; i < request.Count; i++ {
		plain, err := randomActivationCode(20)
		if err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("could not generate activation codes"))
			return
		}
		hash, _ := activationCodeHash(plain)
		stored = append(stored, ActivationCode{CodeHash: hash, BatchID: batch, PlanID: request.PlanID, DurationDays: request.DurationDays})
		issued = append(issued, IssuedActivationCode{Code: plain, BatchID: batch, PlanID: request.PlanID, DurationDays: request.DurationDays})
	}
	if err := s.repo.StoreActivationCodes(r.Context(), stored); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, _ = s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: admin.ID, Action: "activation_codes.create", TargetType: "activation_code_batch", TargetID: batch, Metadata: map[string]any{"count": request.Count, "plan_id": request.PlanID, "duration_days": request.DurationDays}})
	writeJSON(w, http.StatusCreated, apiResponse{Code: 0, Data: map[string]any{"batch_id": batch, "codes": issued}, Msg: "created"})
}

func (s *Server) adminSetVIP(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	userID := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ExpiresAt int64 `json:"expires_at"`
		Days      int   `json:"days"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	expires := request.ExpiresAt
	if request.Days != 0 {
		if request.Days < 1 || request.Days > 36500 || request.ExpiresAt != 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid VIP adjustment"))
			return
		}
		account, err := s.repo.UserByID(r.Context(), userID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		expires = extendedVIPExpiry(account.VIPExpiresAt, request.Days, false, time.Now())
	} else if expires != 0 && expires <= time.Now().UnixMilli() {
		writeError(w, http.StatusBadRequest, errors.New("VIP expiry must be in the future or zero"))
		return
	}
	if err := s.repo.SetUserVIP(r.Context(), userID, expires); err != nil {
		writeStoreError(w, err)
		return
	}
	_, _ = s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: admin.ID, Action: "user.vip.update", TargetType: "user", TargetID: userID, Metadata: map[string]any{"expires_at": expires, "days": request.Days}})
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]int64{"vip_expires_at": expires}, Msg: "updated"})
}

func (s *Server) adminActivateOrder(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	orderNo := strings.TrimSpace(r.PathValue("orderNo"))
	order, err := s.repo.GetOrder(r.Context(), "", orderNo)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if order.Status == "activated" {
		writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"order": order, "activated": false}, Msg: "already activated"})
		return
	}
	tradeNo := "manual-" + strings.ToLower(mustRandomString(16))
	order, activated, err := s.repo.ActivatePaidOrder(r.Context(), orderNo, "manual", tradeNo, order.AmountMinor)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_, _ = s.repo.AppendAudit(r.Context(), AuditEvent{ActorID: admin.ID, Action: "order.manual_activate", TargetType: "order", TargetID: orderNo, Metadata: map[string]any{"activated": activated, "trade_no": tradeNo}})
	writeJSON(w, http.StatusOK, apiResponse{Code: 0, Data: map[string]any{"order": order, "activated": activated}, Msg: "activated"})
}

func activationCodeHash(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(value, "-", "")))
	if len(value) < 12 || len(value) > 64 {
		return "", false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
			return "", false
		}
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), true
}

func randomActivationCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, length)
	bound := big.NewInt(int64(len(alphabet)))
	for i := range raw {
		index, err := rand.Int(rand.Reader, bound)
		if err != nil {
			return "", err
		}
		raw[i] = alphabet[index.Int64()]
	}
	return string(raw), nil
}

func utcDate(now time.Time) string { return now.UTC().Format("2006-01-02") }
