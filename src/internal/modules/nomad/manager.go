package nomad

import (
	"fmt"

	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"

	nomad "github.com/hashicorp/nomad/api"
)

// InitNomad initialises a Nomad client from config and verifies connectivity
// with a lightweight Status().Leader() call.
// Panics on failure so the server does not start with a broken Nomad connection.
func InitNomad(cfg setting.NomadConfig) *nomad.Client {
	nomadCfg := nomad.DefaultConfig()
	nomadCfg.Address = cfg.Dsn()
	nomadCfg.Namespace = cfg.Namespace
	nomadCfg.Region = cfg.Region

	// An empty token means the cluster is no-auth (ACLs disabled), which is the
	// expected setup for local secure dev environments. Only set the SecretID
	// when a token is configured so we don't send an empty ACL token to a
	// no-auth server.
	if cfg.Token != "" {
		nomadCfg.SecretID = cfg.Token
	}

	client, err := nomad.NewClient(nomadCfg)
	if err != nil {
		panic(fmt.Errorf("failed to connect to nomad: %w", err))
	}

	if _, err := client.Status().Leader(); err != nil {
		panic(fmt.Errorf("nomad: connectivity check failed: %w", err))
	}

	log.L().Info("nomad connectivity verified")
	return client
}
