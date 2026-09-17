package setting

// SlurmConfig holds Slurm scheduler configuration.
type SlurmConfig struct {
	// SSH connection settings
	Host     string `mapstructure:"host" yaml:"host"`
	Port     int    `mapstructure:"port" yaml:"port"`
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`

	// Slurm job defaults
	Partition string `mapstructure:"partition" yaml:"partition"`
	Account   string `mapstructure:"account" yaml:"account"`
	TimeLimit string `mapstructure:"time-limit" yaml:"time-limit"`
	QoS       string `mapstructure:"qos" yaml:"qos"`

	// Remote paths
	WorkDir    string `mapstructure:"work-dir" yaml:"work-dir"`
	NextflowURL string `mapstructure:"nextflow-url" yaml:"nextflow-url"`

	// Resource defaults
	Cores  int `mapstructure:"cores" yaml:"cores"`
	Memory int `mapstructure:"memory" yaml:"memory"` // in MB
}

// Defaults fills in zero-value fields with sensible defaults.
func (s *SlurmConfig) Defaults() {
	if s.Port == 0 {
		s.Port = 22
	}
	if s.Partition == "" {
		s.Partition = "normal"
	}
	if s.TimeLimit == "" {
		s.TimeLimit = "24:00:00"
	}
	if s.Cores == 0 {
		s.Cores = 2
	}
	if s.Memory == 0 {
		s.Memory = 8000
	}
	if s.WorkDir == "" {
		s.WorkDir = "/tmp/antelope-jobs"
	}
}
