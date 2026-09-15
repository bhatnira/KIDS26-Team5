package main

import (
	"context"
	"os"
	"runtime"
	"strings"

	nixcmd "antelope/cmd"
	"antelope/internal/modules/log"

	"github.com/urfave/cli/v3"
	"go.uber.org/automaxprocs/maxprocs"
)

// Build-time injection via -ldflags.
var (
	Version = "development" // program version for this build
	Tags    = ""            // the Golang build tags
)

// @title       Antelope API
// @version     1.0
// @description Antelope bioinformatics pipeline management platform API.
//
// @contact.name  Antelope Team
// @license.name  MIT
//
// @host      localhost:8086
// @BasePath  /api/v1
//
// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Enter "Bearer {accessToken}"
//
// @securityDefinitions.apikey RefreshAuth
// @in                         header
// @name                       Authorization
// @description                Enter "Bearer {refreshToken}" — used only for /auth/refresh

func main() {
	maxprocs.Set(maxprocs.Logger(func(string, ...any) {})) //nolint:errcheck // best-effort GOMAXPROCS tuning; failure is non-fatal

	cli.OsExiter = func(code int) {
		log.Close()
		os.Exit(code)
	}

	app := &cli.Command{
		Name:    "antelope",
		Usage:   "Antelope bioinformatics pipeline management platform",
		Version: formatVersion(),
		Commands: []*cli.Command{
			nixcmd.CmdWeb,
			nixcmd.CmdMonitor,
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		os.Exit(1)
	}

	log.Close()
}

func formatVersion() string {
	goVer := runtime.Version()
	if len(Tags) == 0 {
		return Version + " built with " + goVer
	}
	return Version + " built with " + goVer + " : " + strings.ReplaceAll(Tags, " ", ", ")
}
