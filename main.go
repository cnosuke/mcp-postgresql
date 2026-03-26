package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cnosuke/mcp-postgresql/config"
	"github.com/cnosuke/mcp-postgresql/logger"
	"github.com/cnosuke/mcp-postgresql/server"
	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
)

var (
	// Version and Revision are replaced when building.
	// To set specific version, edit Makefile.
	Version  = "0.0.1"
	Revision = "xxx"

	Name  = "mcp-postgresql"
	Usage = "A MCP server implementation for PostgreSQL"
)

func main() {
	app := &cli.Command{
		Name:    Name,
		Usage:   Usage,
		Version: fmt.Sprintf("%s (%s)", Version, Revision),
		Commands: []*cli.Command{
			{
				Name:    "server",
				Aliases: []string{"s"},
				Usage:   "Run the MCP PostgreSQL server (stdio transport)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Value:   "config.yml",
						Usage:   "path to the configuration file",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					configPath := cmd.String("config")

					cfg, err := config.LoadConfig(configPath)
					if err != nil {
						return errors.Wrap(err, "failed to load configuration file")
					}

					if err := logger.InitLogger(cfg.LogLevel, cfg.Log, true); err != nil {
						return errors.Wrap(err, "failed to initialize logger")
					}
					defer logger.Sync()

					defer func() {
						if err := server.CloseDB(); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to close database connections: %v\n", err)
						}
					}()

					return server.Run(cfg, Name, Version, Revision)
				},
			},
			{
				Name:  "http",
				Usage: "Run the MCP PostgreSQL server (Streamable HTTP transport)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Value:   "config.yml",
						Usage:   "path to the configuration file",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					configPath := cmd.String("config")

					cfg, err := config.LoadConfig(configPath)
					if err != nil {
						return errors.Wrap(err, "failed to load configuration file")
					}

					if err := logger.InitLogger(cfg.LogLevel, cfg.Log, false); err != nil {
						return errors.Wrap(err, "failed to initialize logger")
					}
					defer logger.Sync()

					defer func() {
						if err := server.CloseDB(); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to close database connections: %v\n", err)
						}
					}()

					return server.RunHTTP(cfg, Name, Version, Revision)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
