package ui

import (
	"cmp"
	"image"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/mosaic"

	"mtx/internal/model"
	"mtx/internal/theme"
)

// SessionsMsg replaces the live session list.
type SessionsMsg []model.Session

// LineMsg appends L to channel Ch; an unknown channel (including "") is reported as a
// note. Old is history replayed at attach: no unread count, no move to the front.
// ThreadRootMsg fills in the text of a thread head whose root was not seen this session.
type ThreadRootMsg struct{ Ch, Root, Nick, Text string }
type PinsMsg struct {
	Ch  string
	IDs []string // the room's pinned event IDs, oldest pin first
}

// ReplaceLineMsg swaps a placeholder line (same ID) for the decrypted one.
type ReplaceLineMsg struct {
	Ch, ID string
	L      model.Line
	Edit   bool // an edit (keep the old text in the line's history), not a decrypt
}

// ExploreMsg lists public rooms: Entries are the shown text, Targets what :join takes (same order).
type ExploreMsg struct{ Entries, Targets []string }

// ListMsg is a read-only list shown in a picker: Title names it, Entries are the rows.
type ListMsg struct {
	Title   string
	Entries []string
}

// SearchMsg is the server's search results for the room: Entries are shown, IDs are the event IDs (newest first).
type SearchMsg struct{ Entries, IDs []string }

// ReadMsg says another member read channel Ch up to the line with event ID; own lines up to it show read.
type ReadMsg struct{ Ch, ID, Nick string } // Nick: who read (their read receipt sits at ID)

// FullyReadMsg is the event my read marker points at in channel Ch (room account data m.fully_read).
type FullyReadMsg struct{ Ch, ID string }

// ReactsMsg sets the reaction summary of the line with event ID ID.
type ReactsMsg struct{ Ch, ID, Reacts string }

// RemoveLineMsg drops a placeholder line (same ID).
type RemoveLineMsg struct{ Ch, ID string }

// HistoryLinesMsg is a batch of history lines for one channel: one update, one render.
type HistoryLinesMsg struct {
	Ch string
	L  []model.Line
}

type LineMsg struct {
	Old bool
	Ch  string
	L   model.Line
}

// NoteMsg adds L to the focused channel (producer notes with no channel of their own).
// ProgressMsg is a status toast: the one with the same ID is updated in place, with a bar when Total > 0.
type ProgressMsg struct {
	ID, Text    string
	Done, Total int
	Finished    bool
}

type NoteMsg struct{ L model.Line }

// NickMsg sets the user's nick after the endpoint took a fallback name.
type NickMsg struct{ Nick string }

// RoomsMsg sets the visible room (&) channels to exactly these names.
type RoomsMsg []string

// TagsMsg sets each channel's tag group: "favourite", "lowpriority" or "" (none).
type TagsMsg map[string]string

// IDsMsg maps channel names to Matrix room IDs.
type IDsMsg map[string]string

// ToastMsg shows Text as a notification card; Err makes it red.
type ToastMsg struct {
	Text string
	Err  bool
}

// WithTyping sets the hook that sends my typing notice (it must not block).
func (a App) WithTyping(f func(ch string)) App {
	a.typingFn = f
	return a
}

// WithInputSave sets the hook that stores each sent input line (it must not block).
func (a App) WithInputSave(f func(ch, text string)) App {
	a.inputSave = f
	return a
}

// WithFocusSave sets the hook that stores the focused channel on each switch (it must not block).
func (a App) WithFocusSave(f func(ch string)) App {
	a.focusSave = f
	return a
}

// InputsMsg sets every channel's sent input lines (from the store), oldest first.
type InputsMsg map[string][]string

// FocusMsg makes channel Ch current (after /join).
type FocusMsg string

// AgentsMsg sets the subagents of session channel Ch (nicks and busy state).
type AgentsMsg struct {
	Ch     string
	Agents []model.Agent
}

// MembersMsg sets the member nicks of room Ch.
type MembersMsg struct {
	Ch     string
	Nicks  []string
	Levels []int // power level of each nick (parallel to Nicks); admins first
}

// TypingMsg sets the nicks typing now in room Ch (empty = nobody). They show 💬 in the nick pane.
type TypingMsg struct {
	Ch    string
	Nicks []string
}

// InfoMsg replaces the per-pid status the modeline shows.
type InfoMsg map[int]model.SessionInfo

// SendResultMsg reports a finished send of Text on Ch; Err is nil on success.
type SendResultMsg struct {
	Ch, Text string
	Err      error
}

// CmdMsg carries a parsed command (other than /quit) to the program owner,
// which performs the side effect (join, kick, ...).
type CmdMsg struct{ Cmd model.Cmd }

const (
	defaultW, defaultH = 80, 24
	promptCmd          = ':'
	promptShell        = '$'
	promptCapture      = '!'

	wheelRows = 3 // rows one wheel notch scrolls
)

// App is the bubbletea model: keys.go decides, App applies. No sockets here;
// sending goes through the injected send func.
type App struct {
	m             model.Model
	mru           []string // channel names, most recently focused first (the recent picker lists them so)
	tmru          []string // thread roots, most recently opened first (the recent picker lists them so)
	mode          EditMode
	pending       string
	in            input                 // multi-line input: shift+enter adds a line, enter sends
	hist          map[string][]string   // sent input lines per channel, oldest first (InputsMsg loads them)
	inputSave     func(ch, text string) // stores a sent input line; must not block; nil = off
	focusSave     func(ch string)       // stores the focused channel; must not block; nil = off
	themeIDs      []string              // the theme picker's scheme IDs ("" = terminal default), parallel to its rows
	themeBefore   string                // the theme active when the theme picker opened
	themeNow      string                // the theme in use ("" = terminal default)
	themeSave     func(id string)       // stores the chosen theme; must not block; nil = off
	histAt        int                   // index into hist while browsing; -1 = not browsing
	histDraft     string                // the unsent input held while browsing
	send          func(ch, text string) error
	cmdf          func(ch string, c model.Cmd) tea.Cmd // runs a parsed command; ch is the focused channel
	user          string                               // nick of the local echo line
	quote         *model.Line                          // a clicked line being replied to; sent above the next line, not in the input
	info          map[int]model.SessionInfo
	now           func() time.Time
	w, h          int
	prompt        rune // 0 or promptCmd (:): the input line holds a prompt
	query         string
	hit           int                                               // index in the current channel of the last search hit, -1 none
	rows          *rowCache                                         // wrapped rows per line: scrolling reuses them
	lc            *linesCache                                       // shown lines of the current channel, see lines
	sc            *sideCache                                        // side panes between frames, see sides
	offs          map[string]int                                    // per channel: lines scrolled back from the newest
	help          bool                                              // the ? popup is shown; the next key closes it
	focus         paneFocus                                         // tab: which pane takes keys (chat, channels, nicks)
	fq            string                                            // query typed in a focused side pane (fuzzy filter)
	fsel          int                                               // selected row in the focused side pane
	ptop          [focusThreads]int                                 // first shown row of the channel and nick panes (wheel scrolls it)
	stamps        bool                                              // s: show "15:04 " before each line
	pick          bool                                              // ctrl+o or /: the picker is open
	pickWhat      pickKind                                          // what the picker lists: channels (ctrl+o), lines (/), speakers (S)
	speakers      []string                                          // S: show only these nicks' lines; none = all
	grep          string                                            // ctrl+g: show only lines fuzzy-matching this; "" off
	marks         []string                                          // S picker: nicks marked with space, applied by enter
	toasts        []toast                                           // notification cards, newest last; the tick drops old ones
	helpOff       int                                               // help box scroll (j k, wheel) when it is taller than the screen
	pq            textinput.Model                                   // picker query (own input: a typed message survives)
	files         []string                                          // pickFiles: the session's files, from git ls-files (filesMsg)
	psel          int                                               // picker selection, an index in pickList
	comp          completion                                        // tab completion state of the INSERT line
	mentions      bool                                              // m: show only direct lines (see shows)
	typing        map[string][]string                               // channel -> nicks typing now (TypingMsg); a room's nick pane marks them
	reply         func(ch string, to model.Line, text string) error // A + send: a reply to a line with an ID; nil = plain send with a quote
	seen          func(ch string) error                             // the channel's newest line is on screen (read markers); nil = none
	threads       map[string][]model.Thread                         // per channel: thread heads (ThreadsMsg)
	thread        string                                            // selected thread root ("" = main chat): the chat pane shows it, sends go to it
	tags          map[string]string                                 // channel -> tag group (TagsMsg)
	presence      map[string]string                                 // nick -> online, unavailable or offline (PresenceMsg)
	presenceVer   int                                               // bumped by PresenceMsg: the side pane cache key
	img           string                                            // the image popup, rendered; "" = closed
	verifyTxn     string                                            // the device verification in progress (VerifyMsg)
	verifySAS     string                                            // the emojis to compare, while the verification box is open; "" none
	loadContext   func(ch, id string)                               // fetches a message and its neighbours; must not block; nil = off
	wantHit       string                                            // the event ID to select once its history arrives
	loadOlder     func(ch string)                                   // fetches one older page of history; must not block; nil = off
	retryMotion   string                                            // a backward motion waiting for older history: its key
	tagsVer       int                                               // bumped by TagsMsg: the side pane cache key
	typingFn      func(ch string)                                   // tells the room I am typing; must not block; nil = off
	typedAt       time.Time                                         // last typing notice sent
	reactRow      bool                                              // R in the message menu: the quick reaction row is open; the next key picks
	menu          string                                            // the open leader menu ("" none)
	threadOpen    func(ch, root string)                             // loads a thread's history when it is opened; must not block; nil = off
	listTitle     string                                            // the title of the read-only list picker (ListMsg)
	list          []string                                          // its rows
	found         []string                                          // :search: the matches shown in the picker
	foundIDs      []string                                          // the event ID of each match
	explore       []string                                          // :explore: the public rooms shown in the picker
	exploreTo     []string                                          // the :join target of each explore row
	edits         map[string][]string                               // per event ID: earlier texts, oldest first (EDIT history)
	pendingReacts map[string]map[string]string                      // channel -> event ID -> reaction summary for a line not loaded yet
	pins          map[string][]string                               // per channel: pinned event IDs (PinsMsg)
	pinsVer       int
	forwarding    bool                                                  // w in the message menu: the chat picker forwards the selected line
	jumpOpen      bool                                                  // F: the picked label opens its line's link or media
	jumpChan      bool                                                  // gc: the labels pick a channel of the channel pane
	jump          bool                                                  // f: line labels are shown; the next key picks a line
	lastLineCmd   string                                                // the last :react command run on a line, for .
	readBy        map[string]map[string]string                          // channel -> reader nick -> event ID they last read (ReadMsg)
	readAt        map[string]time.Time                                  // per channel: newest line another member has read (ReadMsg)
	fullyRead     map[string]string                                     // per channel: the event my fully-read marker points at (FullyReadMsg)
	tscroll       int                                                   // wheel offset of the threads box, negative = up; 0 on a channel switch
	lastThread    string                                                // the last thread opened (T toggles between main and this)
	mainUnread    map[string]int                                        // per channel: main-thread lines not on screen (main thread row count)
	edit          *model.Line                                           // C: the message being edited; the next send replaces it
	editFn        func(ch string, target model.Line, text string) error // the edit hook (WithEdit)
	quoteBody     bool                                                  // A and O: the quote is also in the body
	seenLive      map[string]int                                        // per thread root: Live replies when it was last read
}

// New returns an App over m that sends channel text through send.
func New(m model.Model, send func(ch, text string) error) App {
	return App{m: m, send: send, user: "me", now: time.Now, w: defaultW, h: defaultH, hit: -1, histAt: -1, rows: &rowCache{m: map[rowKey][]string{}}, lc: &linesCache{}, sc: &sideCache{}}
}

// Init starts the 1 s tick: it drops old toasts and keeps the clock current.
func (a App) Init() tea.Cmd { return tick() }

func tick() tea.Cmd { return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} }) }

type tickMsg struct{}

// toastFor is how long a toast shows (jjui default: 4 s).
const toastFor = 4 * time.Second

// typingEvery is the gap between typing notices while typing.
const typingEvery = 4 * time.Second

type toast struct {
	text    string
	err     bool
	at      time.Time
	id      string // status toasts: the ProgressMsg ID that updates this one
	done    int
	total   int
	running bool // a status still in progress: no expiry
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := a.m.Current().Name
	model, cmd := a.update(msg)
	app, ok := model.(App)
	if ok && app.thread == "" { // on the main thread: its unread count is read
		delete(app.mainUnread, app.m.Current().Name)
	}
	if !ok || app.seen == nil {
		return model, cmd
	}
	ch := app.m.Current().Name
	switch {
	case ch == "":
	case ch != before: // a switch: wait, so arrowing past channels does not load each one
		if app.focusSave != nil {
			app.focusSave(ch)
		}
		cmd = tea.Batch(cmd, tea.Tick(seenDelay, func(time.Time) tea.Msg { return seenDueMsg(ch) }))
	case newLineFor(msg, ch), msg == seenDueMsg(ch):
		cmd = tea.Batch(cmd, seenCmd(app.seen, ch))
	}
	return app, cmd
}

// newLineFor reports whether msg adds a live line to channel ch.
func newLineFor(msg tea.Msg, ch string) bool {
	l, ok := msg.(LineMsg)
	return ok && !l.Old && l.Ch == ch
}

// seenCmd reports to the program owner that channel ch is on screen; it runs off the UI loop.
func seenCmd(seen func(ch string) error, ch string) tea.Cmd {
	return func() tea.Msg {
		_ = seen(ch) // read markers are best effort: a failed receipt shows nothing
		return nil
	}
}

// seenDelay is how long a channel must stay current before its read markers and history load.
const seenDelay = 250 * time.Millisecond

// seenDueMsg fires seenDelay after channel switched to; Update runs the seen hook if it is still current.
type seenDueMsg string

// update applies msg (Update adds the read-marker hook around it).
func (a App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SessionsMsg:
		a.m = a.m.WithSessions(msg)
	case ThreadsMsg:
		if a.threads == nil {
			a.threads = map[string][]model.Thread{}
		}
		a.threads[msg.Ch] = msg.Heads
		if a.seenLive == nil {
			a.seenLive = map[string]int{}
		}
		for _, h := range msg.Heads {
			if _, ok := a.seenLive[h.Root]; !ok || h.Root == a.thread {
				a.seenLive[h.Root] = h.Live
			}
		}
		if a.thread != "" && !slices.ContainsFunc(msg.Heads, func(h model.Thread) bool { return h.Root == a.thread }) && msg.Ch == a.m.Current().Name {
			a.thread = "" // the selected thread is gone
		}
	case TypingMsg:
		if a.typing == nil {
			a.typing = map[string][]string{}
		}
		a.typing[msg.Ch] = msg.Nicks
	case BatchMsg:
		var cmds []tea.Cmd
		for _, m := range coalesceRuns(msg) {
			next, c := a.Update(m)
			a = next.(App)
			cmds = append(cmds, c)
		}
		return a, tea.Batch(cmds...)
	case ThreadRootMsg:
		for i := range a.threads[msg.Ch] {
			if h := &a.threads[msg.Ch][i]; h.Root == msg.Root {
				h.Nick, h.Text = msg.Nick, msg.Text
			}
		}
	case ReplaceLineMsg:
		if c, ok := a.chanByName(msg.Ch); ok && msg.Edit {
			for i := range c.Lines.Len() {
				if l := c.Lines.At(i); l.ID == msg.ID {
					if a.edits == nil {
						a.edits = map[string][]string{}
					}
					a.edits[msg.ID] = append(slices.Clone(a.edits[msg.ID]), l.Text)
					break
				}
			}
		}
		if m, err := a.m.ReplaceLine(msg.Ch, msg.ID, msg.L); err == nil {
			a.m = m
		}
		a = a.applyPending(msg.Ch)
	case ReactsMsg:
		if !a.m.HasLine(msg.Ch, msg.ID) { // the line has not arrived yet: keep the summary until it does
			if a.pendingReacts == nil {
				a.pendingReacts = map[string]map[string]string{}
			}
			if a.pendingReacts[msg.Ch] == nil {
				a.pendingReacts[msg.Ch] = map[string]string{}
			}
			a.pendingReacts[msg.Ch][msg.ID] = msg.Reacts
			break
		}
		if m, err := a.m.SetReacts(msg.Ch, msg.ID, msg.Reacts); err == nil {
			a.m = m
		}
	case PinsMsg:
		if a.pins == nil {
			a.pins = map[string][]string{}
		}
		a.pins[msg.Ch] = slices.Clone(msg.IDs)
		a.pinsVer++
	case RemoveLineMsg:
		if m, err := a.m.RemoveLine(msg.Ch, msg.ID); err == nil {
			a.m = m
		}
	case InputsMsg:
		a.hist = maps.Clone(msg)
	case HistoryLinesMsg:
		before := len(a.lines(a.m.Current()))
		if m, err := a.m.AddHistoryLines(msg.Ch, msg.L); err == nil {
			a.m = m.SortRecent()
		}
		added := 0
		if msg.Ch == a.m.Current().Name {
			added = len(a.lines(a.m.Current())) - before
			if added > 0 && a.hit >= 0 { // the selected line keeps its place after the prepend
				a.hit += added
			}
		}
		a = a.applyPending(msg.Ch)
		if msg.Ch == a.m.Current().Name {
			a = a.selectWanted()
		}
		if k := a.retryMotion; k != "" && msg.Ch == a.m.Current().Name {
			a.retryMotion = ""
			switch {
			case added == 0:
				a = a.toast("no more", false)
			case k == "line":
				a = a.lineStep(false)
			case k == "scroll":
				a = a.scroll(Action{Kind: Scroll, N: -1})
			default:
				a = a.bracket(false, k)
			}
		}
	case LineMsg:
		if msg.Old {
			if m, err := a.m.AddHistory(msg.Ch, msg.L); err == nil {
				a.m = m
			}
		} else if next, err := a.addLine(msg.Ch, msg.L); err == nil {
			a = next
			if msg.L.Thread == "" && (msg.Ch != a.m.Current().Name || a.thread != "") { // a main line not on screen
				a.mainUnread = bumpMain(a.mainUnread, msg.Ch)
			}
		} else {
			a = a.note("line for unknown channel " + msg.Ch)
		}
		a = a.applyPending(msg.Ch)
	case ImageMsg:
		m := mosaic.New().Width(max(a.w*2/3, 1)).Height(max(a.h*2/3, 1))
		a.img = m.Render(msg.Img)
	case VerifyMsg:
		a = a.verifyStep(msg)
	case ThemeMsg:
		a = a.applyTheme(string(msg))
	case ExploreMsg:
		a.explore, a.exploreTo = msg.Entries, msg.Targets
		a = a.openPick(pickExplore)
	case ListMsg:
		a.listTitle, a.list = msg.Title, msg.Entries
		a = a.openPick(pickInfo)
	case SearchMsg:
		a.found, a.foundIDs = msg.Entries, msg.IDs
		if len(msg.Entries) == 0 {
			a = a.toast("no results", false)
			break
		}
		a = a.openPick(pickFound)
	case FullyReadMsg:
		if a.fullyRead == nil {
			a.fullyRead = map[string]string{}
		}
		a.fullyRead[msg.Ch] = msg.ID
	case ReadMsg:
		c, ok := a.chanByName(msg.Ch)
		if !ok {
			break
		}
		for i := range c.Lines.Len() {
			if l := c.Lines.At(i); l.ID == msg.ID && l.At.After(a.readAt[msg.Ch]) {
				if a.readAt == nil {
					a.readAt = map[string]time.Time{}
				}
				a.readAt[msg.Ch] = l.At
			}
		}
		if msg.Nick != "" {
			if a.readBy == nil {
				a.readBy = map[string]map[string]string{}
			}
			if a.readBy[msg.Ch] == nil {
				a.readBy[msg.Ch] = map[string]string{}
			}
			old, had := a.readBy[msg.Ch][msg.Nick]
			a.readBy[msg.Ch][msg.Nick] = msg.ID
			if had && old != msg.ID {
				a = a.setSeen(msg.Ch, old)
			}
			a = a.setSeen(msg.Ch, msg.ID)
		}
	case PresenceMsg:
		if a.presence == nil {
			a.presence = map[string]string{}
		}
		a.presence[msg.Nick] = msg.State
		a.presenceVer++
	case TagsMsg:
		if a.tags == nil {
			a.tags = map[string]string{}
		}
		for ch, g := range msg { // merge: a tag change names one room, the rest keep theirs
			a.tags[ch] = g
		}
		a.tagsVer++
	case IDsMsg:
		a.m = a.m.WithIDs(msg)
	case RoomsMsg:
		a.m = a.m.WithRooms(msg).SortRecent()
	case CmdMsg:
		if a.cmdf == nil {
			return a.note("no command handler installed"), nil
		}
		return a, a.cmdf(a.m.Current().Name, msg.Cmd)
	case ProgressMsg:
		a = a.progressToast(msg)
	case NoteMsg:
		a = a.toast(msg.L.Text, false) // runtime notes are notifications, not scrollback
	case NickMsg:
		a.user = msg.Nick
	case ToastMsg:
		a = a.toast(msg.Text, msg.Err)
	case tickMsg:
		now := a.now()
		a.toasts = slices.DeleteFunc(slices.Clone(a.toasts), func(t toast) bool { return !t.running && now.Sub(t.at) >= toastFor })
		return a, tick()
	case fileBlockMsg:
		if msg.Err != nil {
			a = a.note("file: " + msg.Err.Error())
		} else {
			a = a.insertFence("text "+msg.Path, msg.Text)
		}
	case filesMsg:
		a.files = msg
	case shellOutMsg:
		a = a.insertBlock(msg.Cmd, string(msg.Out))
		if msg.Err != nil {
			a = a.note("command: " + msg.Err.Error())
		}
	case execDoneMsg:
		if msg.Err != nil {
			a = a.note(msg.What + " failed: " + msg.Err.Error())
		}
	case SendResultMsg:
		a = a.sent(msg)
	case FocusMsg:
		a = a.focusName(string(msg))
	case AgentsMsg:
		if m, err := a.m.WithAgents(msg.Ch, msg.Agents); err == nil {
			a.m = m
		}
	case MembersMsg:
		ms := make([]model.Member, len(msg.Nicks))
		for i, n := range msg.Nicks {
			ms[i].Nick = n
			if i < len(msg.Levels) {
				ms[i].Level = msg.Levels[i]
			}
		}
		if m, err := a.m.WithRoomMembers(msg.Ch, ms); err == nil {
			a.m = m
		}
	case InfoMsg:
		a.info = maps.Clone(msg)
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
	case tea.PasteMsg: // pasted text is never keys; it goes to an open input only
		if a.mode == Insert || a.prompt != 0 {
			a.in.InsertString(msg.Content)
			return a, nil
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && !a.help && !a.pick && a.onSponsor(msg.X, msg.Y) {
			return a, openURL(donateURL)
		}
		if msg.Button == tea.MouseLeft && a.help {
			return a.helpClick(tea.Mouse(msg))
		}
		if msg.Button == tea.MouseLeft && !a.help && !a.pick && a.prompt == 0 {
			return a.click(tea.Mouse(msg))
		}
	case tea.MouseWheelMsg:
		if n := notch(msg.Button); n != 0 {
			return a.wheel(msg, n), nil
		}
	case KeyRun:
		if a.mode == Normal && a.focus == focusChat && a.prompt == 0 && !a.pick && !a.help && a.pending == "" {
			if act, _, _ := Key(Normal, "", msg.M.String()); act.Kind == Scroll { // j/k: one clamp for the whole run
				return a.scroll(Action{Scroll, act.N * msg.N, ""}), nil
			}
		}
		var cmds []tea.Cmd
		for range msg.N {
			next, c := a.key(msg.M)
			a = next.(App)
			cmds = append(cmds, c)
		}
		return a, tea.Batch(cmds...)
	case WheelMsg:
		return a.wheel(msg.M, msg.N), nil
	case tea.KeyPressMsg: // v2 decodes one key per event, also when one read holds "gt"
		return a.key(msg)
	}
	return a, nil
}

func (a App) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return a, tea.Quit
	}
	if a.img != "" { // the image popup: any key closes it
		a.img = ""
		return a, nil
	}
	if a.verifySAS != "" { // the emoji box: y they match, n they differ
		switch k {
		case "y":
			return a.runCmd(":verify confirm")
		case "n", "esc":
			return a.runCmd(":verify cancel")
		}
		return a, nil
	}
	if a.jump {
		a.jump = false
		open, chanJump := a.jumpOpen, a.jumpChan
		a.jumpOpen, a.jumpChan = false, false
		if chanJump {
			_, names := a.paneItems(focusChans)
			var rows []string // the channel rows from the top of the pane: group labels skipped
			for _, n := range names[min(a.ptop[focusChans], len(names)):] {
				if !strings.HasPrefix(n, nickLabel) {
					rows = append(rows, n)
				}
			}
			if i := strings.Index(jumpKeys, k); len(k) == 1 && i >= 0 && i < len(rows) {
				return a.focusName(rows[i]), nil
			}
			return a, nil
		}
		ts := a.jumpTargets()
		if i := strings.Index(jumpKeys, k); len(k) == 1 && i >= 0 && i < len(ts) {
			a.hit = ts[i]
			if open {
				l := a.lines(a.m.Current())[a.hit]
				if u := firstURL(l.Text); u != "" {
					return a, openURL(u)
				}
				return a.runCmd(cmdOpen)
			}
		}
		return a, nil
	}
	if a.menu != "" {
		if k == "backspace" { // back to the parent menu; from space, the normal menu with every key
			if a.menu == "space" {
				a.menu = "normal"
			} else {
				a.menu = menuParent(a.menu)
			}
			return a, nil
		}
		items := menus[a.menu]
		a.menu = ""
		for _, it := range items {
			if it.key != k {
				continue
			}
			switch {
			case strings.HasPrefix(it.do, "menu:"):
				a.menu = strings.TrimPrefix(it.do, "menu:")
			case it.do == "react":
				a.reactRow = true
			case strings.HasPrefix(it.do, "key:"):
				var cmd tea.Cmd
				for _, k := range keySeq(strings.TrimPrefix(it.do, "key:")) {
					var m tea.Model
					m, cmd = a.key(keyMsg(k))
					a = m.(App)
				}
				return a, cmd
			case it.do == "permalink":
				return a.permalink()
			case strings.HasPrefix(it.do, "pick:"):
				switch strings.TrimPrefix(it.do, "pick:") {
				case "chat":
					return a.openPick(pickChans), nil
				case "thread":
					return a.openPick(pickThreads), nil
				case "recent":
					return a.openPick(pickRecent), nil
				case "pins":
					return a.openPick(pickPins), nil
				case "edits":
					if l, ok := a.lastWithID(a.hit); !ok || len(a.edits[l.ID]) == 0 {
						return a.toast("no edits", false), nil
					}
					return a.openPick(pickEdits), nil
				}
				return a.openPick(pickUsers), nil
			case it.do == "thread":
				a = a.nextThread()
			case it.do == "stamps":
				a.stamps, a.offs = !a.stamps, nil // row counts changed
			case it.do == "seenby":
				a = a.seenBy()
			case it.do == "forward":
				a = a.openPick(pickChans)
				a.forwarding = true
			case it.do == "jumpunread":
				a = a.jumpUnread()
			case strings.HasPrefix(it.do, "emoji:"):
				a.mode = Insert
				a.in.SetValue(a.in.Value() + strings.TrimPrefix(it.do, "emoji:"))
				a.in.MoveToEnd()
			case strings.HasSuffix(it.do, " "):
				a = a.openPrompt(promptCmd)
				a.in.SetValue(strings.TrimPrefix(it.do, ":"))
				a.in.MoveToEnd()
			default:
				return a.runCmd(it.do)
			}
			return a, nil
		}
		return a, nil // any other key closes the menu
	}
	if a.reactRow {
		a.reactRow = false
		if n := int(k[0] - '0'); len(k) == 1 && n >= 1 && n <= len(quickReacts) {
			return a.runCmd(":react " + quickReacts[n-1])
		}
		return a, nil
	}
	if a.focus != focusChat && a.mode == Normal && a.prompt == 0 && !a.pick && !a.help {
		return a.paneKey(msg)
	}
	if a.pick {
		if a.pickWhat == pickThemes {
			return a.themeKey(msg)
		}
		return a.pickKey(msg)
	}
	if a.help { // the help box: arrows and ctrl+d/u scroll it, esc closes it, a typed key searches help
		switch k {
		case "down":
			return a.helpScroll(1), nil
		case "up":
			return a.helpScroll(-1), nil
		case "ctrl+d":
			return a.helpScroll(a.h / 2), nil
		case "ctrl+u":
			return a.helpScroll(-a.h / 2), nil
		case "esc":
			a.help, a.helpOff = false, 0
			return a, nil
		}
		if t := msg.Text; t != "" { // a typed key: fuzzy search help with it
			a.help, a.helpOff = false, 0
			a = a.openPick(pickHelp)
			a.pq.SetValue(t)
			a.pq.CursorEnd()
			return a, nil
		}
		return a, nil
	}
	if k == "shift+enter" && (a.mode == Insert || a.prompt != 0) && !a.pick && !a.help {
		a.in.InsertString("\n")
		return a, nil
	}
	if a.prompt != 0 {
		return a.promptKey(msg)
	}
	if a.mode == Insert {
		act, mode, _ := Key(Insert, "", k)
		switch {
		case act.Kind == Send:
			return a.sendLine()
		case act.Kind == None && mode == Normal:
			a.mode = Normal
			a.quote, a.edit, a.quoteBody = nil, nil, false // esc drops the reply or edit; the typed text stays
		case act.Kind == None:
			a.in.Key(k, msg.Text)
			if a.typingFn != nil && a.in.Value() != "" && a.now().Sub(a.typedAt) >= typingEvery {
				a.typedAt = a.now()
				a.typingFn(a.m.Current().Name)
			}
			if a.in.Value() == "" { // deleting back to the start ends the reply
				a.quote = nil
			}
			return a, nil
		case act.Kind == Scroll:
			return a.scroll(act), nil
		case act.Kind == LineUp || act.Kind == LineDown:
			dir := "up" // multi-line input: up and down move inside it, until the first or last line
			if act.Kind == LineDown {
				dir = "down"
			}
			if a.mode == Insert && a.in.Key(dir, "") {
				return a, nil
			}
			return a.lineStep(act.Kind == LineDown), nil
		case act.Kind == PickChan:
			return a.openPick(pickChans), nil
		case act.Kind == FileBlock:
			return a.openFilePick(pickFileBlock)
		case act.Kind == PickFile:
			return a.openFilePick(pickFiles)
		case act.Kind == HistPrev || act.Kind == HistNext:
			a = a.histStep(act.Kind == HistPrev)
			return a, nil
		case act.Kind == PaneNext || act.Kind == PanePrev:
			return a.switchChan(act), nil
		}
		if act.Kind == Complete {
			v, comp := a.comp.step(a.in.Value(), a.chanNames(), a.nickPool(), true)
			a.in.SetValue(v)
			a.in.MoveToEnd()
			a.comp = comp
		}
		return a, nil
	}
	if pa, ok := normalPreKeys[k]; ok && a.pending == "" { // "/" and "esc" are taken here, before Key
		switch pa.Kind {
		case PickLines:
			return a.openPick(pickLines), nil
		case ClearLine: // NORMAL esc clears the selected line, the search highlights, the input text and the reply
			a.hit, a.quote, a.query = -1, nil, ""
			a.in.Reset()
			return a, nil
		}
	}
	act, mode, pending := Key(Normal, a.pending, k)
	a.pending = pending
	if mode == Insert {
		a.mode = Insert
		a = a.prepare(act.Kind)
	}
	switch act.Kind {
	case React:
		return a.runCmd(":react " + quickReacts[act.N-1])
	case JumpUnread:
		return a.jumpUnread(), nil
	case GotoDate:
		a = a.openPrompt(promptCmd)
		a.in.SetValue(cmdGoto + " ")
		a.in.MoveToEnd()
		return a, nil
	case Quit:
		return a, tea.Quit
	case Leader:
		a.menu = "space"
		return a, nil
	case CmdLine:
		return a.openPrompt(promptCmd), nil
	case CaptureLine:
		return a.openPrompt(promptCapture), nil
	case ShellLine:
		return a.openPrompt(promptShell), nil
	case Scroll, HalfPage, Top, Bottom:
		a = a.scroll(act)
	case ThreadNext:
		a = a.nextThread()
	case ToggleMentions:
		a.mentions = a.mentions != (act.Kind == ToggleMentions)
		a.hit, a.offs = -1, nil // positions index the shown lines, which changed
	case Help:
		a.help, a.helpOff = true, 0
	case Yank:
		return a.yank()
	case PickInvite:
		if a.m.Current().Kind != model.RoomChan {
			return a.toast("invite works in a & room: :join &name first", true), nil
		}
		return a.openPick(pickInvite), nil
	case Delete:
		if _, ok := a.ownTarget(); !ok {
			return a.toast("delete: select one of my messages", true), nil
		}
		return a.runCmd(cmdRedact) // D: the shift is the confirmation
	case OpenLine:
		return a.runCmd(cmdOpen)
	case Zen:
		zen = !zen
		a.offs = nil // row counts change with the width
	case Bracket:
		a = a.bracket(act.N > 0, act.Key)
	case JumpReply:
		ls := a.lines(a.m.Current())
		if a.hit < 0 || a.hit >= len(ls) || ls[a.hit].ReplyTo == "" {
			return a.toast("gr: select a reply first", true), nil
		}
		for i, l := range ls {
			if l.ID == ls[a.hit].ReplyTo {
				a.hit = i
				return a.showHit(), nil
			}
		}
		return a.toast("gr: the replied message is not loaded", true), nil
	case MsgMenu:
		if a.hit >= 0 {
			a.menu = "message"
		}
		return a, nil
	case Permalink:
		return a.permalink()
	case Repeat:
		if a.lastLineCmd == "" {
			return a, nil
		}
		return a.runCmd(a.lastLineCmd)
	case JumpLine:
		a.jump = true
		return a, nil
	case JumpOpen:
		a.jump, a.jumpOpen = true, true
		return a, nil
	case JumpChan:
		if !wide(a.w, a.h) {
			return a.toast("gc needs the side panes", true), nil
		}
		a.jump, a.jumpChan = true, true
		return a, nil
	case PickUser:
		return a.openPick(pickUsers), nil
	case PickThread:
		return a.openPick(pickThreads), nil
	case PickPins:
		return a.openPick(pickPins), nil
	case PickRecent:
		return a.openPick(pickRecent), nil
	case PickSpeaker:
		return a.openPick(pickSpeakers), nil
	case PickChan:
		return a.openPick(pickChans), nil
	case GrepLines:
		return a.openPick(pickGrep), nil
	case FileBlock:
		return a.openFilePick(pickFileBlock)
	case PickFile:
		return a.openFilePick(pickFiles)
	case LineUp, LineDown:
		a = a.lineStep(act.Kind == LineDown)
	case NextTurn, PrevTurn:
		a = a.turn(act.Kind == NextTurn)
	case FocusPane:
		a.focus, a.fq, a.fsel = focusChans, "", max(slices.Index(a.chanRows(), a.m.Current().Name), 0) // start on the current channel
	case PaneNext, PanePrev, GotoChan:
		a = a.switchChan(act)
	}
	return a, nil
}

func (a App) openPrompt(p rune) App {
	a.prompt = p
	a.in.Reset()
	return a
}

func (a App) closePrompt() (App, string) {
	v := a.in.Value()
	a.prompt = 0
	a.in.Reset()
	return a, v
}

func (a App) promptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a, _ = a.closePrompt()
	case "tab":
		if a.prompt == promptCmd {
			v, comp := a.comp.step(":"+a.in.Value(), a.chanNames(), a.nickPool(), false)
			a.in.SetValue(strings.TrimPrefix(v, ":"))
			a.in.MoveToEnd()
			a.comp = comp
		}
		return a, nil
	case "ctrl+t": // the file finder fills the prompt with a path
		return a.openFilePick(pickFiles)
	case "enter":
		kind := a.prompt
		var v string
		a, v = a.closePrompt()
		if kind == promptCapture {
			return a.runCapture(v)
		}
		if kind == promptShell {
			return a.runShell(v)
		}
		if strings.TrimSpace(v) == "" {
			return a, nil
		}
		return a.runCmd(":" + v)
	default:
		a.in.Key(msg.String(), msg.Text)
		return a, nil
	}
	return a, nil
}

func (a App) switchChan(act Action) App {
	if act.Kind == GotoChan { // {n}gt: the n-th channel in the channel pane's order, 1-based
		names := a.paneNames()
		to := act.N - 1
		if to < 0 || to >= len(names) {
			return a
		}
		return a.focusName(names[to])
	}
	return a.paneChan(act)
}

// paneNames are the channels in the channel pane's order: its rows without the group labels.
func (a App) paneNames() []string {
	var names []string
	for _, n := range a.chanRows() {
		if !strings.HasPrefix(n, nickLabel) {
			names = append(names, n)
		}
	}
	return names
}

// paneChan steps to the next or previous channel in the channel pane's order, with wrap.
func (a App) paneChan(act Action) App {
	names := a.paneNames()
	if len(names) == 0 {
		return a
	}
	to := slices.Index(names, a.m.Current().Name) + 1
	if act.Kind == PanePrev {
		to = slices.Index(names, a.m.Current().Name) - 1
	}
	return a.focusName(names[(to+len(names))%len(names)])
}

// focusName makes channel name current.
func (a App) focusName(name string) App {
	from := a.m.Current().Name
	for i, c := range a.m.All() {
		if c.Name == name && name != from {
			if !slices.Contains(a.mru, from) { // the channel I leave counts as visited
				a = a.touched(from)
			}
			a = a.touched(name)
			a.m, _ = a.m.Focus(i)                   // i is in range: it comes from Channels
			a.hit, a.speakers, a.grep = -1, nil, "" // the speaker and grep filters belong to the channel they were set in
			a.thread = ""                           // a channel opens on its main thread
			a.tscroll = 0
			a.histAt = -1
		}
	}
	return a
}

func (a App) sendLine() (tea.Model, tea.Cmd) {
	text := a.in.Value()
	if strings.TrimSpace(text) != "" && !strings.HasPrefix(text, "/recover") { // the recovery key never reaches history
		ch := a.m.Current().Name
		if a.hist == nil {
			a.hist = map[string][]string{}
		}
		a.hist[ch] = append(a.hist[ch], text)[max(len(a.hist[ch])+1-histMax, 0):]
		if a.inputSave != nil {
			a.inputSave(ch, text)
		}
	}
	a.histAt = -1
	a.in.Reset()
	a.mode = Normal // RET sends and leaves INSERT
	ch := a.m.Current().Name
	switch {
	case text == "" || ch == "":
	default:
		if a.edit != nil && a.editFn != nil { // C: the send replaces the message being edited
			target := *a.edit
			a.edit = nil
			return a, func() tea.Msg { return SendResultMsg{Ch: ch, Text: text, Err: a.editFn(ch, target, text)} }
		}
		if a.thread != "" && a.reply != nil { // a thread send: the thread relation carries the root
			if a.quote != nil && a.quoteBody { // O: the quote is in the body too
				text = quotePrefix(*a.quote) + text
				a.quote, a.quoteBody = nil, false
			}
			to := model.Line{ID: a.thread, Thread: a.thread}
			return a, func() tea.Msg { return SendResultMsg{Ch: ch, Text: text, Err: a.reply(ch, to, text)} }
		}
		if a.quote != nil && a.reply != nil { // a reply: the relation carries the quoted line, the text is sent as typed
			to := *a.quote
			a.quote = nil
			if a.quoteBody { // A: the quote is in the body too
				text = quotePrefix(to) + text
				a.quoteBody = false
			}
			return a, func() tea.Msg { return SendResultMsg{Ch: ch, Text: text, Err: a.reply(ch, to, text)} }
		}
		if a.quote != nil { // the agent sees the line replied to, as markdown quote
			text = "> <" + a.quote.Nick + "> " + a.quote.Text + "\n" + text
			a.quote = nil
		}
		send := a.send // blocking I/O runs in the Cmd, never in Update
		return a, func() tea.Msg { return SendResultMsg{Ch: ch, Text: text, Err: send(ch, text)} }
	}
	return a, nil
}

func (a App) runCmd(line string) (tea.Model, tea.Cmd) {
	line = "/" + strings.TrimPrefix(strings.TrimPrefix(line, ":"), "/") // one form inside: the rewrites below and ParseCmd take "/word"
	if word, rest, _ := strings.Cut(line, " "); word == "/react" {      // the target is the cursor line
		l, ok := a.lastWithID(a.hit)
		if !ok {
			return a.note("react: no message with an ID here"), nil
		}
		if r := strings.TrimSpace(rest); r != "" {
			a.lastLineCmd = ":react " + r
		}
		line = "/react " + strings.TrimSpace(rest) + " " + l.ID
	}
	if word, rest, _ := strings.Cut(line, " "); word == "/redact" { // the target is my cursor line
		l, ok := a.ownTarget()
		if !ok {
			return a.note("redact: no message of mine here"), nil
		}
		line = strings.TrimSpace("/redact " + l.ID + " " + strings.TrimSpace(rest))
	}
	if sub, ok := strings.CutPrefix(line, "/verify "); ok && (sub == "accept" || sub == "confirm" || sub == "cancel") { // the verification in progress is the target
		if a.verifyTxn == "" {
			return a.note("no verification in progress"), nil
		}
		line = "/verify " + sub + " " + a.verifyTxn
	}
	if line == "/preview" { // the target is the selected line's first link
		l, _ := a.lastWithID(a.hit)
		u := firstURL(l.Text)
		if u == "" {
			return a.note("preview: no link here"), nil
		}
		line = "/preview " + u
	}
	if line == "/pin" || line == "/unpin" { // the target is the cursor line
		l, ok := a.lastWithID(a.hit)
		if !ok {
			return a.note("pin: no message with an ID here"), nil
		}
		line += " " + l.ID
	}
	if line == "/view" { // the target is the selected line
		l, ok := a.lastWithID(a.hit)
		if !ok {
			return a.note("view: no message with an ID here"), nil
		}
		line = "/view " + l.ID
	}
	if line == "/open" {
		l, ok := a.lastWithID(a.hit)
		if !ok {
			return a.note("open: no message with an ID here"), nil
		}
		if _, _, media := mediaTag(l.Text); !media { // no media: open the link in the text
			if url := firstURL(l.Text); url != "" {
				return a, openURL(url)
			}
		}
		line = "/open " + l.ID
	}
	c, err := model.ParseCmd(line)
	if err != nil {
		return a.note(err.Error()), nil
	}
	switch c.Name() {
	case model.CmdSponsor:
		return a, openURL(donateURL)
	case model.CmdDocs:
		return a, openURL(docsURL)
	}
	if c.Name() == model.CmdForward { // the target is the selected line
		return a.forward(c.Args()[0])
	}
	if c.Name() == model.CmdOlder {
		if a.loadOlder != nil {
			a.loadOlder(a.m.Current().Name)
		}
		return a.toast("loading older messages…", false), nil
	}
	if c.Name() == model.CmdTheme {
		return a.themeCmd(c.Args()), nil
	}
	if c.Name() == model.CmdGoto {
		return a.gotoDate(c.Args()[0]), nil
	}
	if c.Name() == model.CmdHelp { // the same screen as ? (vic 2026-10-23)
		a.help = true
		return a, nil
	}
	if c.Name() == model.CmdQuit {
		return a, tea.Quit
	}
	return a, func() tea.Msg { return CmdMsg{c} }
}

// note adds a dim local line to the current channel.
func (a App) note(text string) App { return a.toast(text, true) }

// toast shows text in a card at the bottom right for toastFor (vic 2026-10-23,
// like jjui flash messages); err cards are red, others green.
func (a App) toast(text string, err bool) App {
	a.toasts = append(slices.Clone(a.toasts), toast{text: text, err: err, at: a.now()})
	return a
}

// inputLine is the modeline label, the input row text and the key hints the
// modeline shows for the current mode.
func (a App) inputLine() (label, input, hint string) {
	switch {
	case a.prompt == promptCmd:
		if u := a.cmdUsage(); u != "" && !zen {
			return "NORMAL", ": " + withCursor(a.in.Value(), a.in.Pos()), u
		}
		return "NORMAL", ": " + withCursor(a.in.Value(), a.in.Pos()), hintKeys("enter", "run", "C-t", "files", "C-f", "file block", "esc", "cancel")
	case a.prompt == promptCapture:
		return "NORMAL", "! " + withCursor(a.in.Value(), a.in.Pos()), hintKeys("enter", "run, insert output", "C-t", "files", "C-f", "file block", "esc", "cancel")
	case a.prompt == promptShell:
		return "NORMAL", "$ " + withCursor(a.in.Value(), a.in.Pos()), hintKeys("enter", "run locally", "C-t", "files", "C-f", "file block", "esc", "cancel")
	case a.mode == Insert:
		prompt := "> "
		if a.quote != nil { // RE: while replying; the quote is dropped once the input is empty
			prompt = "RE: "
		}
		if a.edit != nil { // C: the input is the edited text; enter saves it
			return "EDIT", prompt + withCursor(a.in.Value(), a.in.Pos()), hintKeys("enter", "save edit", "esc", "cancel")
		}
		hint := hintKeys("enter", "send", "@", "mention", "tab", "complete", "↑↓", "line", "C-t", "files", "C-f", "file block", "C-o", "pick", "esc", "normal")
		if a.quote != nil { // the replied-to line is shown here, not in the input
			quote := keyHint("re", "<"+a.quote.Nick+"> "+a.quote.Text)
			if hint == "" {
				hint = quote
			} else {
				hint = quote + "   " + hint
			}
		}
		return "INSERT", prompt + withCursor(a.in.Value(), a.in.Pos()), hint
	case a.focus == focusChans:
		return "CHANNELS", a.normalFooter(), hintKeys("type", "filter "+a.fq, "↑↓", "select", "enter", "join", "tab", "nicks", "esc", "chat")
	case a.focus == focusThreads:
		return "THREADS", a.normalFooter(), hintKeys("type", "filter "+a.fq, "↑↓", "select", "enter", "open thread", "tab", "chat", "esc", "chat")
	case a.focus == focusNicks:
		return "NICKS", a.normalFooter(), hintKeys("type", "filter "+a.fq, "↑↓", "select", "enter", "mention", "tab", "chat", "esc", "chat")
	}
	if a.hit >= 0 {
		return "NORMAL", a.normalFooter(), hintKeys("m", "message", "a", "reply", "A", "quote", "e", "edit", "r", "react", "T", "thread", "y", "yank", "v", "open", ".", "repeat", "D", "delete", "esc", "unselect")
	}
	if zen {
		return "NORMAL", a.normalFooter(), ""
	}
	return "NORMAL", a.footer(), keyHint("SPC", "menu", "?", "help", ":", "command", "h l", "channel", "C-p C-n", "thread", "u", "unread", "#", "rooms", "@", "people", "%", "threads", "^", "pins", "'", "recent", "i", "write", "f", "find", "↑↓", "line")
}

// normalFooter is the input of a NORMAL-keyed row. zen puts SPC menu and ? help ahead of the footer, so the
// sponsor overlay covers the states, not the keys.
func (a App) normalFooter() string {
	if zen {
		return keyHint("SPC", "menu", "?", "help") + "   " + a.footer()
	}
	return a.footer()
}

// footer is the last row outside INSERT: more keys and the toggle states, since
// there is no input to show (vic 2026-10-23).
func (a App) footer() string {
	// states first: on a narrow screen the keys fall off the end (they are in ? help)
	thread := "thread:" + cmp.Or(threadState(a.thread), "main")
	mentions := "mentions:" + onOff(a.mentions)
	stamps := "time:" + onOff(a.stamps)
	speakers := "speaker:" + cmp.Or(strings.Join(a.speakers, ","), "all")
	grep := "grep:" + cmp.Or(a.grep, "off")
	if zen { // zen's one row: z, then the state values, not the usage hints (vic 2026-10-27)
		return keyHint("z", "zen:"+onOff(zen)) + "  " + dimmed.Render(strings.Join([]string{thread, mentions, stamps, speakers, grep}, "  "))
	}
	return fit(cell, " "+keyHint("t", thread, "M", mentions, "SPC v s", stamps, "S", speakers, "C-g", grep, "/", "search", "J K", "turns", "e", "edit", "O", "thread reply", "z", "zen:"+onOff(zen), ":", "command"), max(a.w, 1)) // one row
}

// View is the full-screen frame; screen is its text.
func (a App) View() tea.View {
	v := tea.NewView(a.screen())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // wheel comes as MouseWheelMsg, so arrows stay keys
	return v
}

// screen is the chat screen, with the help box or the picker drawn over it.
func (a App) screen() string {
	screen := a.chatScreen()
	switch {
	case a.img != "":
		return overlayCentered(screen, a.w, a.h, imgBox(a.img))
	case a.verifySAS != "":
		return overlayCentered(screen, a.w, a.h, verifyBox(a.verifySAS))
	case a.help:
		return overlayCentered(screen, a.w, a.h, helpBox(a.h, a.helpOff))
	case a.pick && a.pickWhat == pickGrep:
		return overlayCentered(screen, a.w, a.h, grepBox(a.w, a.pq.Value()))
	case a.pick:
		titles := map[pickKind]string{pickChans: "open chat", pickThreads: "open thread", pickUsers: "open user", pickLines: "search", pickSpeakers: "speaker", pickInvite: "invite to " + a.m.Current().Name, pickFiles: "file", pickFileBlock: "file block", pickGrep: "grep", pickRecent: "recent", pickPins: "pinned", pickEdits: "edit history", pickExplore: "explore", pickFound: "search results", pickInfo: a.listTitle, pickHelp: "help"}
		what, ok := titles[a.pickWhat]
		if !ok {
			what = "pick"
		}
		return overlayCentered(screen, a.w, a.h, pickBox(a.w, a.h, what, a.pq.Value(), a.matches(), a.psel, a.pickWhat == pickSpeakers, a.shownMarks()))
	}
	return screen
}

// chatScreen is the scrollback, input and modeline, with the completion and toasts floating over them.
func (a App) chatScreen() string {
	label, input, hint := a.inputLine()
	ml := model.Status(a.m, label, a.info, a.now())
	if n := len(a.pins[a.m.Current().Name]); n > 0 && a.m.Current().Kind == model.RoomChan { // room stats on the frame border
		ml.Left += " · " + strconv.Itoa(n) + " pins"
		if t := a.lastPinText(); t != "" {
			ml.Left += ": " + t
		}
	}
	screen := render(a.m, ml, input, a.w, a.h, a.off(), a.hit, a.sides(), a.lines(a.m.Current()), a.wrap(), nil, hint, a.highlight())
	modeline := a.h - len(inputRows(input, a.w, a.h)) - 1
	screen = overlaySuggest(screen, a.w, modeline-1, a.suggest()) // floats above the prompt: the layout does not move
	screen = overlayToasts(screen, a.w, modeline-1, a.toasts)     // cards sit just above the modeline
	name := a.menu                                                // the which-key menu: one key per row, at the bottom right
	if name == "" && slices.Contains(menuPrefixes, a.pending) {
		name = a.pending // a prefix waiting for its second key lists the continuations
	}
	if name != "" {
		screen = overlayRight(screen, a.w, modeline-1, menuBox(name, menus[name], a.h-4))
	}
	return withSponsor(screen, a.w)
}

// page is the number of scrollback rows View shows; a long input takes rows from it.
func (a App) page() int {
	_, input, _ := a.inputLine()
	rows, sep := a.threadRows()
	return max(paneRows(a.w, a.h, len(inputRows(input, a.w, a.h)))-rows-sep, 1) // the threads box takes rows from the chat
}

// off is the current channel's scroll offset.
func (a App) off() int { return a.offs[a.m.Current().Name] }

// setOff stores ch's offset clamped to [0, lines-page], copy-on-write.
func (a App) setOff(ch string, off int) App {
	n := 0
	for _, c := range a.m.All() {
		if c.Name == ch {
			n = rowCount(a.lines(c), textWidth(a.w, a.h), a.wrap())
		}
	}
	offs := make(map[string]int, len(a.offs)+1)
	for k, v := range a.offs {
		offs[k] = v
	}
	offs[ch] = min(max(off, 0), max(n-a.page(), 0))
	a.offs = offs
	return a
}

// scroll applies a keys.go action to the current channel; Scroll/HalfPage N>0
// means down (toward the newest), so the offset shrinks.
func (a App) scroll(act Action) App {
	ch := a.m.Current().Name
	switch act.Kind {
	case Scroll:
		if a.hit >= 0 { // a marked line: j and k move it
			return a.lineStep(act.N > 0)
		}
		b := a.setOff(ch, a.off()-act.N)
		if act.N < 0 && b.off() == a.off() { // at the top of the loaded lines
			return a.older("scroll")
		}
		return b
	case HalfPage:
		return a.setOff(ch, a.off()-act.N*max(a.page()/2, 1))
	case Top:
		if ls := a.lines(a.m.Current()); a.hit >= 0 && len(ls) > 0 { // a marked line: gg marks the first
			a.hit = 0
			return a.showHit()
		}
		cur := a.m.Current()
		return a.setOff(ch, rowCount(a.lines(cur), textWidth(a.w, a.h), a.wrap()))
	case Bottom:
		if ls := a.lines(a.m.Current()); a.hit >= 0 && len(ls) > 0 { // a marked line: G marks the last
			a.hit = len(ls) - 1
			return a.showHit()
		}
	}
	return a.setOff(ch, 0)
}

// sent applies a finished send: a failure becomes a note, success echoes the line.
func (a App) sent(r SendResultMsg) App {
	if r.Err != nil {
		return a.note("send failed: " + r.Err.Error())
	}
	if a.chanKind(r.Ch) == model.RoomChan {
		return a // room lines are echoed by the room runtime, which also logs them
	}
	if m, err := a.m.AddLine(r.Ch, model.Line{At: a.now(), Nick: a.user, Text: r.Text}); err == nil {
		a.m = m
	}
	return a
}

// WithUser sets the nick shown on the local echo of sent lines (default "me").
func (a App) WithUser(nick string) App {
	a.user = nick
	return a
}

// addLine appends l to channel ch; a scrolled-back channel keeps its view anchored.
func (a App) addLine(ch string, l model.Line) (App, error) {
	m, err := a.m.AddLine(ch, l)
	if err != nil {
		return a, err
	}
	a.m = m
	if a.offs[ch] > 0 && a.shows(a.chanKind(ch), l) {
		a = a.setOff(ch, a.offs[ch]+len(a.wrap()(l, textWidth(a.w, a.h))))
	}
	return a, nil
}

// WithCmd sets the handler for parsed commands (other than /quit). It runs on the
// bubbletea loop, so it must return fast; slow work belongs in the returned tea.Cmd.
func (a App) WithCmd(f func(ch string, c model.Cmd) tea.Cmd) App {
	a.cmdf = f
	return a
}

// shows reports whether l, in a channel of kind k, passes the t and m toggles.
// m (mentions) keeps direct lines only: in a # channel the Chat lines (replies to
// you, your lines), in a & room only lines that @mention you or are yours.
func (a App) shows(k model.ChanKind, l model.Line) bool {
	if a.grep != "" {
		if !strings.Contains(strings.ToLower(l.Text), strings.ToLower(a.grep)) { // case-insensitive substring
			return false
		}
	}
	if len(a.speakers) > 0 && !slices.Contains(a.speakers, l.Nick) {
		return false
	}
	if !a.mentions {
		return true
	}
	if k == model.RoomChan {
		return l.Nick == a.user || mentionsUser(l.Text, a.user)
	}
	return true
}

// mentionsUser reports whether text has @user as a whole mention.
func mentionsUser(text, user string) bool {
	for _, m := range mentionRE.FindAllString(text, -1) {
		if m[1:] == user {
			return true
		}
	}
	return false
}

// lines is the part of c's scrollback the toggles show; render, scroll and search all use it.

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// wrap is the row wrapper the s toggle selects.

func (a App) chanNames() []string {
	var out []string
	for _, c := range a.m.All() {
		out = append(out, c.Name)
	}
	return out
}

// pickKind is what the fuzzy picker lists.
type pickKind uint8

const (
	pickChans     pickKind = iota // ctrl+o: channels, @nicks and threads
	pickLines                     // /: the current channel's shown lines; enter jumps
	pickSpeakers                  // S: who spoke here; enter shows only that nick (vic 2026-10-23)
	pickInvite                    // I: who can be invited to this room; enter invites (vic 2026-10-23)
	pickFiles                     // ctrl+t: the session's files; enter inserts the path
	pickFileBlock                 // ctrl+f: the session's files; enter inserts the file as a fenced block
	pickGrep                      // ctrl+g: filter the shown lines to fuzzy matches, live
	pickRecent                    // m: chats most recent first, then this channel's threads
	pickThreads                   // open thread: this channel's threads
	pickUsers                     // open user: nicks of this channel and agents elsewhere
	pickPins                      // space o p: the room's pinned messages
	pickEdits                     // message menu h: the selected message's earlier versions, read only
	pickExplore                   // :explore: the server's public rooms; enter joins the picked one
	pickFound                     // :search: the server's matches in this room; enter selects the message if loaded
	pickInfo                      // a read-only list (ListMsg, e.g. :devices): enter and esc close it
	pickThemes                    // space T, :theme: the color schemes with live preview; enter keeps, esc restores
	pickHelp                      // ?: every key and command with its description; type to filter, enter and esc close it
)

// pickLast is the last pickKind; tests walk 0..pickLast so a new kind is rendered by the tests too.
const pickLast = pickHelp

// allSpeakers is the speaker picker's first entry: it clears the filter.
const allSpeakers = "(all)"

// shownMarks is the S picker's checked entries: (all) when no nick is marked, else the nicks.
func (a App) shownMarks() []string {
	if len(a.marks) == 0 {
		return []string{allSpeakers}
	}
	return a.marks
}

// openPick opens the fuzzy picker over k.
func (a App) openPick(k pickKind) App {
	a.pick, a.pickWhat, a.psel, a.pq = true, k, 0, textinput.New()
	a.marks = slices.Clone(a.speakers) // S starts with the current filter marked
	a.pq.Prompt = ""
	a.pq.Focus()
	return a
}

// touched puts name first in the most-recently-used list.
func (a App) touched(name string) App {
	a.mru = append([]string{name}, slices.DeleteFunc(slices.Clone(a.mru), func(n string) bool { return n == name })...)
	return a
}

// recentChans is the channel names most recently focused first, then the rest in list order.
func (a App) recentChans() []string {
	all := a.chanNames()
	var out []string
	for _, n := range a.mru {
		if slices.Contains(all, n) {
			out = append(out, n)
		}
	}
	for _, n := range all {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// recentThreads is the heads opened most recently first, then the rest newest first.
func (a App) recentThreads(heads []model.Thread) []model.Thread {
	out := threadsRecentFirst(heads)
	visit := func(h model.Thread) int { return slices.Index(a.tmru, h.Root) }
	slices.SortStableFunc(out, func(x, y model.Thread) int {
		switch vx, vy := visit(x), visit(y); {
		case vx >= 0 && vy >= 0:
			return vx - vy
		case vx >= 0:
			return -1
		case vy >= 0:
			return 1
		}
		return 0
	})
	return out
}

// threadsRecentFirst is the heads newest first by their newest reply; the popups list them so.
func threadsRecentFirst(heads []model.Thread) []model.Thread {
	out := slices.Clone(heads)
	slices.SortStableFunc(out, func(a, b model.Thread) int { return b.At.Compare(a.At) })
	return out
}

// pickSource is what the picker searches: channels and @nicks, "<nick> text" per
// shown line, or the distinct speakers of the current channel, newest first.
func (a App) pickSource() []string {
	var out []string
	switch a.pickWhat {
	case pickRecent:
		out = a.recentChans()
		for _, h := range a.recentThreads(a.threads[a.m.Current().Name]) {
			out = append(out, threadLabel(h))
		}
	case pickChans:
		out = a.chanNames()
	case pickExplore:
		out = a.explore
	case pickFound:
		out = a.found
	case pickThemes:
		for _, id := range a.themeIDs {
			out = append(out, themeLabel(id))
		}
	case pickInfo:
		out = a.list
	case pickHelp:
		for _, s := range helpSections {
			for _, k := range s.keys {
				out = append(out, s.head+"  "+k[0]+"  "+k[1])
			}
		}
	case pickEdits:
		l, _ := a.lastWithID(a.hit)
		out = append(slices.Clone(a.edits[l.ID]), l.Text+"  (now)")
	case pickPins:
		cur := a.m.Current()
		for _, id := range a.pins[cur.Name] {
			out = append(out, pinLabel(a.lines(cur), id))
		}
	case pickThreads:
		for _, h := range threadsRecentFirst(a.threads[a.m.Current().Name]) {
			out = append(out, ansi.Strip(threadStamp(h.At, h, a.unreadOf(h)))+threadLabel(h)) // plain text: the picker box matches it rune by rune // time, then the summary columns, as in the threads box
		}
	case pickUsers:
		cur := a.m.Current()
		nicks := chanNicks(cur, a.user)[1:] // this channel: plain nicks
		for _, c := range a.m.All() {       // other sessions: agents only as agent@session, never plain
			if c.Kind == model.SessionChan && c.Name != cur.Name {
				for _, ag := range c.Agents {
					nicks = append(nicks, ag.Nick+"@"+strings.TrimPrefix(c.Name, "#"))
				}
			}
		}
		for _, n := range nicks {
			if !slices.Contains(out, "@"+n) {
				out = append(out, "@"+n)
			}
		}
	case pickLines:
		for _, l := range a.lines(a.m.Current()) {
			out = append(out, "<"+l.Nick+"> "+l.Text)
		}
	case pickFiles, pickFileBlock:
		out = a.files
	case pickInvite: // every session's main agent and its agents as agent@session, minus members
		cur := a.m.Current()
		for _, c := range a.m.All() {
			if c.Kind != model.SessionChan {
				continue
			}
			s := strings.TrimPrefix(c.Name, "#")
			for _, n := range append([]string{s}, agentsAt(c, s)...) {
				if !slices.ContainsFunc(cur.Members, func(m model.Member) bool { return m.Nick == n }) {
					out = append(out, n)
				}
			}
		}
	case pickSpeakers:
		out = []string{allSpeakers}
		cur := a.m.Current()
		ls := cur.Lines.All()
		for i := len(ls) - 1; i >= 0; i-- {
			if !slices.Contains(out, ls[i].Nick) {
				out = append(out, ls[i].Nick)
			}
		}
		for _, n := range chanNicks(cur, a.user) { // then everyone in the pane who has not spoken yet
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

// pickKey runs one key in the open picker: enter goes to the selected channel or line.
func (a App) pickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	ms := a.matches()
	if a.pickWhat == pickUsers && msg.String() == "enter" && a.psel < len(ms) { // a user: enter opens the DM
		a.pick = false
		return a.runCmd(":dm " + ms[a.psel].name)
	}
	if a.pickWhat == pickThreads && msg.String() == "enter" && a.psel < len(ms) { // a head: by its place in the list
		a.pick = false
		heads := threadsRecentFirst(a.threads[a.m.Current().Name])
		if idx := ms[a.psel].idx; idx < len(heads) {
			return a.selectThread(heads[idx].Root), nil
		}
		return a, nil
	}
	if a.pickWhat == pickEdits && (msg.String() == "enter" || msg.String() == "esc") { // read only: enter closes it too
		a.pick = false
		return a, nil
	}
	if (a.pickWhat == pickInfo || a.pickWhat == pickHelp) && (msg.String() == "enter" || msg.String() == "esc") { // read only
		a.pick = false
		return a, nil
	}
	if a.pickWhat == pickFound && (msg.String() == "enter" || msg.String() == "esc") {
		a.pick = false
		if msg.String() == "esc" || a.psel >= len(ms) {
			return a, nil
		}
		id := a.foundIDs[ms[a.psel].idx]
		for i, l := range a.lines(a.m.Current()) {
			if l.ID == id {
				a.hit = i
				return a.showHit(), nil
			}
		}
		return a.toast("not loaded: scroll back to load it", true), nil
	}
	if a.pickWhat == pickExplore && (msg.String() == "enter" || msg.String() == "esc") {
		a.pick = false
		if msg.String() == "esc" || a.psel >= len(ms) {
			return a, nil
		}
		return a.runCmd(":join " + a.exploreTo[ms[a.psel].idx])
	}
	if a.forwarding && a.pickWhat == pickChans && (msg.String() == "enter" || msg.String() == "esc") {
		a.forwarding, a.pick = false, false
		if msg.String() == "esc" || a.psel >= len(ms) {
			return a, nil
		}
		return a.forward(ms[a.psel].name)
	}
	switch msg.String() {
	case "esc":
		if a.pickWhat == pickGrep { // esc clears the grep filter
			a.grep, a.hit, a.offs = "", -1, nil
		}
		a.pick = false
	case "enter":
		if a.psel < len(ms) && a.pickWhat == pickPins {
			a.pick = false
			return a.jumpToPin(ms[a.psel].idx), nil
		}
		if a.psel < len(ms) && a.pickWhat == pickFileBlock {
			a.pick = false
			return a, readFileCmd(a.m.Current().Cwd, ms[a.psel].name)
		}
		if a.psel < len(ms) && a.pickWhat == pickFiles {
			if a.prompt == 0 { // a : or $ prompt keeps its own mode
				a.mode = Insert
			}
			a.in.SetValue(a.in.Value() + ms[a.psel].name + " ")
			a.in.MoveToEnd()
		} else if a.psel < len(ms) && a.pickWhat == pickInvite {
			a.pick = false
			return a.runCmd("/invite " + ms[a.psel].name + " " + a.m.Current().Name)
		}
		if a.pickWhat == pickSpeakers { // an unchecked result alone; a checked one applies the checks
			speakers := slices.Clone(a.marks)
			if a.psel < len(ms) && !slices.Contains(a.shownMarks(), ms[a.psel].name) {
				speakers = []string{ms[a.psel].name}
				if speakers[0] == allSpeakers {
					speakers = nil
				}
			}
			a.speakers, a.hit, a.offs = speakers, -1, nil // positions index the shown lines, which changed
			a.pick = false
			return a, nil
		}
		if a.psel < len(ms) && a.pickWhat == pickLines {
			a.hit, a.query = ms[a.psel].idx, a.pq.Value() // n / N go on from here
			a = a.showHit()
		} else if a.psel < len(ms) {
			_, isChan := a.chanByName(ms[a.psel].name)               // a channel first: a thread label never shadows it
			if h, ok := a.threadAt(ms[a.psel].name); ok && !isChan { // a thread: the chat pane shows it
				a = a.selectThread(h.Root)
			} else if nick, ok := strings.CutPrefix(ms[a.psel].name, "@"); ok && !(isChan && a.pickWhat == pickRecent) { // a DM row of the recent picker is a chat: focus it
				a.mode = Insert
				a.in.SetValue(a.in.Value() + "@" + nick + " ")
				a.in.MoveToEnd()
			} else {
				a = a.focusName(ms[a.psel].name)
			}
		}
		a.pick = false
	case "space": // S picker: mark or unmark the selected nick; (all) clears the marks. Other pickers type a space.
		if a.pickWhat != pickSpeakers {
			var cmd tea.Cmd
			a.pq, cmd = a.pq.Update(msg)
			a.psel = 0
			a = a.grepSync()
			return a, cmd
		}
		if a.pickWhat == pickSpeakers && a.psel < len(ms) {
			n := ms[a.psel].name
			if n == allSpeakers {
				a.marks = nil
			} else if slices.Contains(a.marks, n) {
				a.marks = slices.DeleteFunc(slices.Clone(a.marks), func(s string) bool { return s == n })
			} else {
				a.marks = append(slices.Clone(a.marks), n)
			}
		}
	case "up", "ctrl+p":
		a.psel = max(a.psel-1, 0)
	case "down", "ctrl+n":
		a.psel = max(min(a.psel+1, len(ms)-1), 0)
	default:
		var cmd tea.Cmd
		a.pq, cmd = a.pq.Update(msg)
		a.psel = 0       // a new query starts at its best match
		a = a.grepSync() // grep filters live as you type
		return a.pickHit(), cmd
	}
	return a.pickHit(), nil
}

// pickHit makes the selected search match the current hit, scrolled into view.
func (a App) pickHit() App {
	if a.pickWhat != pickLines {
		return a
	}
	if ms := a.matches(); a.psel < len(ms) {
		a.hit = ms[a.psel].idx
		return a.showHit()
	}
	return a
}

// firstURL is the first link in s: a markdown link's URL, else the first word that starts with http.
func firstURL(s string) string {
	if m := mdLinkRE.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	for _, w := range strings.Fields(s) {
		w = strings.Trim(w, "\"'()[]<>,;")
		if strings.HasPrefix(w, "https://") || strings.HasPrefix(w, "http://") {
			return w
		}
	}
	return ""
}

// forward sends the selected line (else the newest) to channel to, quoted with its sender.
func (a App) forward(to string) (App, tea.Cmd) {
	ls := a.lines(a.m.Current())
	i := a.hit
	if i < 0 || i >= len(ls) {
		i = len(ls) - 1
	}
	if i < 0 {
		return a.toast("forward: nothing to forward", true), nil
	}
	if _, ok := a.chanByName(to); !ok {
		return a.toast("forward: no chat "+to, true), nil
	}
	text := "↪ " + ls[i].Nick + ": " + ls[i].Text
	send := a.send
	return a.toast("forwarded to "+to, false), func() tea.Msg { return SendResultMsg{Ch: to, Text: text, Err: send(to, text)} }
}

// bracket moves the cursor to the next (fwd) or previous line matching key: ] speaker change, m mine,
// @ mentions me, l link or media, p pinned; t opens the next or previous thread; u the next channel with unread.
func (a App) bracket(fwd bool, key string) App {
	switch key {
	case "t":
		return a.stepThread(fwd)
	case "u":
		return a.nextUnreadChan(fwd)
	case "c": // ]c and [c: the channel pane's next and previous channel, as h and l
		if fwd {
			return a.paneChan(Action{Kind: PaneNext})
		}
		return a.paneChan(Action{Kind: PanePrev})
	case "]", "[":
		return a.turn(fwd)
	}
	ls := a.lines(a.m.Current())
	pins := map[string]bool{}
	for _, id := range a.pins[a.m.Current().Name] {
		pins[id] = true
	}
	match := map[string]func(model.Line) bool{
		"m": func(l model.Line) bool { return l.Nick == a.user },
		"@": func(l model.Line) bool { return mentionsUser(l.Text, a.user) },
		"l": func(l model.Line) bool { return firstURL(l.Text) != "" || strings.HasPrefix(l.Text, "[") },
		"p": func(l model.Line) bool { return pins[l.ID] },
	}[key]
	if match == nil {
		return a
	}
	step, i := 1, a.hit
	if !fwd {
		step = -1
	}
	if i < 0 || i >= len(ls) {
		i = len(ls)
		if fwd {
			i = -1
		}
	}
	for j := i + step; j >= 0 && j < len(ls); j += step {
		if match(ls[j]) {
			a.hit = j
			return a.showHit()
		}
	}
	if key == "p" { // no loaded pin that way: fetch the nearest one that is not loaded
		if id := a.unloadedPin(fwd); id != "" {
			return a.loadPin(id)
		}
	}
	if !fwd {
		return a.older(key)
	}
	return a.toast("no more", false)
}

// stepThread opens the next (fwd) or previous thread head; past the last or first, the main chat.
func (a App) stepThread(fwd bool) App {
	rows := a.threadOrder()
	cur := slices.IndexFunc(rows, func(r threadSlot) bool { return r.root == a.thread })
	if cur < 0 { // the current thread is not in the pane's rows: nothing moves
		return a
	}
	next := cur - 1
	if fwd {
		next = cur + 1
	}
	if next < 0 || next >= len(rows) { // the ends stop: no wrap
		return a
	}
	return a.selectThread(rows[next].root)
}

// threadSlot is a thread in the threads pane's order: its root ("" for the main thread) and its label.
type threadSlot struct{ root, label string }

// threadOrder are the threads in the pane's order, unfiltered: the heads in stored order, then the main thread.
func (a App) threadOrder() []threadSlot {
	heads := a.threads[a.m.Current().Name]
	slots := make([]threadSlot, 0, len(heads)+1)
	for _, h := range heads {
		slots = append(slots, threadSlot{h.Root, threadLabel(h)})
	}
	return append(slots, threadSlot{"", mainThreadLabel})
}

// nextUnreadChan focuses the next (fwd) or previous channel with unread messages, wrapping around.
func (a App) nextUnreadChan(fwd bool) App {
	names := a.chanNames()
	if len(names) == 0 {
		return a
	}
	idx := slices.Index(names, a.m.Current().Name)
	step := -1
	if fwd {
		step = 1
	}
	for k := 1; k <= len(names); k++ {
		j := ((idx+step*k)%len(names) + len(names)) % len(names)
		if c, ok := a.chanByName(names[j]); ok && c.Unread > 0 {
			return a.focusName(names[j])
		}
	}
	return a.toast("no unread chats", false)
}

// applyPending sets the reactions that arrived before their line, for the lines of ch that have arrived now.
func (a App) applyPending(ch string) App {
	for id, reacts := range a.pendingReacts[ch] {
		if a.m.HasLine(ch, id) {
			if m, err := a.m.SetReacts(ch, id, reacts); err == nil {
				a.m = m
			}
			delete(a.pendingReacts[ch], id)
		}
	}
	return a
}

// setSeen sets the seen-by summary of the line with event ID id in channel ch: the readers whose latest read it is.
func (a App) setSeen(ch, id string) App {
	var nicks []string
	for n, rid := range a.readBy[ch] {
		if rid == id {
			nicks = append(nicks, n)
		}
	}
	slices.Sort(nicks)
	if m, err := a.m.SetSeen(ch, id, strings.Join(nicks, ", ")); err == nil {
		a.m = m
	}
	return a
}

// seenBy lists the readers whose read reaches the selected message or past it: the message menu's seen by.
func (a App) seenBy() App {
	cur := a.m.Current().Name
	ls := a.lines(a.m.Current())
	if a.hit < 0 || a.hit >= len(ls) {
		return a.toast("seen by: select a message first", true)
	}
	var nicks []string
	for n, rid := range a.readBy[cur] {
		for j, l := range ls {
			if l.ID == rid && j >= a.hit {
				nicks = append(nicks, n)
			}
		}
	}
	slices.Sort(nicks)
	if len(nicks) == 0 {
		nicks = []string{"nobody yet"}
	}
	a.listTitle, a.list = "seen by", nicks
	return a.openPick(pickInfo)
}

// gotoDate selects the first shown line at or after when (a date, a date and time, a time today, or "yesterday").
func (a App) gotoDate(s string) App {
	now := a.now()
	var t time.Time
	var err error
	switch {
	case s == "yesterday":
		y := now.AddDate(0, 0, -1)
		t = time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, now.Location())
	case len(s) == 5:
		var hm time.Time
		if hm, err = time.ParseInLocation("15:04", s, now.Location()); err == nil {
			t = time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
		}
	case len(s) == 10:
		t, err = time.ParseInLocation("2006-01-02", s, now.Location())
	default:
		t, err = time.ParseInLocation("2006-01-02 15:04", s, now.Location())
	}
	if err != nil {
		return a.toast("goto: want 2006-01-02, 2006-01-02 15:04, 15:04 or yesterday", true)
	}
	for i, l := range a.lines(a.m.Current()) {
		if !l.At.Before(t) {
			a.hit = i
			return a.showHit()
		}
	}
	return a.toast("goto: no loaded message after "+s, false)
}

// jumpUnread selects the first shown line after my read marker and scrolls to it.
func (a App) jumpUnread() App {
	ls := a.lines(a.m.Current())
	id := a.fullyRead[a.m.Current().Name]
	for i, l := range ls {
		if id != "" && l.ID == id {
			if i+1 < len(ls) {
				a.hit = i + 1
				return a.showHit()
			}
			return a.toast("no unread messages", false)
		}
	}
	return a.toast("read marker not loaded here", true)
}

// showHit scrolls so the first row of the hit line is the top row of the lines pane.
func (a App) showHit() App {
	ls := a.lines(a.m.Current())
	if a.hit < 0 || a.hit >= len(ls) {
		return a
	}
	after := rowCount(ls[a.hit:], textWidth(a.w, a.h), a.wrap()) // rows from the hit line to the newest
	return a.setOff(a.m.Current().Name, max(after-a.page(), 0))
}

// nickPool is every nick tab can complete: the current channel's agents and members,
// then every session main agent and its agents as agent@session.
func (a App) nickPool() []string {
	cur := a.m.Current()
	var out []string // the current channel's nicks first: tab offers them first
	for _, ag := range cur.Agents {
		out = append(out, ag.Nick)
	}
	for _, m := range cur.Members {
		out = append(out, m.Nick)
	}
	for _, c := range a.m.All() {
		if c.Kind == model.SessionChan {
			s := strings.TrimPrefix(c.Name, "#")
			out = append(out, s)
			for _, ag := range c.Agents { // teammate outside its channel: agent@session (D:229)
				out = append(out, ag.Nick+"@"+s)
			}
		}
	}
	return out
}

// suggest is the completion row above an INSERT line (0 or 1 row): what tab puts in.
func (a App) suggest() []string {
	if a.reactRow {
		var parts []string
		for i, e := range quickReacts {
			parts = append(parts, shortcut.Bold(true).Render(strconv.Itoa(i+1))+" "+e)
		}
		return []string{dimmed.Render("react ") + strings.Join(parts, "  ") + dimmed.Render("  esc cancel")}
	}
	if a.prompt == promptCmd { // the : line: the commands that match the word being typed, with what they do
		word, rest, _ := strings.Cut(a.in.Value(), " ")
		if word == "" || rest != "" || strings.HasPrefix(word, "!") {
			return nil
		}
		var parts []string
		for _, w := range model.CmdWords {
			if strings.HasPrefix(w, word) && w != "rm" && w != "q" {
				parts = append(parts, shortcut.Render(model.Usage(w))+dimmed.Render("  "+cmdDesc(w)))
			}
			if len(parts) == 3 {
				break
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return []string{ansi.Truncate(strings.Join(parts, dimmed.Render("  ·  ")), max(a.w-6, 1), "…")}
	}
	if a.mode != Insert || a.prompt != 0 {
		return nil
	}
	cs := candidates(a.in.Value(), a.chanNames(), a.nickPool(), true)
	if len(cs) == 0 {
		return nil
	}
	_, word := lastWord(a.in.Value())
	var parts []string
	for i, c := range cs {
		st := dimmed
		if i == 0 {
			st = cell.Bold(true)
		}
		if !strings.HasPrefix(c, word) { // an emoji for a :shortcode: no typed prefix to bold
			parts = append(parts, st.Render(c))
			continue
		}
		parts = append(parts, shortcut.Bold(true).Render(c[:len(word)])+st.Render(c[len(word):]))
	}
	return []string{ansi.Truncate(dimmed.Render("tab ")+strings.Join(parts, "  "), max(a.w-6, 1), "…")} // fits in the box
}

// chanKind is the kind of channel name in the model; a name not there is a room.
func (a App) chanKind(name string) model.ChanKind {
	for _, c := range a.m.All() {
		if c.Name == name {
			return c.Kind
		}
	}
	return model.RoomChan
}

// mainLast is the newest main-thread line of the current channel as "nick: text" (one line), "" if none.
func (a App) mainLast() (string, time.Time) {
	c := a.m.Current()
	for i := c.Lines.Len() - 1; i >= 0; i-- {
		if l := c.Lines.At(i); l.Thread == "" {
			s := l.Text
			if l.Nick != "" {
				s = l.Nick + ": " + s
			}
			return strings.ReplaceAll(s, "\n", " "), l.At
		}
	}
	return "", time.Time{}
}

// chanByName is the channel named name.
func (a App) chanByName(name string) (model.Channel, bool) {
	for _, c := range a.m.All() {
		if c.Name == name {
			return c, true
		}
	}
	return model.Channel{}, false
}

// lastMention is the newest @nick in the current channel's shown lines that is
// not the user ("" if none); O starts a reply to it.
func (a App) lastMention() string {
	ls := a.lines(a.m.Current())
	for i := len(ls) - 1; i >= 0; i-- {
		ms := mentionRE.FindAllString(ls[i].Text, -1)
		for j := len(ms) - 1; j >= 0; j-- {
			if n := ms[j][1:]; n != a.user {
				return n
			}
		}
	}
	return ""
}

// Status marks before each nick in the nick pane (vic 2026-10-23). All are wide
// (2 cells) emoji with no variation selector, so widths stay exact.
const (
	busyMark   = "⌛" // working: registry busy, or a subagent mid-turn
	typingMark = "💬" // you, while your INSERT line has text
	idleMark   = "💤" // idle, or not live
)

// statusMarks lists every mark, for the view to split it from the nick.
// quickReacts is the + row: + then 1-9 reacts to the cursor line with that emoji.
var quickReacts = []string{"👍", "❤️", "😂", "🎉", "😮", "😢", "🙏", "🔥", "👀"}

var statusMarks = []string{busyMark, typingMark, idleMark, presenceOnline, presenceAway, presenceOffline}

// Presence dots before a nick (PresenceMsg): green online, yellow away, dim offline. Each is one glyph
// with a space, so the view can split it from the nick and style it.
const (
	presenceOnline  = "● "
	presenceAway    = "◐ "
	presenceOffline = "○ "
)

// presenceMark is the dot of a presence state; "" for a nick with none.
func presenceMark(state string) string {
	switch state {
	case "online":
		return presenceOnline
	case "unavailable":
		return presenceAway
	case "offline":
		return presenceOffline
	}
	return ""
}

// paneNicks is chanNicks with a status mark before each nick.
func (a App) paneNicks() []string {
	cur := a.m.Current()
	out := chanNicks(cur, a.user)
	busy := func(pid int) bool { return a.info[pid].Status == "busy" }
	mark := func(b bool) string {
		if b {
			return busyMark
		}
		return idleMark
	}
	out[0] = mark(false) + out[0]
	if a.mode == Insert && a.in.Value() != "" {
		out[0] = typingMark + chanNicks(cur, a.user)[0]
	}
	for i := 1; i < len(out); i++ {
		switch {
		case cur.Kind == model.RoomChan:
			nick := cur.Members[i-1].Nick
			switch p := presenceMark(a.presence[nick]); {
			case busy(a.pidOf(nick)):
				out[i] = busyMark + out[i]
			case slices.Contains(a.typing[cur.Name], nick):
				out[i] = typingMark + out[i]
			case p != "": // a dot for a nick we know the presence of
				out[i] = p + out[i]
			default:
				// no presence known: no mark
			}
		case i == 1: // the main agent
			out[i] = mark(busy(cur.PID)) + out[i]
		default:
			out[i] = mark(cur.Agents[i-2].Busy) + out[i]
		}
	}
	return out
}

// pidOf is the session pid behind a member nick: a main agent nick is its # channel
// name without "#", a teammate "x@s" belongs to session s (D:229). 0 if none.
func (a App) pidOf(nick string) int {
	if _, s, ok := strings.Cut(nick, "@"); ok {
		nick = s
	}
	for _, c := range a.m.All() {
		if c.Kind == model.SessionChan && c.Name == "#"+nick {
			return c.PID
		}
	}
	return 0
}

// turn moves the cursor (the reversed line) to the next (fwd) or previous start of a
// speaker run: the first line whose nick differs from the line before it. With no
// cursor, fwd starts from the oldest line and back from the newest.
func (a App) turn(fwd bool) App {
	ls := a.lines(a.m.Current())
	start := func(i int) bool { return i == 0 || ls[i].Nick != ls[i-1].Nick }
	i, step := a.hit, 1
	if !fwd {
		step = -1
	}
	if i < 0 || i >= len(ls) {
		i = len(ls)
		if fwd {
			i = -1
		}
	}
	for j := i + step; j >= 0 && j < len(ls); j += step {
		if start(j) {
			a.hit = j
			return a.showHit()
		}
	}
	if !fwd {
		return a.older("[")
	}
	return a
}

// paneFocus is the pane that takes NORMAL keys; tab cycles it (vic 2026-10-22).
type paneFocus uint8

const (
	focusChat paneFocus = iota
	focusChans
	focusNicks
	focusThreads // the thread heads at the top of the chat column (tab cycles here too)
)

// paneItems is the focused side pane's rows: names (channels, or nicks with busy
// marks) filtered fuzzily by fq, and the plain names to act on. Unfocused or
// with no query, every row in pane order.
func (a App) paneItems(f paneFocus) (show, names []string) {
	if f == focusThreads {
		names = nil // the heads, then the main thread last
		for _, r := range a.threadOrder() {
			names = append(names, r.label)
		}
		show = names
	} else if f == focusChans {
		names = a.chanRows()
		if f == a.focus && strings.TrimSpace(a.fq) != "" {
			names = a.chanNames() // a filter matches channel names, never group labels
		}
		show = names
	} else {
		names = chanNicks(a.m.Current(), a.user)
		show, _ = a.nickRows()
	}
	if f != a.focus || strings.TrimSpace(a.fq) == "" {
		return show, names
	}
	var fs, fn []string
	for _, m := range pickList(names, a.fq) {
		fs, fn = append(fs, show[m.idx]), append(fn, names[m.idx])
	}
	return fs, fn
}

// paneKey is paneStep plus the channel preview: moving over the channel pane shows
// that channel in the chat and nick panes (vic 2026-10-23).
func (a App) paneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, cmd := a.paneStep(msg)
	b := next.(App)
	if k := msg.String(); b.focus == focusChans && k != "esc" && k != "enter" && k != "tab" {
		if _, names := b.paneItems(focusChans); b.fsel < len(names) {
			b = b.focusName(names[b.fsel])
		}
	}
	return b, cmd
}

// paneStep runs one NORMAL key while a side pane has focus: type to filter, up/down
// to select, enter to join a channel or start a line mentioning a nick, tab to the
// next pane, esc back to chat.
func (a App) paneStep(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	_, names := a.paneItems(a.focus)
	switch k := msg.String(); k {
	case "esc":
		a.focus = focusChat
	case "tab":
		a.focus, a.fq, a.fsel = a.nextPane(), "", 0
	case "up", "ctrl+p":
		a.fsel = max(a.fsel-1, 0)
	case "down", "ctrl+n":
		a.fsel = max(min(a.fsel+1, len(names)-1), 0)
	case "backspace":
		if r := []rune(a.fq); len(r) > 0 {
			a.fq, a.fsel = string(r[:len(r)-1]), 0
		}
	case "enter":
		if a.fsel >= len(names) {
			return a, nil
		}
		if a.focus == focusThreads {
			if names[a.fsel] == mainThreadLabel {
				a = a.selectThread("")
			} else if h, ok := a.threadAt(names[a.fsel]); ok {
				a = a.selectThread(h.Root)
			}
		} else if a.focus == focusChans {
			a = a.focusName(names[a.fsel])
		} else {
			a = a.mention(names[a.fsel])
		}
		a.focus = focusChat
	default:
		if msg.Text != "" {
			a.fq, a.fsel = a.fq+msg.Text, 0
		}
	}
	return a.keepSelVisible(), nil
}

// click handles a left click in wide mode: a channel row switches to that channel,
// a nick row starts an insert line mentioning it (vic 2026-10-23). Row 0 is the pane title.
func (a App) click(m tea.Mouse) (tea.Model, tea.Cmd) {
	if rows, sep := a.threadRows(); wide(a.w, a.h) && m.X >= chanW+2 && m.X < a.w-nickW-2 && len(a.pins[a.m.Current().Name]) > 0 {
		if (rows == 0 && m.Y == 0) || (sep == 1 && m.Y == rows+1) { // the frame title with the pins: go to the newest pin
			return a.jumpToPin(len(a.pins[a.m.Current().Name]) - 1), nil
		}
	}
	if !wide(a.w, a.h) || m.Y < 1 {
		return a, nil
	}
	i := m.Y - 1
	switch {
	case m.X < chanW+2: // the channel pane and its borders
		if _, names := a.paneItems(focusChans); i < len(names) {
			a = a.focusName(names[i])
		}
	case m.X >= a.w-nickW-2: // the nick pane and its borders
		if _, names := a.paneItems(focusNicks); i < len(names) {
			a = a.mention(names[i])
		}
	default: // the lines pane: a file or `command` opens, else a click replies to that line's nick
		rows, sep := a.threadRows()
		if i < rows { // a threads box row: open that thread (the last row is the main thread)
			heads := a.threads[a.m.Current().Name]
			top := threadTopOf(len(heads)+1, a.fsel, a.focus == focusThreads, a.tscroll)
			if j := top + i; j < len(heads) {
				a = a.selectThread(heads[j].Root)
			} else if j == len(heads) {
				a = a.selectThread("")
			}
			return a, nil
		}
		l, idx, row, ok := a.lineAt(i - rows - sep)
		if !ok {
			break
		}
		if key := a.reactKeyAt(l, row, m.X-chanW-3); key != "" && l.ID != "" { // a reaction: toggle mine
			if c, err := model.ParseCmd("/react " + key + " " + l.ID); err == nil {
				return a, func() tea.Msg { return CmdMsg{c} }
			}
		}
		if url := urlAt(row, m.X-chanW-3); url != "" { // a link opens in the browser
			return a, openURL(url)
		}
		if kind, _, ok := mediaTag(l.Text); ok && l.ID != "" { // a media line: view the image, or open the file
			cmd := "open"
			if kind == "image" {
				cmd = "view"
			}
			if c, err := model.ParseCmd("/" + cmd + " " + l.ID); err == nil {
				return a, func() tea.Msg { return CmdMsg{c} }
			}
		}
		if path, cmd, hit := clickTarget(row, m.X-chanW-3); hit { // the text starts after the border and a space
			return a.openTarget(path, cmd)
		}
		if a.hit != idx { // first click: the line becomes current, as if moved there with up/down
			a.hit = idx
			a.menu = "message"
			return a, nil
		}
		if l.Nick != "" && l.Nick != a.user { // a click on the current line: reply to it
			a = a.mention(l.Nick)
			a.quote = &l
			a.menu = ""
		}
	}
	return a, nil
}

// openTarget opens path in $EDITOR, suspending the app while it runs. A code span (cmd) is copied.
func (a App) openTarget(path, cmd string) (App, tea.Cmd) {
	if cmd != "" {
		return a.toast("copied "+cmd, false), tea.SetClipboard(cmd)
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.m.Current().Cwd, path)
	}
	argv := append(strings.Fields(cmp.Or(strings.TrimSpace(os.Getenv("EDITOR")), "vi")), path)
	return a, tea.ExecProcess(exec.Command(argv[0], argv[1:]...), execDone("editor"))
}

// runShell runs cmd with sh -c, suspending the app. It waits for Enter before the
// app comes back, so the output stays readable (vic 2026-10-23, like jjui $).
func (a App) runShell(cmd string) (App, tea.Cmd) {
	if strings.TrimSpace(cmd) == "" {
		return a, nil
	}
	script := cmd + "\necho; echo 'press enter to return'; read _"
	return a, tea.ExecProcess(exec.Command("sh", "-c", script), execDone("command"))
}

// execDoneMsg reports that a suspended process exited; Err is nil on a clean exit.
type execDoneMsg struct {
	What string
	Err  error
}

// execDone is the callback for tea.ExecProcess that reports the exit as an execDoneMsg.
func execDone(what string) func(error) tea.Msg {
	return func(err error) tea.Msg { return execDoneMsg{What: what, Err: err} }
}

// lineAt is the shown line of lines-pane row i, as render draws it, and the row's plain text.
func (a App) lineAt(i int) (model.Line, int, string, bool) {
	if i < 0 {
		return model.Line{}, -1, "", false
	}
	shown := a.lines(a.m.Current())
	rows, of := tailRows(shown, a.page(), a.off(), textWidth(a.w, a.h), a.wrap())
	if i >= len(rows) {
		return model.Line{}, -1, "", false
	}
	return shown[of[i]], of[i], rows[i], true
}

// mention starts an insert line with @nick, after any text already typed.
func (a App) mention(nick string) App {
	a.mode = Insert
	a.in.SetValue(a.in.Value() + "@" + nick + " ")
	a.in.MoveToEnd()
	return a
}

// lineStep moves the cursor (the reversed line) one line down (to newer) or up,
// and scrolls just enough to keep it in view (vic 2026-10-23). Up with no cursor
// starts at the newest line; down past the newest clears it.
func (a App) lineStep(down bool) App {
	ls := a.lines(a.m.Current())
	w := textWidth(a.w, a.h)
	ch := a.m.Current().Name
	if a.hit >= 0 && a.hit < len(ls) { // a line taller than the pane: j and k scroll through it a row at a time
		after := rowCount(ls[a.hit:], w, a.wrap())
		rows := len(a.wrap()(ls[a.hit], w))
		if rows > a.page() {
			if down && a.off() > after-rows { // its bottom is below the pane
				return a.setOff(ch, a.off()-1)
			}
			if !down && after > a.off()+a.page() { // its top is above the pane
				return a.setOff(ch, a.off()+1)
			}
		}
	}
	switch {
	case len(ls) == 0:
		return a
	case a.hit < 0 || a.hit >= len(ls):
		if down {
			return a
		}
		a.hit = len(ls) - 1
	case down && a.hit == len(ls)-1:
		a.hit = -1
		return a.setOff(a.m.Current().Name, 0)
	case down:
		a.hit++
	default:
		if a.hit == 0 {
			return a.older("line")
		}
		a.hit--
	}
	after := rowCount(ls[a.hit:], w, a.wrap())           // rows from the cursor line's top to the newest
	below := after - len(a.wrap()(ls[a.hit], w))         // rows under the cursor line
	off := min(a.off(), below)                           // its bottom row is on screen
	off = max(off, after-a.page())                       // and its top row
	if !down && len(a.wrap()(ls[a.hit], w)) > a.page() { // a tall line entered from below shows its bottom first
		off = below
	}
	return a.setOff(a.m.Current().Name, off)
}

// withCursor draws the input cursor at rune position pos: that rune reversed, or a
// reversed space at the end (vic 2026-10-23).
func withCursor(s string, pos int) string {
	rs := []rune(s)
	pos = min(max(pos, 0), len(rs))
	if pos == len(rs) {
		return s + cursorMark + " " + cursorEndMark
	}
	if rs[pos] == '\n' { // a cursor on a line end shows as a block, the newline stays
		return string(rs[:pos]) + cursorMark + " " + cursorEndMark + string(rs[pos:])
	}
	return string(rs[:pos]) + cursorMark + string(rs[pos]) + cursorEndMark + string(rs[pos+1:])
}

// cursorMark and cursorEndMark bracket the cursor in input text; inputRows turns them into
// the cursor style and uses the cursor's row to scroll the input.
const (
	cursorMark    = ""
	cursorEndMark = ""
)

// Channel is the current channel's name ("" when there is none).
func (a App) Channel() string { return a.m.Current().Name }

// yank copies the cursor line's text to the system clipboard (OSC 52) (vic 2026-10-23).
func (a App) yank() (tea.Model, tea.Cmd) {
	ls := a.lines(a.m.Current())
	if a.hit < 0 || a.hit >= len(ls) {
		a = a.toast("nothing to yank: ↑ selects a line", true)
		return a, nil
	}
	text := ls[a.hit].Text
	a = a.toast("yanked: "+strings.ReplaceAll(text, "\n", " "), false)
	return a, tea.SetClipboard(text)
}

// agentsAt is c's agents as agent@session (D:229).
func agentsAt(c model.Channel, session string) []string {
	var out []string
	for _, ag := range c.Agents {
		out = append(out, ag.Nick+"@"+session)
	}
	return out
}

func (a App) helpScroll(n int) App {
	a.helpOff = helpOff(helpRows(), a.h, a.helpOff+n)
	return a
}

// cmdUsage is the usage of the /command being typed in INSERT, as hint text: every
// command that matches the first word, or "" when the line is not a command.
func (a App) cmdUsage() string {
	if a.prompt != promptCmd {
		return ""
	}
	word, _, _ := strings.Cut(a.in.Value(), " ")
	rest := word
	if rest == "" || strings.HasPrefix(rest, "!") {
		return ""
	}
	us := model.Usages(rest)
	if len(us) == 0 {
		return alert.Render("unknown command")
	}
	hint := ""
	if len(us) == 1 { // one command matches: say what it is for
		if d := cmdDesc(strings.TrimPrefix(strings.Fields(us[0])[0], ":")); d != "" {
			hint = dimmed.Render("  ·  " + d)
		}
	}
	for i, u := range us {
		us[i] = shortcut.Render(u)
	}
	return dimmed.Render("usage ") + strings.Join(us, dimmed.Render("  ·  ")) + hint
}

// filesMsg is the session's file list for the file finder.
type filesMsg []string

// openFilePick opens the file finder; the list comes from git in the session's directory.
func (a App) openFilePick(k pickKind) (App, tea.Cmd) {
	cwd := a.m.Current().Cwd
	return a.openPick(k), func() tea.Msg { return filesMsg(listFiles(cwd)) }
}

// grepSync sets the ctrl+g filter to the picker's text while the grep picker is open.
func (a App) grepSync() App {
	if a.pickWhat == pickGrep {
		a.grep, a.hit, a.offs = strings.TrimSpace(a.pq.Value()), -1, nil
	}
	return a
}

// shellOutMsg is the finished output of a ! command; Cmd is the command line, Err its exit error, if any.
type shellOutMsg struct {
	Cmd string
	Out []byte
	Err error
}

// runCapture runs cmd with sh -c in the background; its output comes back as a shellOutMsg.
func (a App) runCapture(cmd string) (App, tea.Cmd) {
	if strings.TrimSpace(cmd) == "" {
		return a, nil
	}
	return a, func() tea.Msg {
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		return shellOutMsg{Cmd: cmd, Out: out, Err: err}
	}
}

// insertBlock appends cmd and its output as a sh fenced block, the command as a $ line, and opens INSERT so it can be edited and sent.
func (a App) insertBlock(cmd, text string) App {
	a.mode = Insert
	a = a.insertFence("sh", "$ "+cmd+"\n"+strings.TrimRight(text, "\n"))
	a.in.MoveToEnd()
	return a
}

// histMax is how many sent input lines ctrl+p and ctrl+n step through.
const histMax = 100

// histStep moves the input through sent lines: back (ctrl+p) to older, forward (ctrl+n)
// to newer; past the newest it returns the unsent draft. Not persisted.
func (a App) histStep(back bool) App {
	h := a.hist[a.m.Current().Name]
	if len(h) == 0 || (!back && a.histAt < 0) {
		return a
	}
	if a.histAt < 0 {
		a.histDraft, a.histAt = a.in.Value(), len(h)
	}
	if back {
		a.histAt = max(a.histAt-1, 0)
	} else {
		a.histAt++
	}
	if a.histAt >= len(h) {
		a.histAt = -1
		a.in.SetValue(a.histDraft)
	} else {
		a.in.SetValue(h[a.histAt])
	}
	a.in.MoveToEnd()
	return a
}

// fileBlockMsg is a file read for ctrl+f: Text is its content, Err why it could not be read.
type fileBlockMsg struct {
	Path, Text string
	Err        error
}

// readFileCmd reads name (relative to cwd) in the background for a fenced block.
func readFileCmd(cwd, name string) tea.Cmd {
	return func() tea.Msg {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		b, err := os.ReadFile(path)
		return fileBlockMsg{Path: name, Text: strings.TrimRight(string(b), "\n"), Err: err}
	}
}

// insertFence appends a fenced block to the input, info on its opening line, and opens INSERT.
func (a App) insertFence(info, text string) App {
	a.mode = Insert
	a.in.SetValue(a.in.Value() + "```" + info + "\n" + text + "\n```\n")
	a.in.MoveToEnd()
	return a
}

// highlight is the text whose matches the lines pane marks: the ctrl+g grep, else the / search.
func (a App) highlight() string {
	switch {
	case a.pick && a.pickWhat == pickLines: // the / search: matches light up as you type
		return strings.TrimSpace(a.pq.Value())
	case a.grep != "":
		return a.grep
	}
	return a.query
}

// WithSeen sets the read-marker hook: it runs with a channel's name when the channel is shown
// and a line arrives in it, or when it becomes current. Errors are ignored.
func (a App) WithSeen(f func(ch string) error) App {
	a.seen = f
	return a
}

// WithReply sets the reply hook: a send while a line is quoted (A) goes through it with that line.
func (a App) WithReply(f func(ch string, to model.Line, text string) error) App {
	a.reply = f
	return a
}

// WithPicker opens the ctrl+o channel picker at start.
func (a App) WithPicker() App { return a.openPick(pickChans) }

// progressToast updates the status toast with p.ID, or adds it. A finished one expires like any toast.
func (a App) progressToast(p ProgressMsg) App {
	ts := slices.Clone(a.toasts)
	i := slices.IndexFunc(ts, func(t toast) bool { return t.id == p.ID })
	if i < 0 {
		ts = append(ts, toast{id: p.ID})
		i = len(ts) - 1
	}
	ts[i].text, ts[i].done, ts[i].total = p.Text, p.Done, p.Total
	ts[i].running = !p.Finished
	if p.Finished {
		ts[i].at = a.now()
	}
	a.toasts = ts
	return a
}

// wrap is the row wrapper of the chat pane, memoized: a scroll frame reuses the rows of lines it
// showed before, instead of wrapping every line above the view again.
func (a App) wrap() wrapFunc {
	base := wrapLine
	if a.stamps {
		base = wrapStamped
	}
	if a.rows == nil {
		return base
	}
	return a.rows.memo(base, a.stamps)
}

// rowKey is what a wrapped row depends on.
type rowKey struct {
	at                                   int64
	nick, text, id, thread, reacts, seen string
	dim, stamped                         bool
	width                                int
}

// rowCache holds wrapped rows; it is shared by copies of App, so it lives behind a pointer.
type rowCache struct {
	mu sync.Mutex
	m  map[rowKey][]string
}

const rowCacheMax = 50000 // rows; the cache is cleared when it is full

func (c *rowCache) memo(base wrapFunc, stamped bool) wrapFunc {
	return func(l model.Line, width int) []string {
		k := rowKey{at: l.At.UnixNano(), nick: l.Nick, text: l.Text, id: l.ID, thread: l.Thread,
			dim: l.Dim, stamped: stamped, width: width, reacts: l.Reacts, seen: l.Seen}
		c.mu.Lock()
		rs, ok := c.m[k]
		c.mu.Unlock()
		if ok {
			return rs
		}
		rs = base(l, width)
		c.mu.Lock()
		if len(c.m) >= rowCacheMax {
			c.m = map[rowKey][]string{}
		}
		c.m[k] = rs
		c.mu.Unlock()
		return rs
	}
}

// paneAt is the side pane under column x in the wide layout, or focusChat.
func paneAt(x, w, h int) paneFocus {
	if !wide(w, h) {
		return focusChat
	}
	switch {
	case x < chanW+2:
		return focusChans
	case x >= w-nickW-2:
		return focusNicks
	}
	return focusChat
}

// paneWheel scrolls pane p's list by delta rows. The selection stays where it is.
func (a App) paneWheel(p paneFocus, delta int) App {
	_, names := a.paneItems(p)
	a.ptop[p] = max(min(a.ptop[p]+delta, len(names)-1), 0)
	return a
}

// wheel scrolls what is under the wheel by n notches: help box, side pane, or chat.
func (a App) wheel(m tea.MouseWheelMsg, n int) App {
	if a.pick { // a picker moves its selection, one row a notch; the theme picker previews as keys do
		ms := a.matches()
		a.psel = max(min(a.psel+n, len(ms)-1), 0)
		if a.pickWhat == pickThemes {
			return a.previewTheme()
		}
		return a
	}
	step := n * wheelRows
	if a.help {
		return a.helpScroll(step)
	}
	if p := paneAt(m.X, a.w, a.h); p != focusChat {
		return a.paneWheel(p, n)
	}
	if rows, _ := a.threadRows(); rows > 0 && m.Y >= 1 && m.Y-1 < rows {
		a.tscroll += step
		return a
	}
	return a.scroll(Action{Scroll, step, ""})
}

// linesCache keeps the shown lines of the current channel between frames. Scrolling and typing
// change neither the lines nor the filters, so they reuse it instead of copying and filtering the ring.
type linesCache struct {
	key   linesKey
	lines []model.Line
}

// linesKey is everything the shown lines depend on.
type linesKey struct {
	ch, grep, user, thread, speakers string
	kind                             model.ChanKind
	mentions                         bool
	ver                              uint64
	n                                int
}

// lines are the lines of c that the view shows (filters and thread applied), cached until they change.
func (a App) lines(c model.Channel) []model.Line {
	k := linesKey{ch: c.Name, grep: a.grep, user: a.user, thread: a.thread, speakers: strings.Join(a.speakers, "\x00"),
		kind: c.Kind, mentions: a.mentions, ver: c.Lines.Version(), n: c.Lines.Len()}
	if a.lc != nil && a.lc.key == k {
		return a.lc.lines
	}
	var out []model.Line
	for _, l := range c.Lines.All() {
		if a.shows(c.Kind, l) && (c.Name != a.m.Current().Name || a.inThread(l)) {
			out = append(out, l)
		}
	}
	if a.lc != nil {
		a.lc.key, a.lc.lines = k, out
	}
	return out
}

// sideCache keeps the side panes between frames: a frame that changed no channel, focus, filter or
// thread reuses them. Unread counts and the threads box position are cheap and always recomputed.
type sideCache struct {
	key sideKey
	val sidePanes
}

// sideKey is what the side panes depend on (session busy marks are not cached: see sides).
type sideKey struct {
	ver                                 uint64
	cur, fq, thread, user, grep, typing string
	pinsVer                             int
	focus                               paneFocus
	fsel, mainUnread                    int
	mentions                            bool
	tagsVer                             int
	presenceVer                         int
	heads                               []model.Thread
}

func (k sideKey) same(o sideKey) bool {
	return k.ver == o.ver && k.pinsVer == o.pinsVer && k.cur == o.cur && k.fq == o.fq && k.thread == o.thread && k.user == o.user &&
		k.grep == o.grep && k.typing == o.typing && k.focus == o.focus && k.fsel == o.fsel &&
		k.mainUnread == o.mainUnread && k.mentions == o.mentions && slices.Equal(k.heads, o.heads) &&
		k.tagsVer == o.tagsVer && k.presenceVer == o.presenceVer
}

// sides are the side panes of the current frame.
func (a App) sides() sidePanes {
	cur := a.m.Current().Name
	heads := a.threads[cur]
	k := sideKey{ver: a.m.Version(), pinsVer: a.pinsVer, cur: cur, fq: a.fq, thread: a.thread, user: a.user, grep: a.grep,
		typing: a.in.Value(), focus: a.focus, fsel: a.fsel, mainUnread: a.mainUnread[cur],
		mentions: a.mentions, heads: slices.Clone(heads), tagsVer: a.tagsVer, presenceVer: a.presenceVer}
	p, hit := sidePanes{}, false
	if a.sc != nil && len(a.info) == 0 && a.sc.key.same(k) {
		p, hit = a.sc.val, true
	}
	if !hit {
		chans, _ := a.paneItems(focusChans)
		nicks, _ := a.paneItems(focusNicks)
		_, rowOf := a.nickRows()
		nickSel := -1
		if a.fsel >= 0 && a.fsel < len(rowOf) {
			nickSel = rowOf[a.fsel]
		}
		p = sidePanes{chans: chans, nicks: nicks, focus: a.focus, sel: a.fsel, nickSel: nickSel, chanTop: a.ptop[focusChans], nickTop: a.ptop[focusNicks], q: a.fq, heads: heads, thread: a.thread, mainUnread: a.mainUnread[cur]}
		if a.sc != nil && len(a.info) == 0 {
			a.sc.key, a.sc.val = k, p
		}
	}
	p.mainLast, p.mainLastAt = a.mainLast()
	p.readAt, p.user = a.readAt[cur], a.user
	p.pinned = pinSet(a.pins[cur])
	p.jump = a.jump && !a.jumpChan
	p.chanJump = a.jumpChan
	p.unread = a.unreadAll()
	p.threadTop = threadTopOf(len(heads)+1, a.fsel, a.focus == focusThreads, a.tscroll)
	return p
}

// menuItem is one key of a leader menu: a label and what it does — another menu ("menu:<name>"),
// "react" (the + row), "thread" (the next thread), or a command (":word", or ":word " which opens the :
// prompt for its argument).
type menuItem struct{ key, label, do string }

// emojiMenu is the first nine emojis, each on its digit key: the emoji menu.
func emojiMenu() []menuItem {
	var out []menuItem
	for i, e := range emojis[:9] {
		out = append(out, menuItem{strconv.Itoa(i + 1), e.e + " " + e.name, "emoji:" + e.e})
	}
	return out
}

// menus are the leader menus: space opens "space"; c in it opens "channel". m opens "message".
var menus = map[string][]menuItem{
	"space": {{"space", "recent", "pick:recent"}, {"/", "search server", ":search "}, {":", "command", "key::"}, {"?", "all keys", "menu:normal"},
		{"m", "message…", "key:m"}, {"c", "channel…", "menu:channel"}, {"C", "room admin…", "menu:admin"}, {"p", "people…", "menu:people"},
		{"t", "threads…", "menu:threads"}, {"u", "unread…", "menu:unread"}, {"s", "search…", "menu:search"}, {"f", "files…", "menu:files"},
		{"i", "insert…", "menu:insert"}, {"v", "toggles…", "menu:toggles"}, {"I", "invites…", "menu:invites"}, {"a", "account…", "menu:account"},
		{"h", "help…", "menu:help"}, {"q", "quit…", "menu:quit"}},
	"channel": {{"c", "pick channel", "pick:chat"}, {"[", "prev channel", "key:h"}, {"]", "next channel", "key:l"}, {"w", "who (members)", ":names"},
		{"p", "pinned messages", "pick:pins"}, {"e", "explore public", ":explore"}, {"J", "join", ":join "}, {"N", "new channel", ":create "},
		{"I", "invite person", ":invite "}, {"M", "mute", ":mute"}, {"U", "unmute", ":unmute"}, {"F", "favourite", ":tag fav"},
		{"L", "low priority", ":tag low"}, {"T", "untag", ":tag none"}, {"P", "part (leave)", ":part"}, {"X", "close (leave + forget)", ":close"}},
	"admin": {{"t", "topic", ":topic "}, {"n", "name", ":name "}, {"a", "alias", ":alias "}, {"v", "avatar", ":roomavatar "},
		{"j", "join rule", ":joinrule "}, {"h", "history visibility", ":history "}, {"O", "op (power level)", ":op "}, {"K", "kick", ":kick "},
		{"B", "ban", ":ban "}, {"U", "unban", ":unban "}},
	"people": {{"p", "pick person: DM", "pick:user"}, {"w", "whois", ":whois "}, {"d", "DM by name", ":dm "}, {"m", "message person", ":msg "},
		{"I", "ignore", ":ignore "}, {"U", "unignore", ":unignore "}},
	"threads": {{"t", "toggle main / last", "thread"}, {"p", "pick thread", "pick:thread"}, {"[", "prev thread", "key:ctrl+p"},
		{"]", "next thread", "key:ctrl+n"}, {"n", "new thread", "key:T"}},
	"unread":  {{"u", "first unread", "jumpunread"}, {"o", "load older", ":older"}, {"R", "mark read", ":read"}, {"U", "mark unread", ":unread"}},
	"search":  {{"s", "server", ":search "}, {"/", "loaded lines", "key:/"}, {"g", "grep", "key:ctrl+g"}, {"w", "one speaker", "key:S"}, {"d", "date", ":goto "}},
	"files":   {{"f", "insert path", "key:ctrl+t"}, {"b", "insert file block", "key:ctrl+f"}, {"U", "upload", ":upload "}},
	"insert":  append(emojiMenu(), menuItem{"m", "emote (/me)", ":me "}, menuItem{"c", "shell capture", "key:!"}),
	"toggles": {{"v", "zen", "key:z"}, {"s", "stamps", "stamps"}, {"m", "mentions only", "key:M"}, {"t", "theme", ":theme"}},
	"invites": {{"I", "list", ":invites"}, {"A", "accept", ":accept"}, {"R", "reject", ":reject"}},
	"account": {{"a", "devices", ":devices"}, {"N", "nick", ":nick "}, {"A", "avatar", ":avatar "}, {"V", "verify device", ":verify"}, {"R", "recover keys", ":recover "}},
	"help":    {{"h", "help", ":help"}, {"k", "all keys", "menu:normal"}, {"d", "docs", ":docs"}, {"s", "sponsor", ":sponsor"}},
	"quit":    {{"q", "quit", ":quit"}},
	"message": {{"m", "reply", "key:a"}, {"a", "reply", "key:a"}, {"A", "reply quoted", "key:A"}, {"e", "edit", "key:e"}, {"t", "thread on it", "key:T"},
		{"v", "view/open", ":open"}, {"i", "image popup", ":view"}, {"p", "link preview", ":preview"}, {"y", "yank text", "key:y"}, {"Y", "yank link", "key:Y"},
		{"g", "go to replied", "key:gr"}, {"h", "edit history", "pick:edits"}, {"s", "seen by", "seenby"}, {"R", "react", "react"},
		{".", "repeat", "key:."}, {"P", "pin", ":pin"}, {"U", "unpin", ":unpin"}, {"F", "forward", "forward"}, {"D", "delete", ":redact"}},
	"move": {{"j", "down line", "key:j"}, {"k", "up line", "key:k"}, {"up", "up line", "key:up"}, {"down", "down line", "key:down"}, {"ctrl+d", "half page down", "key:ctrl+d"}, {"ctrl+u", "half page up", "key:ctrl+u"},
		{"G", "bottom", "key:G"}, {"h", "prev channel", "key:h"}, {"l", "next channel", "key:l"}, {"ctrl+p", "prev thread", "key:ctrl+p"},
		{"ctrl+n", "next thread", "key:ctrl+n"}, {"J", "next speaker", "key:J"}, {"K", "prev speaker", "key:K"}, {"u", "first unread", "key:u"},
		{"f", "find a line", "key:f"}, {"F", "find + open", "key:F"}, {"'", "recent", "key:'"}, {"tab", "next pane", "key:tab"}},
	"view": {{"i", "new message", "key:i"}, {"a", "reply", "key:a"}, {"A", "reply quoted", "key:A"}, {"o", "reply in thread", "key:o"},
		{"O", "thread reply quoted", "key:O"}, {"e", "edit mine", "key:e"}, {"C", "edit mine", "key:C"}, {"T", "start thread", "key:T"}, {"t", "toggle thread", "key:t"},
		{"D", "delete mine", "key:D"}, {"y", "yank text", "key:y"}, {"Y", "yank link", "key:Y"}, {"r", "react…", "menu:r"}, {".", "repeat", "key:."},
		{"v", "view/open", "key:v"}, {"m", "message menu", "key:m"}, {"#", "pick channel", "key:#"}, {"@", "pick person", "key:@"},
		{"%", "pick thread", "key:%"}, {"^", "pick pin", "key:^"}, {"z", "zen", "key:z"}, {"M", "mentions only", "key:M"}, {"S", "one speaker", "key:S"},
		{"I", "invite person", "key:I"}, {"ctrl+g", "grep", "key:ctrl+g"}, {"ctrl+o", "pick channel", "key:ctrl+o"}, {"ctrl+t", "file path", "key:ctrl+t"},
		{"ctrl+f", "file block", "key:ctrl+f"}, {"$", "shell", "key:$"}, {"!", "capture", "key:!"}, {"?", "help", "key:?"}},
	"g": {{"g", "top", "key:gg"}, {"e", "bottom", "key:ge"}, {"t", "next channel", "key:gt"}, {"T", "prev channel", "key:gT"}, {"x", "open", "key:gx"},
		{"f", "open", "key:gf"}, {"w", "find a line", "key:gw"}, {"i", "new message", "key:gi"}, {"r", "go to replied", "key:gr"},
		{"c", "jump to channel", "key:gc"}, {"d", "go to date", "key:gd"}},
	"]": {{"]", "next speaker", "key:]]"}, {"m", "my next msg", "key:]m"}, {"@", "next mention", "key:]@"}, {"l", "next link", "key:]l"},
		{"p", "next pin", "key:]p"}, {"t", "next thread", "key:]t"}, {"u", "next unread channel", "key:]u"}, {"c", "next channel", "key:]c"}},
	"[": {{"[", "prev speaker", "key:[["}, {"m", "my prev msg", "key:[m"}, {"@", "prev mention", "key:[@"}, {"l", "prev link", "key:[l"},
		{"p", "prev pin", "key:[p"}, {"t", "prev thread", "key:[t"}, {"u", "prev unread channel", "key:[u"}, {"c", "prev channel", "key:[c"}},
	"r":      reactMenu(),
	"Z":      {{"Z", "quit", "key:ZZ"}},
	"c":      {{"c", "edit mine", "key:cc"}},
	"ctrl+w": {{"w", "next pane", "key:tab"}},
}

// reactMenu is the r prefix: r1-9 quick reactions, rr the first.
func reactMenu() []menuItem {
	out := []menuItem{{"r", quickReacts[0], "key:rr"}}
	for i, e := range quickReacts {
		out = append(out, menuItem{strconv.Itoa(i + 1), e, "key:r" + strconv.Itoa(i+1)})
	}
	return out
}

// jumpKeys are the f labels, home row first; jumpTargets are the shown lines they name, top to bottom.
const jumpKeys = "asdfghjklqwertyuiopzxcvbnm"

// jumpTargets are the shown-line indexes of the lines on screen, top to bottom, one per label.
func (a App) jumpTargets() []int {
	_, of := tailRows(a.lines(a.m.Current()), a.page(), a.off(), textWidth(a.w, a.h), a.wrap())
	var out []int
	for i, o := range of {
		if (i == 0 || of[i-1] != o) && len(out) < len(jumpKeys) {
			out = append(out, o)
		}
	}
	return out
}

// WithThreadOpen sets the hook that loads a thread's history when the thread is opened (it must not block).
func (a App) WithThreadOpen(f func(ch, root string)) App {
	a.threadOpen = f
	return a
}

// permalink copies a matrix.to link to the selected line (else the newest line with an ID) and toasts it.
func (a App) permalink() (tea.Model, tea.Cmd) {
	l, ok := a.lastWithID(a.hit)
	c := a.m.Current()
	if !ok || c.ID == "" {
		return a.toast("permalink: no message with an ID here", true), nil
	}
	link := "https://matrix.to/#/" + url.PathEscape(c.ID) + "/" + url.PathEscape(l.ID)
	return a.toast("copied "+link, false), tea.SetClipboard(link)
}

// pinLabel is a pinned message's picker row: its "<nick> first line" if the line is loaded, else its ID.
func pinLabel(ls []model.Line, id string) string {
	for _, l := range ls {
		if l.ID == id {
			first, _, _ := strings.Cut(strings.TrimLeft(l.Text, "\r\n"), "\n")
			return "<" + l.Nick + "> " + strings.TrimRight(first, "\r")
		}
	}
	return "(not loaded) " + id
}

// jumpToPin selects pinned message number idx of the current channel in the chat.

// pinSet is the set of pinned message IDs.
func pinSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// WithThemeSave sets the hook that stores the chosen theme (must not block).
func (a App) WithThemeSave(f func(id string)) App {
	a.themeSave = f
	return a
}

// ImageMsg is a decoded image to show in a popup; esc closes it.
type ImageMsg struct{ Img image.Image }

// VerifyMsg is one step of a device verification (verify.go): Kind is request, ready, sas, done or cancel.
type VerifyMsg struct{ Kind, Txn, From, Emojis, Reason string }

// ThemeMsg is the saved theme to apply at start ("" is the terminal default).
type ThemeMsg string

// PresenceMsg is a member's presence: State is online, unavailable or offline.
type PresenceMsg struct{ Nick, State string }

// themeCmd runs :theme: with a scheme ID it applies and saves that theme; bare, it opens the theme picker.
func (a App) themeCmd(args []string) App {
	if len(args) > 0 {
		if _, ok := theme.ByID(args[0]); !ok {
			return a.toast("theme: no scheme "+args[0], true)
		}
		a = a.applyTheme(args[0])
		if a.themeSave != nil {
			a.themeSave(args[0])
		}
		return a.toast("theme: "+args[0], false)
	}
	a.themeIDs = []string{""}
	all, _ := theme.All()
	for _, s := range all {
		a.themeIDs = append(a.themeIDs, s.ID)
	}
	a.themeBefore = a.themeNow
	return a.openPick(pickThemes)
}

// themeLabel is a theme picker row: the scheme's name, system and variant.
func themeLabel(id string) string {
	if id == "" {
		return "terminal (default)"
	}
	s, _ := theme.ByID(id)
	return s.Name + "  (" + s.System + ", " + s.Variant + ")"
}

// themeKey handles a key in the theme picker: enter keeps the theme and saves it, esc restores the one
// active when the picker opened, and any other key previews the selected row's theme.
func (a App) themeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		a.pick = false
		if a.themeSave != nil {
			a.themeSave(a.themeNow)
		}
		return a.toast("theme: "+themeLabel(a.themeNow), false), nil
	case "esc":
		a.pick = false
		return a.applyTheme(a.themeBefore), nil
	}
	m, cmd := a.pickKey(msg)
	b := m.(App)
	if !b.pick {
		return b, cmd
	}
	return b.previewTheme(), cmd
}

// previewTheme applies the theme of the picker's selected row, the live preview of the theme picker.
func (a App) previewTheme() App {
	ms := a.matches()
	if a.psel < len(ms) {
		a = a.applyTheme(a.themeIDs[ms[a.psel].idx])
	}
	return a
}

// keySeq splits a "key:" menu item into the keys it replays: a named key ("up", "ctrl+d") is one key,
// anything else is one key per rune ("gg" is two).
func keySeq(s string) []string {
	switch s {
	case "up", "down", "enter", "tab", "space", "esc":
		return []string{s}
	}
	if strings.Contains(s, "+") && len(s) > 1 {
		return []string{s}
	}
	var out []string
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

// keyMsg is the key press that key name s stands for: a named key, ctrl+<letter>, or a single rune.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if letter, ok := strings.CutPrefix(s, "ctrl+"); ok && len(letter) == 1 {
		return tea.KeyPressMsg{Code: rune(letter[0]), Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(s)
	return tea.KeyPressMsg{Code: r, Text: s}
}

// menuPrefixes are the prefix keys whose continuations the which-key popup lists while they wait.
var menuPrefixes = []string{"g", "[", "]", "r"}

// reactKeyAt is the reaction key under column col of a row of line l: on the summary's first row, or on
// a row that continues the wrapped summary below it.
func (a App) reactKeyAt(l model.Line, row string, col int) string {
	if key := reactAt(row, col); key != "" || l.Reacts == "" {
		return key
	}
	k, s := -1, -1
	for i, r := range a.wrap()(l, textWidth(a.w, a.h)) {
		if k < 0 && r == row {
			k = i
		}
		if s < 0 && strings.Contains(r, "  [") {
			s = i
		}
	}
	if s < 0 || k <= s {
		return ""
	}
	return reactAtCont(row, col)
}

// menuParent is the menu that opens the menu name: the one with a "menu:<name>" item, else space. The
// top menus have none ("space" and "normal").
func menuParent(name string) string {
	if name == "space" || name == "normal" || name == "" {
		return ""
	}
	for parent, items := range menus {
		for _, it := range items {
			if it.do == "menu:"+name {
				return parent
			}
		}
	}
	return "space"
}

// normalMenu lists every NORMAL key in one menu: the motions, the views and the prefix keys.
func normalMenu() []menuItem {
	out := append(slices.Clone(menus["move"]), menus["view"]...)
	return append(out,
		menuItem{"g", "g…", "menu:g"}, menuItem{"]", "next…", "menu:]"}, menuItem{"[", "prev…", "menu:["},
		menuItem{":", "command", "key::"}, menuItem{"space", "menus", "menu:space"})
}

// Command names, from the model's command names the dispatcher compares and runs. normalCmdNames maps the
// NORMAL actions to them for the hints.
const (
	cmdRedact = ":" + string(model.CmdRedact)
	cmdOpen   = ":" + string(model.CmdOpen)
	cmdGoto   = string(model.CmdGoto) // the prompt text is cmdGoto and a space
	cmdQuit   = ":" + string(model.CmdQuit)
	cmdHelp   = ":" + string(model.CmdHelp)
)

// namedActions are the which-key items that name a NORMAL action by its Kind.
var namedActions = map[string]ActKind{"pick:chat": PickChan, "pick:thread": PickThread, "pick:recent": PickRecent,
	"pick:user": PickUser, "pick:pins": PickPins, "thread": ThreadNext, "jumpunread": JumpUnread, "permalink": Permalink}

// noNormalKey are the which-key items with no NORMAL key, by the start of their do string, so no hint.
// "emoji:" items insert text.
var noNormalKey = []string{"pick:edits", "stamps", "seenby", "forward", "emoji:"}

// noNormal is whether the which-key item with do string do has no NORMAL key.
func noNormal(do string) bool {
	return slices.ContainsFunc(noNormalKey, func(p string) bool { return strings.HasPrefix(do, p) })
}

// normalCmdNames is the ":command" each NORMAL action runs, by the constant the dispatcher compares or runs it with.
var normalCmdNames = map[ActKind]string{Delete: cmdRedact, OpenLine: cmdOpen, Quit: cmdQuit, Help: cmdHelp, GotoDate: ":" + cmdGoto}

// normalCmd is the ":command" a NORMAL action runs, by the command's name; "" for the rest.
func normalCmd(a Action) string {
	return normalCmdNames[a.Kind]
}

// menuMatch accepts the NORMAL bindings that run the command of a which-key item: the action of its key for a
// "key:" item, the same Kind for a named item, and the same ":command" for a ":" item.
// ok is false for the items with no NORMAL key.
func menuMatch(it menuItem) (match func(binding) bool, ok bool) {
	if noNormal(it.do) {
		return nil, false
	}
	if it.do == "react" {
		return func(b binding) bool { return b.act.Kind == React }, true
	}
	if seq, isKey := strings.CutPrefix(it.do, "key:"); isKey {
		want := replayKeys(keySeq(seq))
		return func(b binding) bool { return b.act == want }, want != Action{}
	}
	if cmd, isCmd := strings.CutPrefix(it.do, ":"); isCmd {
		want := ":" + strings.TrimSpace(cmd)
		return func(b binding) bool { return normalCmd(b.act) == want }, true
	}
	kind, named := namedActions[it.do]
	return func(b binding) bool { return b.act.Kind == kind }, named
}

func init() { menus["normal"] = normalMenu() }

// keepSelVisible scrolls the focused side pane so its selection is on screen.
func (a App) keepSelVisible() App {
	f := a.focus
	if f != focusChans && f != focusNicks {
		return a
	}
	rows := paneRows(a.w, a.h, 1)
	pos := a.fsel
	if f == focusNicks {
		if _, rowOf := a.nickRows(); a.fsel >= 0 && a.fsel < len(rowOf) {
			pos = rowOf[a.fsel]
		}
	}
	if pos < a.ptop[f] {
		a.ptop[f] = pos
	}
	if pos >= a.ptop[f]+rows {
		a.ptop[f] = pos - rows + 1
	}
	return a
}

// WithContextLoad sets the hook that fetches a message and its neighbours (must not block).
func (a App) WithContextLoad(f func(ch, id string)) App {
	a.loadContext = f
	return a
}

// loadPin fetches the pinned message id with its neighbours, then selects it when it arrives.
func (a App) loadPin(id string) App {
	if a.loadContext == nil {
		return a.toast("pinned message not loaded", true)
	}
	a.wantHit = id
	a.loadContext(a.m.Current().Name, id)
	return a.toast("loading pinned message…", false)
}

// unloadedPin is the first (fwd) or last pin of the channel that is not loaded yet, "" if none.
func (a App) unloadedPin(fwd bool) string {
	cur := a.m.Current().Name
	ids := a.pins[cur]
	for k := range ids {
		id := ids[k]
		if !fwd {
			id = ids[len(ids)-1-k]
		}
		if !a.m.HasLine(cur, id) {
			return id
		}
	}
	return ""
}

// selectWanted selects the line a loadPin asked for, once it is loaded.
func (a App) selectWanted() App {
	if a.wantHit == "" {
		return a
	}
	for i, l := range a.lines(a.m.Current()) {
		if l.ID == a.wantHit {
			a.hit, a.wantHit = i, ""
			return a.showHit()
		}
	}
	return a
}

// jumpToPin selects pinned message idx of the channel; one that is not loaded is fetched with its neighbours.
func (a App) jumpToPin(idx int) App {
	cur := a.m.Current().Name
	if idx < 0 || idx >= len(a.pins[cur]) {
		return a
	}
	id := a.pins[cur][idx]
	for i, l := range a.lines(a.m.Current()) {
		if l.ID == id {
			a.hit = i
			return a.showHit()
		}
	}
	return a.loadPin(id)
}

// WithOlderLoad sets the hook that fetches one older page of history (must not block).
func (a App) WithOlderLoad(f func(ch string)) App {
	a.loadOlder = f
	return a
}

// verifyStep applies one device verification step: the request, ready, SAS (the box to compare emojis
// in), done, or cancel.
func (a App) verifyStep(msg VerifyMsg) App {
	switch msg.Kind {
	case "request":
		a.verifyTxn = msg.Txn
		return a.toast("verification request from "+msg.From+": :verify accept", false)
	case "ready":
		return a.toast("verification ready with "+msg.From, false)
	case "sas":
		a.verifyTxn, a.verifySAS = msg.Txn, msg.Emojis
	case "done":
		a.verifyTxn, a.verifySAS = "", ""
		return a.toast("device verified ✓", false)
	case "cancel":
		a.verifyTxn, a.verifySAS = "", ""
		return a.toast("verification cancelled: "+msg.Reason, true)
	}
	return a
}

// older asks for one older page and remembers the motion to retry when it arrives.
func (a App) older(key string) App {
	if a.loadOlder == nil {
		return a.toast("no more", false)
	}
	if a.retryMotion == "" { // one page at a time
		a.loadOlder(a.m.Current().Name)
	}
	a.retryMotion = key
	return a.toast("loading older messages…", false)
}

// matches are the picker's rows for its query: help matches exactly, everything else fuzzily.
func (a App) matches() []pickMatch {
	if a.pickWhat == pickHelp {
		return exactList(a.pickSource(), a.pq.Value())
	}
	return pickList(a.pickSource(), a.pq.Value())
}

// lastPinText is the first line of the newest pinned message of the current channel, "" when it is not loaded.
func (a App) lastPinText() string {
	cur := a.m.Current()
	ids := a.pins[cur.Name]
	if len(ids) == 0 {
		return ""
	}
	for _, l := range a.lines(cur) {
		if l.ID == ids[len(ids)-1] {
			first, _, _ := strings.Cut(strings.TrimLeft(l.Text, "\r\n"), "\n")
			return strings.TrimRight(first, "\r")
		}
	}
	return ""
}
