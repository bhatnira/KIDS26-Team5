package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"antelope/internal/modules/log"
	"antelope/internal/modules/sse"
	"antelope/pkg/response"
	authsvc "antelope/services/auth"
	notifsvc "antelope/services/notification"
)

// NotificationHandler handles notification REST and SSE endpoints.
type NotificationHandler struct {
	svc    notifsvc.Service
	rdb    redis.UniversalClient
	sseMgr *sse.Manager
}

func NewNotificationHandler(svc notifsvc.Service, rdb redis.UniversalClient, sseMgr *sse.Manager) *NotificationHandler {
	return &NotificationHandler{svc: svc, rdb: rdb, sseMgr: sseMgr}
}

// List returns a paginated list of the authenticated user's notifications.
// @Summary      List notifications
// @Description  Returns a paginated list of the authenticated user's notifications.
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Page size (default 20, max 100)"
// @Success      200        {object}  map[string]interface{}
// @Failure      401        {object}  map[string]interface{}
// @Router       /notifications [get]
func (h *NotificationHandler) List(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	page, errPage := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, errSize := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 || pageSize <= 0 || errPage != nil || errSize != nil {
		response.CheckFail(c, nil, response.PageOrSizeError)
		return
	}
	if pageSize > 100 {
		response.CheckFail(c, nil, response.TooManyRequests)
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.ServerError(c, nil, err.Error())
		return
	}
	dtos := make([]notifsvc.NotificationDTO, len(items))
	for i, n := range items {
		dtos[i] = notifsvc.ToDTO(n)
	}
	c.JSON(http.StatusOK, gin.H{
		"code": response.SuccessCode,
		"data": gin.H{"items": dtos, "total": total},
		"msg":  response.OK,
	})
}

// MarkRead marks a single notification as read.
// @Summary Mark notification read
// @Description Mark notification read
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Notification ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /notifications/{id}/read [put]
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, uint(id)); err != nil {
		response.ServerError(c, nil, err.Error())
		return
	}
	response.Success(c, nil, response.OK)
}

// MarkAllRead marks all of the authenticated user's notifications as read.
// @Summary Mark all notifications read
// @Description Mark all notifications read
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /notifications/read-all [put]
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}
	if err := h.svc.MarkAllRead(c.Request.Context(), userID); err != nil {
		response.ServerError(c, nil, err.Error())
		return
	}
	response.Success(c, nil, response.OK)
}

// Stream opens an SSE connection managed by the SSE Manager.
// It delegates all streaming logic to NotificationSSESource, which is
// registered with the manager for proper graceful-shutdown handling.
// @Summary Notifications SSE stream
// @Description Notifications SSE stream
// @Tags notifications
// @Produce text/event-stream
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /notifications/stream [get]
func (h *NotificationHandler) Stream(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	if userID == 0 {
		response.Fail(c, nil, response.RequestError)
		return
	}

	sseW, err := sse.NewWriter(c.Writer)
	if err != nil {
		response.ServerError(c, nil, "streaming not supported")
		return
	}

	src := notifsvc.NewNotificationSSESource(h.svc, h.rdb, userID)
	if err := h.sseMgr.Serve(c.Request.Context(), sseW, src); err != nil {
		log.L().Debug("notification stream ended with error",
			zap.String("conn", src.ConnectionID()),
			zap.Error(err))
	}
}
