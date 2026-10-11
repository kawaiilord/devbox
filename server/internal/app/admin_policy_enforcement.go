package app

import (
	"context"
	"errors"
	"net/http"
)

func (s *Server) validateLiveUser(ctx context.Context, user User) error {
	account, err := s.repo.UserByID(ctx, user.ID)
	if err != nil || !accountActive(account.User) {
		return ErrUnauthorized
	}
	if user.SessionVersion > 0 && user.SessionVersion != account.SessionVersion {
		return ErrUnauthorized
	}
	return nil
}
func (s *Server) withRoomCreationPolicy(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.userFromRequest(r)
		if err != nil {
			writeError(w, 401, err)
			return
		}
		handled := false
		err = s.withNamedMutation(r.Context(), "create-user:"+user.ID, func(ctx context.Context) error {
			p, _, err := s.sitePolicy(ctx)
			if err != nil {
				return err
			}
			if !p.RoomCreationEnabled && !permissionGranted(user, "room.manage") {
				return errors.New("站点当前暂停创建房间")
			}
			if p.MaxRoomsPerUser > 0 {
				_, total, err := s.repo.AdminFindRooms(ctx, AdminQuery{OwnerID: user.ID, Status: "active", Limit: 1})
				if err != nil {
					return err
				}
				_, pending, err := s.repo.FindApprovals(ctx, user.ID, "mine", "", AdminQuery{Kind: "room_creation", Status: "pending", Limit: 1})
				if err != nil {
					return err
				}
				if total+pending >= p.MaxRoomsPerUser {
					return errors.New("已达到本站允许的个人房间数量上限")
				}
			}
			handled = true
			next(w, r.WithContext(ctx))
			return nil
		})
		if err != nil && !handled {
			writeError(w, 403, err)
		}
	}
}
