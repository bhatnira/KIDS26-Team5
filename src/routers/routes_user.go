package routers

import (
	v1 "antelope/routers/api/v1"

	"github.com/gin-gonic/gin"
)

func (rm *RouterManager) registerUserRoutes(
	pub, priv *gin.RouterGroup,
	userH *v1.UserHandler,
	jobH *v1.JobHandler,
	dashH *v1.DashboardHandler,
	setRoleMW gin.HandlerFunc,
) {
	pub.POST("/user/register", userH.Register)
	pub.POST("/user/register/code", userH.SendRegisterCode)
	pub.POST("/user/reset", userH.ResetPassword)
	pub.POST("/user/reset/code", userH.SendResetCode)

	userPriv := priv.Group("/user")
	userPriv.Use(setRoleMW)
	{
		userPriv.GET("/jobs", jobH.GetUserJobs)
		userPriv.GET("/job/log", jobH.StreamUserJobLog)
		userPriv.GET("/dashboard/stats", dashH.GetUserStats)
		userPriv.GET("/profile", userH.Profile)
		userPriv.PUT("/profile", userH.UpdateProfile)
		userPriv.POST("/password", userH.ChangePassword)
	}
}

func (rm *RouterManager) registerJobRoutes(priv *gin.RouterGroup, h *v1.JobHandler) {
	priv.POST("/job/add", h.AddJob)
	priv.DELETE("/job/delete", h.DeleteJob)
	priv.POST("/job/stop", h.StopJob)
	priv.GET("/job/details", h.GetJobDetails)
}
