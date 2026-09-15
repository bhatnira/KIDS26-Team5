package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/runner"
	"antelope/internal/modules/setting"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/types"

	"github.com/gin-gonic/gin"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// NomadJobsAPI is the narrow Nomad interface needed by the pipeline service.
type NomadJobsAPI interface {
	ParseHCL(jobHCL string, canonicalize bool) (*nomad.Job, error)
	Register(job *nomad.Job, q *nomad.WriteOptions) (*nomad.JobRegisterResponse, *nomad.WriteMeta, error)
	Deregister(jobID string, purge bool, q *nomad.WriteOptions) (string, *nomad.WriteMeta, error)
}

// Service provides pipeline management operations.
type Service interface {
	GetList(page, pageSize int) (gin.H, error)
	GetAll() (gin.H, error)
	Add(dto types.PipelineAddDto) error
	Delete(dto types.PipelineDeleteDto) error
	Update(dto types.PipelineAddDto) error
	GetSchema(repository, version string) (gin.H, error)
	// ReconcileStalePending resolves pipelines left mid-registration by a pod
	// that died. Intended to run once at startup.
	ReconcileStalePending(olderThan time.Duration) (int64, error)
}

type pipelineService struct {
	db        *gorm.DB
	redis     redis.UniversalClient
	nomadJobs NomadJobsAPI
	nomadCfg  setting.NomadConfig
}

func NewService(db *gorm.DB, rdb redis.UniversalClient, nomadJobs NomadJobsAPI, nomadCfg setting.NomadConfig) Service {
	return &pipelineService{db: db, redis: rdb, nomadJobs: nomadJobs, nomadCfg: nomadCfg}
}

func (s *pipelineService) GetList(page, pageSize int) (gin.H, error) {
	var pipelines []types.PipelineListDto

	if page > 0 && pageSize > 0 {
		var total int64
		// BUG-3/CONS-2: check Count error
		if err := s.db.Model(&models.Pipeline{}).Count(&total).Error; err != nil {
			log.L().Error("count pipelines failed", zap.Error(err))
			return nil, apperr.ServerError(err.Error())
		}
		if err := s.db.Limit(pageSize).Offset((page - 1) * pageSize).
			Model(&models.Pipeline{}).
			Select("id,name,version,status,repository,author,description").
			Order("id ASC").
			Scan(&pipelines).Error; err != nil {
			log.L().Error("get pipeline list with pagination failed", zap.Error(err))
			return nil, apperr.ServerError(err.Error())
		}
		return gin.H{
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": int(math.Ceil(float64(total) / float64(pageSize))),
			},
			"pipelines": pipelines,
		}, nil
	}

	if err := s.db.Model(&models.Pipeline{}).
		Select("id,name,version,status,repository,author,description").
		Order("id ASC").
		Scan(&pipelines).Error; err != nil {
		log.L().Error("get pipeline list failed", zap.Error(err))
		return nil, apperr.ServerError(err.Error())
	}
	return gin.H{"pipelines": pipelines}, nil
}

func (s *pipelineService) GetAll() (gin.H, error) {
	return s.GetList(0, 0)
}

func (s *pipelineService) Add(dto types.PipelineAddDto) error {
	// BUG-2: isPipelineExist now returns (pipeline, found, error)
	_, found, err := isPipelineExist(s.db, dto.Name, dto.Version)
	if err != nil {
		log.L().Error("check pipeline existence failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	if found {
		return apperr.CheckFail(response.CheckFailCode, response.PipelineRegistered)
	}

	newPipeline := models.Pipeline{
		Name:        dto.Name,
		Version:     dto.Version,
		Repository:  dto.Repository,
		NomadJobID:  fmt.Sprintf("%s-%s", dto.Name, dto.Version),
		Author:      dto.Author,
		Description: dto.Description,
		Status:      "pending",
	}

	if err := s.db.Create(&newPipeline).Error; err != nil {
		// Two pods can both pass the isPipelineExist check above and race to
		// insert the same (name, version); the unique index then rejects the
		// loser. Translate that into the same friendly "already registered"
		// result the pre-check returns, instead of a raw 500.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperr.CheckFail(response.CheckFailCode, response.PipelineRegistered)
		}
		log.L().Error("insert pipeline into db failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}

	// GO-2: capture ID explicitly so the goroutine does not close over the whole struct
	pipelineID := newPipeline.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), nomadRegisterTimeout)
		defer cancel()
		// Conditional on the row still being "pending" so a stale write can't
		// clobber a concurrent state change; rows left "pending" if this pod dies
		// mid-registration are resolved by ReconcileStalePending at startup.
		if err := s.registerNomadJob(ctx, dto); err != nil {
			s.db.Model(&models.Pipeline{}).Where("id = ? AND status = ?", pipelineID, "pending").
				Update("status", "failed")
			log.L().Error("async nomad pipeline registration failed",
				zap.Uint("pipeline_id", pipelineID), zap.Error(err))
		} else {
			s.db.Model(&models.Pipeline{}).Where("id = ? AND status = ?", pipelineID, "pending").
				Update("status", "ready")
			log.L().Info("pipeline added successfully", zap.Uint("pipeline_id", pipelineID))
		}
	}()

	return nil
}

// ReconcileStalePending fails pipelines stuck in the "pending" registration
// state older than olderThan — left behind when the registering pod died before
// the async goroutine finished. The age threshold avoids touching registrations
// still in flight on another pod.
func (s *pipelineService) ReconcileStalePending(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	res := s.db.Model(&models.Pipeline{}).
		Where("status = ? AND created_at < ?", "pending", cutoff).
		Update("status", "failed")
	return res.RowsAffected, res.Error
}

func (s *pipelineService) Delete(dto types.PipelineDeleteDto) error {
	resp := s.db.Model(&models.Pipeline{}).
		Where("name = ? AND version = ? AND status NOT IN (?)",
			dto.Name, dto.Version, []string{"deleting", "pending"}).
		Update("status", "deleting")

	if resp.Error != nil {
		log.L().Error("update pipeline status failed", zap.Error(resp.Error))
		return apperr.ServerError(response.SystemError)
	}
	if resp.RowsAffected == 0 {
		// Distinguish between "doesn't exist" and "already being processed"
		_, found, err := isPipelineExist(s.db, dto.Name, dto.Version)
		if err != nil {
			log.L().Error("check pipeline existence failed", zap.Error(err))
			return apperr.ServerError(response.SystemError)
		}
		if !found {
			return apperr.CheckFail(response.CheckFailCode, response.PipelineNotExist)
		}
		return apperr.CheckFail(response.CheckFailCode, response.PipelineDeletionConflict)
	}

	existingPipeline, found, err := isPipelineExist(s.db, dto.Name, dto.Version)
	if err != nil {
		log.L().Error("check pipeline existence after status update failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	if !found || existingPipeline.ID == 0 {
		log.L().Error("pipeline disappeared after status update",
			zap.String("name", dto.Name), zap.String("version", dto.Version))
		return apperr.ServerError(response.SystemError)
	}

	pipelineID := existingPipeline.ID
	nomadJobID := existingPipeline.NomadJobID
	go func() {
		if err := s.deleteNomadJob(nomadJobID); err != nil {
			s.db.Model(&models.Pipeline{}).Where("id = ?", pipelineID).Update("status", "failed")
			log.L().Error("async nomad pipeline deletion failed",
				zap.Uint("pipeline_id", pipelineID), zap.Error(err))
		} else {
			r := s.db.Unscoped().Delete(&models.Pipeline{}, pipelineID)
			if r.Error != nil {
				s.db.Model(&models.Pipeline{}).Where("id = ?", pipelineID).Update("status", "failed")
				log.L().Error("database deletion failed after nomad deletion",
					zap.Uint("pipeline_id", pipelineID), zap.Error(r.Error))
			}
		}
	}()

	return nil
}

func (s *pipelineService) Update(dto types.PipelineAddDto) error {
	// BUG-2: use the new 3-return isPipelineExist
	_, found, err := isPipelineExist(s.db, dto.Name, dto.Version)
	if err != nil {
		log.L().Error("check pipeline existence failed", zap.Error(err))
		return apperr.ServerError(err.Error())
	}
	if !found {
		return apperr.CheckFail(response.CheckFailCode, response.PipelineNotExist)
	}

	// BUG-1: use AND not && (which is the PostgreSQL array-overlap operator)
	if err := s.db.Model(&models.Pipeline{}).
		Where("name = ? AND version = ? AND repository = ?", dto.Name, dto.Version, dto.Repository).
		Updates(models.Pipeline{Author: dto.Author, Description: dto.Description}).Error; err != nil {
		return apperr.ServerError(response.PipelineUpdateError)
	}
	return nil
}

// nomadRegisterTimeout bounds the background Nomad register HTTP call so a hung
// Nomad API cannot leave the detached registration goroutine running until pod
// restart. ReconcileStalePending resolves the DB row regardless; this keeps the
// goroutine itself bounded.
const nomadRegisterTimeout = 60 * time.Second

func (s *pipelineService) registerNomadJob(ctx context.Context, dto types.PipelineAddDto) error {
	datacenters, cores, memory, nextflowURL := s.nomadCfg.PipelineDefaults()

	rendered, err := runner.RenderHCL(runner.NextflowHCL, map[string]any{
		"datacenters":  datacenters,
		"cores":        cores,
		"memory":       memory,
		"nextflow_url": nextflowURL,
	})
	if err != nil {
		return fmt.Errorf("failed to render nomad HCL template: %w", err)
	}

	jobConf, err := s.nomadJobs.ParseHCL(rendered, true)
	if err != nil {
		log.L().Error("nomad job parse failed", zap.Error(err))
		return fmt.Errorf("failed to parse job: %w", err)
	}

	jobID := dto.JobId()
	jobConf.ID = &jobID
	jobConf.Name = &jobID

	_, _, err = s.nomadJobs.Register(jobConf, (&nomad.WriteOptions{}).WithContext(ctx))
	return err
}

func (s *pipelineService) deleteNomadJob(nomadJobID string) error {
	_, _, err := s.nomadJobs.Deregister(nomadJobID, false, nil)
	if err != nil {
		// Nomad returns 404 when the job was never registered or already purged.
		// Treat this as success so a pipeline whose registration failed can still be deleted.
		if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			log.L().Warn("nomad job not found, treating as already deleted", zap.String("jobId", nomadJobID))
			return nil
		}
		return fmt.Errorf("deregister nomad job failed: %w", err)
	}
	log.L().Info("nomad job deleted successfully", zap.String("jobId", nomadJobID))
	return nil
}

// isPipelineExist queries for a pipeline by name+version.
// Returns (pipeline, true, nil) when found, (zero, false, nil) when not found,
// and (zero, false, err) on a real database error.
func isPipelineExist(db *gorm.DB, name, version string) (models.Pipeline, bool, error) {
	var pipeline models.Pipeline
	err := db.Where("name = ? AND version = ?", name, version).First(&pipeline).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pipeline, false, nil
		}
		return pipeline, false, fmt.Errorf("query pipeline: %w", err)
	}
	return pipeline, true, nil
}
