package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// decoratedCmd builds a command whose body is fn, decorated with the
// --max-duration budget and set to the given duration.
func decoratedCmd(t *testing.T, d time.Duration, fn func(ctx context.Context) error) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fn(cmd.Context())
		},
	}
	cmd.SetContext(context.Background())
	decorateRunWithMaxDuration(cmd)
	if err := cmd.Flags().Set("max-duration", d.String()); err != nil {
		t.Fatalf("set max-duration: %v", err)
	}

	return cmd
}

func TestDecorateRunWithMaxDuration_DeadlineExceededIsCleanStop(t *testing.T) {
	cmd := decoratedCmd(t, 30*time.Minute, func(ctx context.Context) error {
		return context.DeadlineExceeded
	})

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("expected nil for deadline with a budget set, got %v", err)
	}
}

func TestDecorateRunWithMaxDuration_WrappedDeadlineExceededIsCleanStop(t *testing.T) {
	cmd := decoratedCmd(t, 30*time.Minute, func(ctx context.Context) error {
		return errors.Join(context.DeadlineExceeded, nil)
	})

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("expected nil for wrapped deadline, got %v", err)
	}
}

func TestDecorateRunWithMaxDuration_OtherErrorPassesThrough(t *testing.T) {
	want := errors.New("boom")
	cmd := decoratedCmd(t, 30*time.Minute, func(ctx context.Context) error {
		return want
	})

	if err := cmd.RunE(cmd, nil); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestDecorateRunWithMaxDuration_NoBudgetDoesNotSwallowDeadline(t *testing.T) {
	cmd := decoratedCmd(t, 0, func(ctx context.Context) error {
		return context.DeadlineExceeded
	})

	if err := cmd.RunE(cmd, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("with no budget the deadline must not be swallowed, got %v", err)
	}
}

func TestDecorateRunWithMaxDuration_NoBudgetLeavesContextWithoutDeadline(t *testing.T) {
	cmd := decoratedCmd(t, 0, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("context must not have a deadline when no budget is set")
		}
		return nil
	})

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecorateRunWithMaxDuration_BudgetSetsContextDeadline(t *testing.T) {
	cmd := decoratedCmd(t, 30*time.Minute, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("context must have a deadline when a budget is set")
		}
		return nil
	})

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
