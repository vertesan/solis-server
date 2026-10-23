package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vertesan/solis-server/internal/command/makino"
	"github.com/vertesan/solis-server/internal/utils/config"
)

func main() {
	if err := config.LoadEnvironment(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "makino:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := makino.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "makino:", err)
		os.Exit(1)
	}
}
