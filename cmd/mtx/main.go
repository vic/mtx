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
	in := ui.Coalesce(func(msg tea.Msg) { p.Send(msg) }) // one batch, one render per burst
	r := newRunner(c.MX(), func(msg tea.Msg) {           // backend bursts share the input queue: one batch, one render
		if !in(msg) {
			p.Send(msg) // queue full: deliver directly
		}
	})
	r.loadHistory = func(room id.RoomID) {
		go func() {
			c.LoadRoom(ctx, room, r.onEvent)
			if name, ok := r.nameOf(room); ok {
				r.flushHistory(name)
			}
		}()
	}
	r.cacheLine = startCacher(ctx, c.SaveLines)
	r.cacheRooms = func(ls map[id.RoomID]string) {
		go func() {
			if err := c.SaveRooms(ctx, ls); err != nil {
				logf("cache rooms: %v", err)
			}
		}()
	}
	c.OnInvite(r.toastInvite)
	c.OnTyping(r.onTyping)
	c.OnPresence(r.onPresence)
	c.OnReceipt(r.onReceipt)
	me := c.MX().UserID.String() // cfg.UserID is empty when the login came from the store
	_, server, _ := strings.Cut(me, ":")
	cs := &cmds{r: r, me: id.UserID(me), server: server, inv: c, rec: c}

	closeLog, err := openLog()
	if err != nil {
		return err
	}
	defer closeLog()
	logf("start user=%s", me)
	app := ui.New(m, r.send).WithUser(localpart(me)).WithCmd(cs.cmd).WithSeen(r.seen).WithReply(r.reply).WithEdit(r.edit).
		WithInputSave(func(ch, text string) {
			go func() {
				if err := c.AddInput(ctx, ch, text); err != nil {
					logf("input history: %v", err)
				}
			}()
		}).
		WithFocusSave(func(ch string) {
			go func() {
				if err := c.SetKV(ctx, "focus", ch); err != nil {
					logf("focus: %v", err)
				}
			}()
		}).WithThemeSave(func(id string) {
		go func() {
			if err := c.SetKV(ctx, "theme", id); err != nil {
				logf("theme: %v", err)
			}
		}()
	}).WithTyping(r.typing).WithThreadOpen(r.openThread(c.LoadThread)).WithContextLoad(r.openContext(c.LoadContext)).WithOlderLoad(r.openOlder(c.LoadOlder)).WithPicker()
	p = tea.NewProgram(app, tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
		switch msg.(type) {
		case tea.MouseWheelMsg, tea.KeyPressMsg: // in order, summed per batch: one render
			if in(msg) {
				return nil
			}
		}
		return msg
	}))
	c.OnProgress(func(p matrix.Progress) {
		r.emit(ui.ProgressMsg{ID: p.ID, Text: p.Text, Done: p.Done, Total: p.Total, Finished: p.Finished})
	})

	syncErr := make(chan error, 1)
	go func() {
		r.loadCache(ctx, c.LoadRooms, c.LoadLines)
		if ins, err := c.Inputs(ctx); err == nil {
			r.emit(ui.InputsMsg(ins))
		}
		if f, err := c.KV(ctx, "focus"); err == nil && f != "" {
			r.emit(ui.FocusMsg(f)) // the channel that had focus when mtx last quit
		}
		if t, err := c.KV(ctx, "theme"); err == nil && t != "" {
			r.emit(ui.ThemeMsg(t)) // the theme the user chose last time
		}
		go func() { // the room list loads beside sync: cached rooms already show
			r.refreshMu.Lock()
			defer r.refreshMu.Unlock()
			if err := r.refresh(); err != nil {
				r.emit(ui.ToastMsg{Text: "rooms: " + err.Error(), Err: true})
			}
		}()
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
