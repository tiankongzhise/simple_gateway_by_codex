package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"simple_gateway_by_codex/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
