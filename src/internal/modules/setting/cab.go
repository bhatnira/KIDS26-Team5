package setting

// CabConfig holds the connection settings for the external St. Jude CAB
// (Clinical Analytics Backend / "nightingale") service that Antelope proxies
// FASTQ lookups and pipeline submissions to.
//
// The base URL is intentionally configurable (ANTELOPE_CAB_BASE_URL /
// cab.base-url) rather than compiled in, so dev and prod deployments differ
// without rebuilding the image.
type CabConfig struct {
	BaseURL string `mapstructure:"base-url" json:"base-url" yaml:"base-url"`
}
