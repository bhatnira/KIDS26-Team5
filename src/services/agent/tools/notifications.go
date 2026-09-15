package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"antelope/internal/modules/agent"
	"antelope/models"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// NotificationLister is the narrow contract the list_notifications tool
// needs from the notification service (mirrors GET /notification).
type NotificationLister interface {
	List(ctx context.Context, userID uint, page, pageSize int) ([]models.Notification, int64, error)
}

type listNotificationsTool struct {
	notifs NotificationLister
}

// NewListNotificationsTool returns a tool that lists the requesting user's
// platform notifications (job completion alerts and similar), newest first.
func NewListNotificationsTool(notifs NotificationLister) tool.CallableTool {
	return &listNotificationsTool{notifs: notifs}
}

func (t *listNotificationsTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "list_notifications",
		Description: "List the current user's platform notifications (e.g. job completion/failure alerts), newest first.",
		InputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"page":      {Type: "integer", Description: "Page number (default 1)."},
				"page_size": {Type: "integer", Description: "Notifications per page (default 10, max 50)."},
			},
		},
	}
}

type listNotificationsInput struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`
}

type notificationInfo struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Desc      string    `json:"desc,omitempty"`
	IsRead    bool      `json:"is_read"`
	TagTitle  string    `json:"tag_title,omitempty"`
	TagType   string    `json:"tag_type,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (t *listNotificationsTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	userID, ok := agent.UserIDFromContext(ctx)
	if !ok {
		return nil, errors.New("user id missing from context")
	}
	var in listNotificationsInput
	if len(jsonArgs) > 0 {
		if err := json.Unmarshal(jsonArgs, &in); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}

	items, total, err := t.notifs.List(ctx, userID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	out := make([]notificationInfo, 0, len(items))
	for _, n := range items {
		out = append(out, notificationInfo{
			ID:        n.ID,
			Title:     n.Title,
			Desc:      n.Desc,
			IsRead:    n.IsRead,
			TagTitle:  n.TagTitle,
			TagType:   n.TagType,
			CreatedAt: n.CreatedAt,
		})
	}
	return map[string]any{"items": out, "total": total, "page": page}, nil
}
