package cmd

import (
	"fmt"

	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"
)

// setupConfig loads the application configuration and initializes the logger.
// configPath is the value of the -c / --config CLI flag (empty string = auto-detect).
func setupConfig(configPath string) (setting.ServerConfig, error) {
	var cfg setting.ServerConfig
	_ = setting.InitConfig(&cfg, configPath)

	// The log encoder is derived from the run mode so dev and prod differ
	// automatically: release emits structured JSON (no ANSI color, no prefix)
	// for log aggregators, while debug stays human-readable colored console.
	// These three keys in config.yaml are therefore advisory — see the zap
	// block comment in config.yaml.example.
	zapCfg := cfg.Zap
	if cfg.System.IsProduction() {
		zapCfg.Format = "json"
		zapCfg.EncodeLevel = "CapitalLevelEncoder"
		zapCfg.Prefix = ""
	} else {
		zapCfg.Format = "console"
		zapCfg.EncodeLevel = "CapitalColorLevelEncoder"
	}
	if err := log.Init(zapCfg); err != nil {
		return setting.ServerConfig{}, fmt.Errorf("failed to initialize logger: %w", err)
	}
	return cfg, nil
}
