package models

import "gorm.io/gorm"

type Notification struct {
	gorm.Model

	UserId   uint   `gorm:"index;not null"`
	Title    string `gorm:"type:varchar(128);not null"`
	Desc     string `gorm:"type:text"`
	Type     int    `gorm:"default:0"` // 0 = notification (job alerts)
	IsRead   bool   `gorm:"default:false"`
	TagTitle string `gorm:"type:varchar(32)"`
	TagType  string `gorm:"type:varchar(16)"` // info / success / warning / error
	Icon     string `gorm:"type:varchar(64)"`
}
