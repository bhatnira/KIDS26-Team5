package scheduler

import (
	"fmt"

	"antelope/internal/modules/setting"

	nomad "github.com/hashicorp/nomad/api"
)

// SchedulerType represents the type of scheduler to use.
type SchedulerType string

const (
	SchedulerTypeNomad SchedulerType = "nomad"
	SchedulerTypeSlurm SchedulerType = "slurm"
)

// Config holds the scheduler configuration.
type Config struct {
	Type  SchedulerType   `mapstructure:"type" yaml:"type"`
	Nomad setting.NomadConfig
	Slurm setting.SlurmConfig
}

// New creates a Scheduler based on the config type.
func New(cfg Config, nomadClient *nomad.Client) (Scheduler, error) {
	switch cfg.Type {
	case SchedulerTypeNomad, "":
		if nomadClient == nil {
			return nil, fmt.Errorf("nomad client is required for nomad scheduler")
		}
		return NewNomadDispatcher(nomadClient), nil

	case SchedulerTypeSlurm:
		return NewSlurmDispatcher(cfg.Slurm), nil

	default:
		return nil, fmt.Errorf("unknown scheduler type: %s", cfg.Type)
	}
}

// NewFromNomadConfig creates a Nomad scheduler from NomadConfig (backward compatible).
func NewFromNomadConfig(client *nomad.Client) Scheduler {
	return NewNomadDispatcher(client)
}

// NewFromSlurmConfig creates a Slurm scheduler from SlurmConfig.
func NewFromSlurmConfig(cfg setting.SlurmConfig) Scheduler {
	return NewSlurmDispatcher(cfg)
}
