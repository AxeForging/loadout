package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/web"
	"github.com/urfave/cli/v3"
)

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Serve the local web UI (dashboard, store browser, builder) on localhost",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "addr",
				Usage:   "Listen address",
				Value:   "127.0.0.1:4600",
				Sources: cli.EnvVars("LOADOUT_ADDR"),
			},
			&cli.BoolFlag{
				Name:  "dev",
				Usage: "Enable permissive CORS for the Vite dev server",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			mgr, err := newSourceManager(cmd)
			if err != nil {
				return err
			}
			mgr.Store = eng.Store
			fmt.Printf("loadout web UI: http://%s\n", cmd.String("addr"))
			srv := web.New(web.Options{Addr: cmd.String("addr"), Engine: eng, Sources: mgr, Dev: cmd.Bool("dev")})
			return srv.ListenAndServe(ctx)
		},
	}
}
