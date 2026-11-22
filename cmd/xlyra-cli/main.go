package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Yachiiiiyo/xLyra-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.NewApp().Run(ctx, os.Args[1:]))
}
