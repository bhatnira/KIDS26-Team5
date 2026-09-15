package setting

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// InitConfig loads configuration into cfg. configPath may be empty, in which
// case the env var ANTELOPE_CONFIG or the default config.yaml is tried.
func InitConfig(cfg *ServerConfig, configPath string) *viper.Viper {
	v := viper.New()

	// Enable environment variable override
	// ANTELOPE_POSTGRESQL_HOST -> postgresql.host
	v.SetEnvPrefix("ANTELOPE")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Set defaults so env vars work even without a config file
	setDefaults(v)

	// Explicitly bind every leaf key of ServerConfig to its ANTELOPE_* env
	// var. viper's AutomaticEnv only feeds Unmarshal for keys it already
	// knows (defaults or a loaded config file); without this, env-only keys
	// that lack a default (passwords, signing keys, llm.api-key, email.*,
	// …) are silently dropped when running from env vars alone (e.g. in a
	// container with no config.yaml). Binding does not override config-file
	// values when the env var is unset.
	bindEnvs(v, ServerConfig{})

	// Try to load config file (optional in container deployments)
	config := resolveConfigPath(configPath)
	if config != "" {
		v.SetConfigFile(config)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			// If the caller explicitly pointed to a file, treat failure as fatal.
			if configPath != "" || os.Getenv(ConfigEnv) != "" {
				panic(fmt.Errorf("fatal error reading config file %s: %w", config, err))
			}
			fmt.Printf("Warning: config file %s not found, using env vars and defaults.\n", config)
		} else {
			fmt.Printf("Loaded config from: %s\n", config)
			v.OnConfigChange(func(e fsnotify.Event) {
				fmt.Println("config file changed:", e.Name)
				if err := v.Unmarshal(cfg); err != nil {
					fmt.Println(err)
				}
			})
			v.WatchConfig()
		}
	}

	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Errorf("fatal error unmarshalling config: %w", err))
	}

	// Print a redacted summary before validating so the snapshot is visible
	// even when start-up aborts. The zap logger is not initialised yet at this
	// point, so this writes to stdout directly.
	fmt.Fprint(os.Stdout, cfg.Summary())

	// Fail-fast on missing per-deployment secrets in release mode; in debug
	// mode Validate only warns so local development keeps working.
	if err := cfg.Validate(); err != nil {
		panic(fmt.Errorf("invalid configuration: %w", err))
	}

	return v
}

// setDefaults registers a default for every NON-SECRET config key, so the
// server can start from a bare environment and the redacted startup summary is
// always fully populated.
//
// Secrets are deliberately omitted: jwt.access-signing-key,
// jwt.refresh-signing-key, postgresql.password, redis.password and
// email.password have NO default. A shipped default secret is a footgun (a
// default DB/Redis/email password can never match the real external service,
// and a default JWT key would let anyone forge tokens), and Validate() relies
// on their emptiness/known-default to fail fast in release mode. The one
// exception is system.super-user-password, which seeds a local account and is
// therefore useful to default for dev — it is a known-insecure sentinel that
// Validate() still rejects in release.
//
// zap.format / zap.encode-level / zap.prefix are also intentionally absent:
// they are auto-selected from system.mode in cmd.setupConfig.
func setDefaults(v *viper.Viper) {
	// system
	v.SetDefault("system.port", 8086)
	v.SetDefault("system.mode", "release")
	v.SetDefault("system.super-user", "admin@antelope.dev")
	v.SetDefault("system.super-user-password", "password")

	// jwt (signing keys omitted — secrets)
	v.SetDefault("jwt.issuer", "antelope")
	v.SetDefault("jwt.access-expires-time", "24h")
	v.SetDefault("jwt.refresh-expires-time", "168h")

	// postgresql (password omitted — secret)
	v.SetDefault("postgresql.host", "127.0.0.1")
	v.SetDefault("postgresql.port", 5432)
	v.SetDefault("postgresql.db", "antelope")
	v.SetDefault("postgresql.username", "postgres")
	v.SetDefault("postgresql.sslmode", "disable")

	// redis (password omitted — secret)
	v.SetDefault("redis.host", "127.0.0.1")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)

	// nomad (the task name is an internal constant, not a config key —
	// see runner.TaskName)
	v.SetDefault("nomad.host", "http://localhost")
	v.SetDefault("nomad.port", 4646)
	v.SetDefault("nomad.namespace", "default")
	v.SetDefault("nomad.region", "global")
	v.SetDefault("nomad.token", "")
	v.SetDefault("nomad.datacenters", []string{"dc1"})
	v.SetDefault("nomad.cores", 2)
	v.SetDefault("nomad.memory", 8000)
	v.SetDefault("nomad.nextflow-url",
		"https://github.com/nextflow-io/nextflow/releases/download/v25.04.8/nextflow-25.04.8-dist")

	// cors
	v.SetDefault("cors.mode", "allow-all")

	// email (password omitted — secret; auth-type/tls-policy mirror the
	// parser fallbacks in internal/modules/email)
	v.SetDefault("email.host", "localhost")
	v.SetDefault("email.port", 25)
	v.SetDefault("email.from", "no-reply@antelope.dev")
	v.SetDefault("email.nickname", "Antelope")
	v.SetDefault("email.auth-type", "plain")
	v.SetDefault("email.tls-policy", "opportunistic")

	// zap (format/encode-level/prefix are mode-driven; see cmd.setupConfig)
	v.SetDefault("zap.level", "info")
	v.SetDefault("zap.director", "log")
	v.SetDefault("zap.show-line", true)
	v.SetDefault("zap.stacktrace-key", "stacktrace")
	v.SetDefault("zap.log-in-console", true)
	v.SetDefault("zap.max-backups", 7)
	v.SetDefault("zap.retention-day", 30)

	// agent (platform-wide defaults; per-user settings live in the DB)
	v.SetDefault("agent.app-name", "antelope")
	v.SetDefault("agent.daytona.default-snapshot", "antelope-bio")
	v.SetDefault("agent.daytona.workspace-path", "/home/user/trpc_agent_workspace")
	v.SetDefault("agent.daytona.sandbox-timeout-sec", 600)
	v.SetDefault("agent.daytona.auto-stop-minutes", 30)
	v.SetDefault("agent.daytona.insecure-tls-transfers", false)
	// Empty bundle roots fall back to skillbundle's own defaults (the
	// bundler's output path on disk, and a temp dir for the embedded copy), so
	// they are left unset here rather than hardcoded in two places.
	v.SetDefault("agent.skills.bundle-root", "")
	v.SetDefault("agent.skills.bundle-cache-root", "")
	v.SetDefault("agent.skills.user-cache-root", "/tmp/antelope-skills")
	v.SetDefault("agent.artifacts.presigned-ttl-seconds", 3600)

	// cab (external St. Jude CAB / nightingale upstream)
	v.SetDefault("cab.base-url", "http://nightingale-dev.stjude.org:8080")
}

// bindEnvs walks the mapstructure-tagged fields of iface and registers a
// viper env binding for every leaf key (e.g. "postgresql.password" ->
// ANTELOPE_POSTGRESQL_PASSWORD via the configured prefix + key replacer).
// Nested structs are recursed into; the dotted path mirrors the config file.
func bindEnvs(v *viper.Viper, iface any, parts ...string) {
	val := reflect.ValueOf(iface)
	typ := reflect.TypeOf(iface)
	if typ.Kind() == reflect.Pointer {
		val = val.Elem()
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("mapstructure")
		if tag == "-" {
			continue
		}
		// Strip mapstructure options (e.g. ",squash", ",omitempty").
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = field.Name
		}
		path := append(append([]string{}, parts...), name)
		if field.Type.Kind() == reflect.Struct {
			bindEnvs(v, val.Field(i).Interface(), path...)
			continue
		}
		// BindEnv computes the env name from the prefix + key replacer set
		// on v, so this stays in sync with AutomaticEnv naming.
		_ = v.BindEnv(strings.Join(path, "."))
	}
}

// resolveConfigPath returns the effective config file path.
// Priority: explicit (from CLI flag) > ANTELOPE env var > config.yaml default.
// Returns empty string if none is available.
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		fmt.Printf("Using config file: `%s` (CLI flag).\n", explicit)
		return explicit
	}
	if env := os.Getenv(ConfigEnv); env != "" {
		fmt.Printf("Using config file: `%s` (env var).\n", env)
		return env
	}
	if _, err := os.Stat(ConfigDefaultFile); err == nil {
		fmt.Printf("Using default config file: `%s`.\n", ConfigDefaultFile)
		return ConfigDefaultFile
	}
	fmt.Println("No config file found, using environment variables and defaults.")
	return ""
}
