package notification

import (
	"context"

	"antelope/models"

	"gorm.io/gorm"
)

type Service interface {
	List(ctx context.Context, userId uint, page, pageSize int) ([]models.Notification, int64, error)
	MarkRead(ctx context.Context, userId, id uint) error
	MarkAllRead(ctx context.Context, userId uint) error
}

type service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) Service {
	return &service{db: db}
}

func (s *service) List(ctx context.Context, userId uint, page, pageSize int) ([]models.Notification, int64, error) {
	var items []models.Notification
	var total int64

	q := s.db.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userId)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *service) MarkRead(ctx context.Context, userId, id uint) error {
	return s.db.WithContext(ctx).
		Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userId).
		Update("is_read", true).Error
}

func (s *service) MarkAllRead(ctx context.Context, userId uint) error {
	return s.db.WithContext(ctx).
		Model(&models.Notification{}).
		Where("user_id = ? AND is_read = false", userId).
		Update("is_read", true).Error
}
