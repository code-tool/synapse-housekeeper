package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// addMaxDurationFlag registers the --max-duration budget on a long-running command.
func addMaxDurationFlag(cmd *cobra.Command) {
	cmd.Flags().Duration("max-duration", 0, "Stop the run after this wall-clock budget (e.g. 30m, 2h); 0 means no limit")
}

// decorateRunWithMaxDuration adds the --max-duration flag and wraps cmd.RunE so
// the command runs under that wall-clock budget when one is set. The command's
// own body is left untouched and keeps using cmd.Context() as before.
//
// Reaching the deadline is a clean stop (returns nil), not a failure: the
// cleanup operations are idempotent, so whatever was in flight when the budget
// elapsed is simply re-picked-up on the next run.
func decorateRunWithMaxDuration(cmd *cobra.Command) *cobra.Command {
	addMaxDurationFlag(cmd)

	inner := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		d, err := cmd.Flags().GetDuration("max-duration")
		if err != nil {
			return err
		}

		if d > 0 {
			ctx, cancel := context.WithTimeout(cmd.Context(), d)
			defer cancel()
			cmd.SetContext(ctx)
		}

		err = inner(cmd, args)
		if d > 0 && errors.Is(err, context.DeadlineExceeded) {
			logger.Info("max-duration reached; stopping", zap.Duration("max_duration", d))
			return nil
		}

		return err
	}

	return cmd
}
