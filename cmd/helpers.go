package cmd

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"
)

const limitMax = 100

// validateLimit clamps limit to 1-100 and returns an error if limit is less than 1.
func validateLimit(limit int) (int, error) {
	if limit < 1 {
		return 0, errors.New("limit must be at least 1")
	}
	if limit > limitMax {
		return limitMax, nil
	}
	return limit, nil
}

// requireLimit reads --limit from cmd, validates it, and returns ErrSilent on failure.
func requireLimit(cmd *cobra.Command) (int, error) {
	raw, _ := cmd.Flags().GetInt("limit")
	limit, err := validateLimit(raw)
	if err != nil {
		return 0, failArg(err.Error())
	}
	return limit, nil
}

// sleepContext waits for d or until ctx is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
