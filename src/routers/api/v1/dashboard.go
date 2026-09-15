package v1

import (
	"github.com/gin-gonic/gin"

	"antelope/pkg/response"
	authsvc "antelope/services/auth"
	dashsvc "antelope/services/dashboard"
)

// DashboardHandler handles dashboard statistics routes.
type DashboardHandler struct {
	svc dashsvc.Service
}

func NewDashboardHandler(svc dashsvc.Service) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// @Summary Admin dashboard stats
// @Tags dashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /admin/dashboard/stats [get]
func (h *DashboardHandler) GetAdminStats(c *gin.Context) {
	data, err := h.svc.GetAdminStats()
	response.Render(c, data, err)
}

// @Summary Current user dashboard stats
// @Tags dashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/dashboard/stats [get]
func (h *DashboardHandler) GetUserStats(c *gin.Context) {
	userID := authsvc.GetUserID(c)
	data, err := h.svc.GetUserStats(userID)
	response.Render(c, data, err)
}
