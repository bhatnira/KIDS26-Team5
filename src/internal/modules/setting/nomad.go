package setting

import "fmt"

type NomadConfig struct {
	Host      string `mapstructure:"host" json:"host" yaml:"host"`
	Port      int    `mapstructure:"port" json:"port" yaml:"port"`
	Namespace string `mapstructure:"namespace" json:"namespace" yaml:"namespace"`
	Region    string `mapstructure:"region" json:"region" yaml:"region"`

	// Token is the Nomad ACL token (SecretID) used to authenticate against a
	// remote, ACL-enabled cluster. Leave empty to connect to a no-auth Nomad
	// server (e.g. a local dev instance with ACLs disabled).
	Token string `mapstructure:"token" json:"token" yaml:"token"`

	// Pipeline job registration settings
	Datacenters []string `mapstructure:"datacenters" json:"datacenters" yaml:"datacenters"`
	Cores       int      `mapstructure:"cores" json:"cores" yaml:"cores"`
	Memory      int      `mapstructure:"memory" json:"memory" yaml:"memory"`
	NextflowURL string   `mapstructure:"nextflow-url" json:"nextflow-url" yaml:"nextflow-url"`
}

func (n *NomadConfig) Dsn() string {
	return fmt.Sprintf("%s:%d", n.Host, n.Port)
}

// PipelineDefaults returns default values for pipeline HCL template vars,
// falling back to sensible defaults when config fields are zero/empty.
func (n *NomadConfig) PipelineDefaults() (datacenters []string, cores, memory int, nextflowURL string) {
	datacenters = n.Datacenters
	if len(datacenters) == 0 {
		datacenters = []string{"dc1"}
	}
	cores = n.Cores
	if cores <= 0 {
		cores = 2
	}
	memory = n.Memory
	if memory <= 0 {
		memory = 8000
	}
	nextflowURL = n.NextflowURL
	return datacenters, cores, memory, nextflowURL
}
