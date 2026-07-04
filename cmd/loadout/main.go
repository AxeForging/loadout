package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AxeForging/loadout/internal/app"
)

func main() {
	if err := app.New().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
