package app

import (
	"context"
	"errors"
)

type PlatformRepository interface {
	RegisterAccount(context.Context, AccountRecord, bool) (ApprovalRequest, error)
	CreateRoomWithApproval(context.Context, Room, RoomFeatures, bool) (ApprovalRequest, error)
	RequestRoomApproval(context.Context, string, string) (ApprovalRequest, error)
	ReadApproval(context.Context, string) (ApprovalRequest, error)
	FindApprovals(context.Context, string, string, string, AdminQuery) ([]ApprovalRequest, int, error)
	DecideApproval(context.Context, string, string, int64, bool, string) (ApprovalRequest, error)
}
type ApprovalRequest struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	RoomCode    string `json:"room_code,omitempty"`
	RoomName    string `json:"room_name,omitempty"`
	Status      string `json:"status"`
	Version     int64  `json:"version"`
	Reason      string `json:"reason"`
	CreatedAt   int64  `json:"created_at"`
	ReviewedAt  int64  `json:"reviewed_at"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
}
type ApprovalPendingError struct{ RequestID, Kind, RoomCode string }

func (e *ApprovalPendingError) Error() string {
	switch e.Kind {
	case "registration":
		return "注册申请已提交，请等待管理员审核后登录"
	case "room_creation":
		return "建房申请已提交，请在我的申请中查看进度"
	default:
		return "加入房间的申请已提交，请等待房主或管理员审核"
	}
}

var ErrReviewRejected = errors.New("申请未通过审核，请联系管理员")

func approvalPermission(actor User, request ApprovalRequest, room Room) bool {
	if !accountActive(actor) {
		return false
	}
	if permissionGranted(actor, "review.manage") {
		return true
	}
	return accountActive(actor) && request.Kind == "room_join" && room.OwnerID == actor.ID
}
