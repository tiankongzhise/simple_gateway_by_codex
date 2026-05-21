package app

import (
	"context"
	"fmt"
)

// Run starts the gateway service. Feature modules are wired in subsequent steps.
func Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return fmt.Errorf("gateway service is not configured yet")
}
