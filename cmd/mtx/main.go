// Command mtx is a terminal Matrix client. Config comes from MTX_* env only.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"maunium.net/go/mautrix/event"

	"mtx/internal/matrix"
)

func main() { os.Exit(run()) }

func run() int {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "mtx:", err)
			return 1
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	cfg, err := matrix.LoadConfig(os.Getenv, dataHome)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mtx:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := matrix.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mtx:", err)
		return 1
	}
	defer c.Close()

	err = c.Run(ctx, func(_ context.Context, evt *event.Event, decErr error) error {
		if decErr != nil {
			fmt.Printf("%s %s [undecryptable: %v]\n", evt.Sender, evt.ID, decErr)
			return nil
		}
		fmt.Printf("%s %s\n", evt.Sender, evt.Content.AsMessage().Body)
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "mtx:", err)
		return 1
	}
	return 0
}
