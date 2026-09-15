package models

import (
	"gorm.io/gorm"
)

type Pipeline struct {
	gorm.Model
	Name        string `gorm:"type:varchar(64);not null;uniqueIndex:idx_pipeline_name_version,priority:1"`
	Version     string `gorm:"type:varchar(20);not null;uniqueIndex:idx_pipeline_name_version,priority:2"`
	NomadJobID  string `gorm:"type:varchar(128);not null;unique"`
	Status      string `gorm:"type:varchar(20);not null"` // "pending", "deleting", "ready", "failed"
	Repository  string `gorm:"type:varchar(128);not null"`
	Author      string `gorm:"type:varchar(64);not null"`
	Description string `gorm:"type:text"`
	TemplateID  *uint  `gorm:"index" json:"template_id,omitempty"`

	Template *JobTemplate `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
	Jobs     []Job        `gorm:"constraint:OnDelete:SET NULL;" json:"jobs,omitempty"`
}

func (p *Pipeline) GetPipelineID() uint {
	return p.ID
}

func (p *Pipeline) GetPipelineName() string {
	return p.Name
}

func (p *Pipeline) GetPipelineVersion() string {
	return p.Version
}

func (p *Pipeline) BeforeDelete(tx *gorm.DB) error {
	// before pipeline deletion
	return tx.Model(&Job{}).Where("pipeline_id = ?", p.GetPipelineID()).Updates(map[string]any{
		"pipeline_name":    p.GetPipelineName(),
		"pipeline_version": p.GetPipelineVersion(),
	}).Error
}
