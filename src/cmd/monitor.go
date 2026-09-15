package cmd

import (
	"context"

	appmod "antelope/internal/app"

	"github.com/urfave/cli/v3"
)

// CmdMonitor starts the standalone Nomad event monitor with a minimal HTTP
// health endpoint.  Use this when you want to run the monitor as an independent
// process (e.g. separate Kubernetes Deployment).
var CmdMonitor = &cli.Command{
	Name:  "monitor",
	Usage: "Start standalone Nomad event monitor (for independent deployment)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to the configuration file",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		cfg, err := setupConfig(cmd.String("config"))
		if err != nil {
			return err
		}

		a := appmod.NewMonitorApp(cfg)
		defer a.Shutdown()

		srv := NewServer(a)
		srv.RunMonitor()
		return nil
	},
}
