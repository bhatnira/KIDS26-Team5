package setting

type ServerConfig struct {
	Jwt       JwtConfig       `mapstructure:"jwt" json:"jwt" yaml:"jwt"`
	Zap       ZapConfig       `mapstructure:"zap" json:"zap" yaml:"zap"`
	DB        DBConfig        `mapstructure:"postgresql" json:"postgresql" yaml:"postgresql"`
	Redis     RedisConfig     `mapstructure:"redis" json:"redis" yaml:"redis"`
	Nomad     NomadConfig     `mapstructure:"nomad" json:"nomad" yaml:"nomad"`
	Slurm     SlurmConfig     `mapstructure:"slurm" json:"slurm" yaml:"slurm"`
	Scheduler SchedulerConfig `mapstructure:"scheduler" json:"scheduler" yaml:"scheduler"`
	Cors      CORSConfig      `mapstructure:"cors" json:"cors" yaml:"cors"`
	Email     EmailConfig     `mapstructure:"email" json:"email" yaml:"email"`
	System    SystemConfig    `mapstructure:"system" json:"system" yaml:"system"`
	Agent     AgentConfig     `mapstructure:"agent" json:"agent" yaml:"agent"`
	Cab       CabConfig       `mapstructure:"cab" json:"cab" yaml:"cab"`
}

// SchedulerConfig selects which job scheduler backend to use.
type SchedulerConfig struct {
	Type string `mapstructure:"type" yaml:"type"` // "nomad" or "slurm"
}
