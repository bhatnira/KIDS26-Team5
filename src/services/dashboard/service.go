package dashboard

import (
	"time"

	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DashboardStats holds admin-level statistics.
type DashboardStats struct {
	TotalUsers     int64 `json:"total_users"`
	ActiveUsers    int64 `json:"active_users"`
	TotalPipelines int64 `json:"total_pipelines"`
	ReadyPipelines int64 `json:"ready_pipelines"`
	TotalJobs      int64 `json:"total_jobs"`
	RecentJobs     int64 `json:"recent_jobs"`
	SuccessfulJobs int64 `json:"successful_jobs"`
	FailedJobs     int64 `json:"failed_jobs"`
	PendingJobs    int64 `json:"pending_jobs"`
	LocalUsers     int64 `json:"local_users"`
	LdapUsers      int64 `json:"ldap_users"`
	OidcUsers      int64 `json:"oidc_users"`
	JobsToday      int64 `json:"jobs_today"`
	JobsThisWeek   int64 `json:"jobs_this_week"`
	JobsThisMonth  int64 `json:"jobs_this_month"`

	PipelinesPending  int64 `json:"pipelines_pending"`
	PipelinesReady    int64 `json:"pipelines_ready"`
	PipelinesFailed   int64 `json:"pipelines_failed"`
	PipelinesDeleting int64 `json:"pipelines_deleting"`
}

// UserDashboardStats holds per-user statistics.
type UserDashboardStats struct {
	TotalPipelines int64 `json:"total_pipelines"`
	ReadyPipelines int64 `json:"ready_pipelines"`
	TotalJobs      int64 `json:"total_jobs"`
	SuccessfulJobs int64 `json:"successful_jobs"`
	FailedJobs     int64 `json:"failed_jobs"`
	PendingJobs    int64 `json:"pending_jobs"`
	JobsToday      int64 `json:"jobs_today"`
	JobsThisWeek   int64 `json:"jobs_this_week"`
	JobsThisMonth  int64 `json:"jobs_this_month"`

	PipelinesPending  int64 `json:"pipelines_pending"`
	PipelinesReady    int64 `json:"pipelines_ready"`
	PipelinesFailed   int64 `json:"pipelines_failed"`
	PipelinesDeleting int64 `json:"pipelines_deleting"`
}

// RecentJob is a summary row for the recent jobs list.
type RecentJob struct {
	ID              uint      `json:"id"`
	PipelineName    string    `json:"pipeline_name"`
	PipelineVersion string    `json:"pipeline_version"`
	UserEmail       string    `json:"user_email"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// Service provides dashboard statistics.
type Service interface {
	GetAdminStats() (gin.H, error)
	GetUserStats(userID uint) (gin.H, error)
}

type dashboardService struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return &dashboardService{db: db}
}

func (s *dashboardService) GetAdminStats() (gin.H, error) {
	stats := DashboardStats{}

	s.db.Model(&models.User{}).Count(&stats.TotalUsers)
	s.db.Model(&models.User{}).Where("status = ?", 1).Count(&stats.ActiveUsers)
	s.db.Model(&models.User{}).Where("auth_source = ?", models.AuthSourceLocal).Count(&stats.LocalUsers)
	s.db.Model(&models.User{}).Where("auth_source = ?", models.AuthSourceLDAP).Count(&stats.LdapUsers)
	s.db.Model(&models.User{}).Where("auth_source = ?", models.AuthSourceOIDC).Count(&stats.OidcUsers)

	s.db.Model(&models.Pipeline{}).Count(&stats.TotalPipelines)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "ready").Count(&stats.ReadyPipelines)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "pending").Count(&stats.PipelinesPending)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "ready").Count(&stats.PipelinesReady)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "failed").Count(&stats.PipelinesFailed)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "deleting").Count(&stats.PipelinesDeleting)

	s.db.Model(&models.Job{}).Count(&stats.TotalJobs)
	s.db.Model(&models.Job{}).Where("status = ?", "completed").Count(&stats.SuccessfulJobs)
	s.db.Model(&models.Job{}).Where("status = ?", "failed").Count(&stats.FailedJobs)
	s.db.Model(&models.Job{}).Where("status = ?", "submitted").Count(&stats.PendingJobs)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -7)
	monthStart := todayStart.AddDate(0, -1, 0)

	s.db.Model(&models.Job{}).Where("created_at >= ?", todayStart).Count(&stats.JobsToday)
	s.db.Model(&models.Job{}).Where("created_at >= ?", weekStart).Count(&stats.JobsThisWeek)
	s.db.Model(&models.Job{}).Where("created_at >= ?", monthStart).Count(&stats.JobsThisMonth)
	stats.RecentJobs = stats.JobsThisWeek

	var recentJobs []RecentJob
	if err := s.db.Model(&models.Job{}).
		Select("id, pipeline_name, pipeline_version, user_email, status, created_at").
		Order("created_at DESC").Limit(10).Scan(&recentJobs).Error; err != nil {
		log.L().Error("failed to get recent jobs", zap.Error(err))
	}

	return gin.H{"stats": stats, "recent_jobs": recentJobs, "is_admin": true}, nil
}

func (s *dashboardService) GetUserStats(userID uint) (gin.H, error) {
	if userID == 0 {
		return nil, apperr.Unauthorized(response.Unauthorized)
	}

	stats := UserDashboardStats{}

	s.db.Model(&models.Pipeline{}).Count(&stats.TotalPipelines)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "ready").Count(&stats.ReadyPipelines)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "pending").Count(&stats.PipelinesPending)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "ready").Count(&stats.PipelinesReady)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "failed").Count(&stats.PipelinesFailed)
	s.db.Model(&models.Pipeline{}).Where("status = ?", "deleting").Count(&stats.PipelinesDeleting)

	s.db.Model(&models.Job{}).Where("user_id = ?", userID).Count(&stats.TotalJobs)
	s.db.Model(&models.Job{}).Where("user_id = ? AND status = ?", userID, "completed").Count(&stats.SuccessfulJobs)
	s.db.Model(&models.Job{}).Where("user_id = ? AND status = ?", userID, "failed").Count(&stats.FailedJobs)
	s.db.Model(&models.Job{}).Where("user_id = ? AND status = ?", userID, "submitted").Count(&stats.PendingJobs)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -7)
	monthStart := todayStart.AddDate(0, -1, 0)

	s.db.Model(&models.Job{}).Where("user_id = ? AND created_at >= ?", userID, todayStart).Count(&stats.JobsToday)
	s.db.Model(&models.Job{}).Where("user_id = ? AND created_at >= ?", userID, weekStart).Count(&stats.JobsThisWeek)
	s.db.Model(&models.Job{}).Where("user_id = ? AND created_at >= ?", userID, monthStart).Count(&stats.JobsThisMonth)

	var recentJobs []RecentJob
	if err := s.db.Model(&models.Job{}).
		Where("user_id = ?", userID).
		Select("id, pipeline_name, pipeline_version, user_email, status, created_at").
		Order("created_at DESC").Limit(10).Scan(&recentJobs).Error; err != nil {
		log.L().Error("failed to get recent jobs for user", zap.Uint("userID", userID), zap.Error(err))
	}

	return gin.H{"stats": stats, "recent_jobs": recentJobs, "is_admin": false}, nil
}
