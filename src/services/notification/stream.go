package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/sse"
	"antelope/models"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NotificationSSESource implements sse.SSESource for per-user notification push.
// On connect it immediately sends the user's current notifications, then blocks
// on a Redis pub/sub channel. Each time the monitor publishes a signal it
// re-fetches from the DB and writes a fresh "notifications" SSE event.
type NotificationSSESource struct {
	svc    Service
	rdb    redis.UniversalClient
	userID uint
	connID string
}

// NewNotificationSSESource creates a source. The ConnectionID is fixed at
// creation time so the same key is used for the full lifetime of the stream.
func NewNotificationSSESource(svc Service, rdb redis.UniversalClient, userID uint) *NotificationSSESource {
	return &NotificationSSESource{
		svc:    svc,
		rdb:    rdb,
		userID: userID,
		connID: fmt.Sprintf("notify:%d-%d", userID, time.Now().UnixNano()),
	}
}

// ConnectionID satisfies sse.SSESource.
func (s *NotificationSSESource) ConnectionID() string {
	return s.connID
}

// Stream satisfies sse.SSESource. It sends the initial notification list then
// waits for Redis pub/sub signals to push updates. Returns nil on clean
// client disconnect (ctx canceled).
func (s *NotificationSSESource) Stream(ctx context.Context, w *sse.Writer) error {
	s.push(ctx, w)

	channel := fmt.Sprintf("notify:user:%d", s.userID)
	pubsub := s.rdb.Subscribe(ctx, channel)
	defer pubsub.Close()

	msgCh := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-msgCh:
			if !ok {
				return nil
			}
			s.push(ctx, w)
		}
	}
}

// push fetches the latest notifications and writes a single SSE event.
func (s *NotificationSSESource) push(ctx context.Context, w *sse.Writer) {
	items, _, err := s.svc.List(ctx, s.userID, 1, 50)
	if err != nil {
		log.L().Warn("notification stream: fetch failed",
			zap.String("conn", s.connID),
			zap.Error(err))
		return
	}
	dtos := toDTOs(items)
	b, _ := json.Marshal(dtos)
	if err := w.WriteEvent("notifications", string(b)); err != nil {
		log.L().Debug("notification stream: write error",
			zap.String("conn", s.connID),
			zap.Error(err))
	}
}

// ── DTO ───────────────────────────────────────────────────────────────────────

// NotificationDTO is the JSON shape sent over SSE and returned by the REST list.
type NotificationDTO struct {
	ID       uint   `json:"id"`
	Title    string `json:"title"`
	Desc     string `json:"description"`
	Type     int    `json:"type"`
	IsRead   bool   `json:"is_read"`
	TagTitle string `json:"tag_title"`
	TagType  string `json:"tag_type"`
	Icon     string `json:"icon"`
	Date     string `json:"date"`
}

func ToDTO(n models.Notification) NotificationDTO {
	return NotificationDTO{
		ID:       n.ID,
		Title:    n.Title,
		Desc:     n.Desc,
		Type:     n.Type,
		IsRead:   n.IsRead,
		TagTitle: n.TagTitle,
		TagType:  n.TagType,
		Icon:     n.Icon,
		Date:     n.CreatedAt.Format(time.DateTime),
	}
}

func toDTOs(items []models.Notification) []NotificationDTO {
	dtos := make([]NotificationDTO, len(items))
	for i, n := range items {
		dtos[i] = ToDTO(n)
	}
	return dtos
}
