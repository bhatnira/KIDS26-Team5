package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"antelope/internal/modules/log"
	nixstorage "antelope/internal/modules/storage"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/types"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Service provides storage bucket/object and configuration operations.
type Service interface {
	// Config
	GetConfig(userID uint) (gin.H, error)
	SaveConfig(userID uint, dto types.UserStorageConfigDto) error
	DeleteConfig(userID uint) error
	TestConnection(dto types.TestStorageConnectionDto) (gin.H, error)

	// Bucket / object operations (all require a valid userID)
	GetBuckets(ctx context.Context, userID uint) (gin.H, error)
	GetObjects(ctx context.Context, userID uint, bucket, prefix string) (gin.H, error)
	GetUploadURL(ctx context.Context, userID uint, req types.PresignedURLReqDto) (gin.H, error)
	GetDownloadURL(ctx context.Context, userID uint, req types.PresignedURLReqDto) (gin.H, error)
	CreateBucket(ctx context.Context, userID uint, req types.CreateBucketReqDto) error
	DeleteBucket(ctx context.Context, userID uint, bucketName string) error
	DeleteObject(ctx context.Context, userID uint, bucket, prefix string, recursive bool) error
}

type ossService struct {
	manager *nixstorage.ClientManager
}

func NewOssService(manager *nixstorage.ClientManager) Service {
	return &ossService{manager: manager}
}

// ── Storage config operations ──────────────────────────────────────────────

func (s *ossService) GetConfig(userID uint) (gin.H, error) {
	if s.manager == nil {
		return gin.H{"config": types.UserStorageConfigResponse{Configured: false}}, nil
	}

	cfg, err := s.manager.GetProviderConfig(userID)
	if err != nil {
		log.L().Error("failed to get storage config from Redis", zap.Uint("userID", userID), zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	if cfg == nil {
		return gin.H{"config": types.UserStorageConfigResponse{Configured: false}}, nil
	}

	var minioConfig nixstorage.MinioConfig
	if err := json.Unmarshal(cfg.RawConfig, &minioConfig); err != nil {
		log.L().Error("failed to unmarshal storage config", zap.Uint("userID", userID), zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	return gin.H{"config": types.UserStorageConfigResponse{
		Host:               minioConfig.Host,
		Port:               minioConfig.Port,
		AccessKey:          minioConfig.AccessKey,
		SecretKey:          maskString(minioConfig.SecretKey),
		UseSSL:             minioConfig.UseSSL,
		Region:             minioConfig.Region,
		InsecureSkipVerify: minioConfig.InsecureSkipVerify,
		Configured:         true,
	}}, nil
}

func (s *ossService) SaveConfig(userID uint, dto types.UserStorageConfigDto) error {
	minioConfig := nixstorage.MinioConfig{
		Host:               dto.Host,
		Port:               dto.Port,
		AccessKey:          dto.AccessKey,
		SecretKey:          dto.SecretKey,
		UseSSL:             dto.UseSSL,
		Region:             dto.Region,
		InsecureSkipVerify: dto.InsecureSkipVerify,
	}

	rawConfig, err := json.Marshal(minioConfig)
	if err != nil {
		return apperr.ServerError(response.SystemError)
	}

	if s.manager != nil {
		if _, err := s.manager.SetClient(userID, nixstorage.ProviderMinio, json.RawMessage(rawConfig)); err != nil {
			log.L().Error("storage connection test failed", zap.Uint("userID", userID), zap.Error(err))
			return apperr.CheckFail(response.CheckFailCode, "Failed to connect to storage: "+err.Error())
		}
	} else {
		if err := nixstorage.TestMinioConnection(minioConfig); err != nil {
			log.L().Error("storage connection test failed", zap.Uint("userID", userID), zap.Error(err))
			return apperr.CheckFail(response.CheckFailCode, "Failed to connect to storage: "+err.Error())
		}
	}

	log.L().Info("user storage configuration saved", zap.Uint("userID", userID), zap.String("host", dto.Host))
	return nil
}

func (s *ossService) DeleteConfig(userID uint) error {
	if s.manager != nil {
		if err := s.manager.RemoveClient(userID); err != nil {
			log.L().Error("failed to delete storage config", zap.Uint("userID", userID), zap.Error(err))
			return apperr.ServerError(response.SystemError)
		}
	}
	log.L().Info("user storage configuration deleted", zap.Uint("userID", userID))
	return nil
}

func (s *ossService) TestConnection(dto types.TestStorageConnectionDto) (gin.H, error) {
	testConfig := nixstorage.MinioConfig{
		Host:               dto.Host,
		Port:               dto.Port,
		AccessKey:          dto.AccessKey,
		SecretKey:          dto.SecretKey,
		UseSSL:             dto.UseSSL,
		Region:             dto.Region,
		InsecureSkipVerify: dto.InsecureSkipVerify,
	}
	if err := nixstorage.TestMinioConnection(testConfig); err != nil {
		return nil, apperr.CheckFail(response.CheckFailCode, "Connection failed: "+err.Error())
	}
	return gin.H{"success": true}, nil
}

// ── Bucket / object operations ─────────────────────────────────────────────

func (s *ossService) storageClient(userID uint) (nixstorage.StorageClient, error) {
	if s.manager == nil {
		return nil, apperr.CheckFail(response.CheckFailCode, response.StorageNotConfigured)
	}
	client := s.manager.GetClient(userID)
	if client == nil {
		return nil, apperr.CheckFail(response.CheckFailCode, response.StorageNotConfigured)
	}
	return client, nil
}

func (s *ossService) GetBuckets(ctx context.Context, userID uint) (gin.H, error) {
	client, err := s.storageClient(userID)
	if err != nil {
		return nil, err
	}

	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		log.L().Error("list buckets failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	bucketInfos := make([]types.BucketInfo, len(buckets))
	for i, b := range buckets {
		bucketInfos[i] = types.BucketInfo{Name: b.Name, CreationDate: b.CreatedAt}
	}
	return gin.H{"buckets": bucketInfos}, nil
}

func (s *ossService) GetObjects(ctx context.Context, userID uint, bucket, prefix string) (gin.H, error) {
	client, err := s.storageClient(userID)
	if err != nil {
		return nil, err
	}

	if exists, err := client.BucketExists(ctx, bucket); err != nil {
		log.L().Error("check bucket existence failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	} else if !exists {
		return nil, apperr.NotFound(response.BucketNotFound)
	}

	objectCh, err := client.ListObjects(ctx, bucket, prefix, false)
	if err != nil {
		log.L().Error("list objects failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	var objects []types.ObjectInfo
	for entry := range objectCh {
		if entry.Err != nil {
			log.L().Error("get object failed", zap.String("bucket", bucket), zap.String("object", entry.Key), zap.Error(entry.Err))
			return nil, apperr.ServerError(response.SystemError)
		}
		if entry.Key == prefix {
			continue
		}
		relativeName := strings.TrimPrefix(entry.Key, prefix)
		if relativeName == "" {
			continue
		}
		if entry.IsDir {
			if folderName := strings.TrimSuffix(relativeName, "/"); folderName != "" {
				objects = append(objects, types.ObjectInfo{Name: folderName, IsFolder: true})
			}
		} else {
			objects = append(objects, types.ObjectInfo{
				Name:         relativeName,
				Size:         entry.Size,
				LastModified: entry.LastModified,
				IsFolder:     false,
				ContentType:  entry.ContentType,
			})
		}
	}
	return gin.H{"objects": objects}, nil
}

func (s *ossService) GetUploadURL(ctx context.Context, userID uint, req types.PresignedURLReqDto) (gin.H, error) {
	client, err := s.storageClient(userID)
	if err != nil {
		return nil, err
	}
	uploadURL, err := client.PresignedPutObject(ctx, req.Bucket, req.Key, 24*time.Hour)
	if err != nil {
		log.L().Error("get object upload url failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"urls": types.PresignedURLRespDto{UploadURL: uploadURL}}, nil
}

func (s *ossService) GetDownloadURL(ctx context.Context, userID uint, req types.PresignedURLReqDto) (gin.H, error) {
	client, err := s.storageClient(userID)
	if err != nil {
		return nil, err
	}
	downloadURL, err := client.PresignedGetObject(ctx, req.Bucket, req.Key, 24*time.Hour)
	if err != nil {
		log.L().Error("get object download url failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"urls": types.PresignedURLRespDto{DownloadURL: downloadURL}}, nil
}

func (s *ossService) CreateBucket(ctx context.Context, userID uint, req types.CreateBucketReqDto) error {
	client, err := s.storageClient(userID)
	if err != nil {
		return err
	}
	if err := client.CreateBucket(ctx, req.Name); err != nil {
		log.L().Error("create bucket failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *ossService) DeleteBucket(ctx context.Context, userID uint, bucketName string) error {
	client, err := s.storageClient(userID)
	if err != nil {
		return err
	}
	if err := client.RemoveBucket(ctx, bucketName); err != nil {
		log.L().Error("delete bucket failed", zap.Error(err))
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *ossService) DeleteObject(ctx context.Context, userID uint, bucket, prefix string, recursive bool) error {
	client, err := s.storageClient(userID)
	if err != nil {
		return err
	}

	if recursive && strings.HasSuffix(prefix, "/") {
		listCh, err := client.ListObjects(ctx, bucket, prefix, true)
		if err != nil {
			log.L().Error("list objects for deletion failed", zap.Error(err))
			return apperr.ServerError(response.ObjectDeleteError)
		}

		objectsCh := make(chan nixstorage.ObjectEntry)
		go func() {
			defer close(objectsCh)
			for entry := range listCh {
				if entry.Err != nil {
					log.L().Error("list object failed", zap.Error(entry.Err))
					continue
				}
				select {
				case objectsCh <- entry:
				case <-ctx.Done():
					// RemoveObjects stopped consuming (e.g. the loop below
					// returned on first error); stop feeding rather than
					// blocking this goroutine until ctx is torn down.
					return
				}
			}
		}()

		for rErr := range client.RemoveObjects(ctx, bucket, objectsCh) {
			if rErr.Err != nil {
				log.L().Error("remove object failed", zap.Error(rErr.Err))
				return apperr.ServerError(response.ObjectDeleteError)
			}
		}
		return nil
	}

	if err := client.DeleteObject(ctx, bucket, prefix); err != nil {
		log.L().Error("remove object failed", zap.Error(err))
		return apperr.ServerError(response.ObjectDeleteError)
	}
	return nil
}

func maskString(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
