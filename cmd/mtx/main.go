// Command mtx is a terminal Matrix client. Config comes from MTX_* env, or from the first-run
// screen when no env is set and no store exists.
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

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "mtx:", err)
			return 1
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	cfg, err := startConfig(args, dataHome)
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
	if err := runTUI(ctx, c, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "mtx:", err)
		return 1
	}
	return 0
}

// runTUI is the ircc UI with the Matrix backend: same app, same commands, same keys.
func runTUI(ctx context.Context, c *matrix.Client, cfg matrix.Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m, err := model.New(ringCap)
	if err != nil {
		return err
	}
	var p *tea.Program
	in := ui.Coalesce(func(msg tea.Msg) { p.Send(msg) }) // human keys and wheel only, one batch per window
	r := newRunner(c.MX(), func(msg tea.Msg) { p.Send(msg) })
	r.loadHistory = func(room id.RoomID) {
		go func() {
			c.LoadRoom(ctx, room, r.onEvent)
			if name, ok := r.nameOf(room); ok {
				r.flushHistory(name)
			}
		}()
	}
	c.OnInvite(r.toastInvite)
	c.OnTyping(r.onTyping)
	_, server, _ := strings.Cut(cfg.UserID, ":")
	cs := &cmds{r: r, me: id.UserID(cfg.UserID), server: server}

	closeLog, err := openLog()
	if err != nil {
		return err
	}
	defer closeLog()
	logf("start user=%s", cfg.UserID)
	app := ui.New(m, r.send).WithUser(localpart(cfg.UserID)).WithCmd(cs.cmd).WithLiteral(r.sendLiteral).WithSeen(r.seen).WithReply(r.reply).WithEdit(r.edit).WithPicker()
	p = tea.NewProgram(app, tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
		switch msg.(type) {
		case tea.MouseWheelMsg, tea.KeyPressMsg: // in order, summed per frame: one render
			in(msg)
			return nil
		}
		return msg
	}))
	c.OnProgress(func(p matrix.Progress) {
		r.emit(ui.ProgressMsg{ID: p.ID, Text: p.Text, Done: p.Done, Total: p.Total, Finished: p.Finished})
	})

	syncErr := make(chan error, 1)
	go func() {
		if err := r.refresh(); err != nil {
			syncErr <- err
			p.Quit()
			return
		}
		if err := c.Run(ctx, r.onEvent); err != nil && !errors.Is(err, context.Canceled) {
			logf("sync stopped: %v", err)
			syncErr <- err
			p.Quit()
		}
	}()

	_, err = p.Run()
	cancel()
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
