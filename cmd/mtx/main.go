// Command mtx is a terminal Matrix client. Config comes from MTX_* env only.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"maunium.net/go/mautrix/id"

	"mtx/internal/matrix"
	"mtx/internal/model"
	"mtx/internal/ui"
)

const ringCap = 1000

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
	if err := runTUI(ctx, c, cfg, dataHome); err != nil {
		fmt.Fprintln(os.Stderr, "mtx:", err)
		return 1
	}
	return 0
}

// runTUI is the ircc UI with the Matrix backend: same app, same commands, same keys.
func runTUI(ctx context.Context, c *matrix.Client, cfg matrix.Config, dataHome string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m, err := model.New(ringCap)
	if err != nil {
		return err
	}
	var p *tea.Program
	r := newRunner(c.MX(), func(msg tea.Msg) { p.Send(msg) })
	c.OnInvite(r.toastInvite)
	_, server, _ := strings.Cut(cfg.UserID, ":")
	cs := &cmds{r: r, me: id.UserID(cfg.UserID), server: server}

	last := filepath.Join(dataHome, "mtx", "last") // the channel mtx was on when it last quit
	prev, _ := os.ReadFile(last)
	app := ui.New(m, r.send).WithUser(localpart(cfg.UserID)).WithCmd(cs.cmd).WithLiteral(r.sendLiteral).WithResume(strings.TrimSpace(string(prev)))
	p = tea.NewProgram(app)

	syncErr := make(chan error, 1)
	go func() {
		if err := r.refresh(); err != nil {
			syncErr <- err
			p.Quit()
			return
		}
		if err := c.Run(ctx, r.onEvent); err != nil && !errors.Is(err, context.Canceled) {
			syncErr <- err
			p.Quit()
		}
	}()

	final, err := p.Run()
	cancel()
	if a, ok := final.(ui.App); ok && a.Channel() != "" {
		if err := os.MkdirAll(filepath.Dir(last), 0o700); err == nil {
			_ = os.WriteFile(last, []byte(a.Channel()+"\n"), 0o600) // best effort
		}
	}
	if err != nil {
		return err
	}
	select {
	case err := <-syncErr:
		return fmt.Errorf("sync: %w", err)
	default:
	}
	return nil
}
