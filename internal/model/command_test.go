package model

import (
	"slices"
	"strings"
	"testing"
)

func TestParseCmd(t *testing.T) {
	tests := []struct {
		in      string
		name    CmdName
		args    []string
		wantErr bool
	}{
		{"/join &x", CmdJoin, []string{"&x"}, false},
		{":join &x", CmdJoin, []string{"&x"}, false},
		{"/kick bob", CmdKick, []string{"bob"}, false},
		{"/invite bob &x", CmdInvite, []string{"bob", "&x"}, false},
		{"/msg bob hi there", CmdMsg, []string{"bob", "hi there"}, false},
		{"/msg #a  two  spaces", CmdMsg, []string{"#a", "two  spaces"}, false},
		{"/msg\tbob hi", CmdMsg, []string{"bob", "hi"}, false},
		{"/topic a  b", CmdTopic, []string{"a  b"}, false},
		{"/topic", "", nil, true},
		{"/msg bob\thi", CmdMsg, []string{"bob", "hi"}, false},
		{"/whois bob", CmdWhois, []string{"bob"}, false},
		{"/quit", CmdQuit, nil, false},
		{"/read", CmdRead, nil, false},
		{":unread &x", CmdUnread, []string{"&x"}, false},
		{"/forward #b", CmdForward, []string{"#b"}, false},
		{"/ban @bob:mock.test spam here", CmdBan, []string{"@bob:mock.test", "spam", "here"}, false},
		{"/unban @bob:mock.test", CmdUnban, []string{"@bob:mock.test"}, false},
		{"/op @bob:mock.test 50", CmdOp, []string{"@bob:mock.test", "50"}, false},
		{"/op @bob:mock.test", "", nil, true},
		{"/forward", "", nil, true},
		{":q", CmdQuit, nil, false},
		{"/rm &x", CmdClose, []string{"&x"}, false},
		{"/foo", "", nil, true},
		{"/mode x", "", nil, true}, // /mode dropped (vic 2026-10-22)
		{"/mode", "", nil, true},
		{"/thinking on", "", nil, true}, // dropped (vic 2026-10-22)
		{"/join", "", nil, true},
		{"/join &a &b", "", nil, true},
		{"/kick", "", nil, true},
		{"/kick a b", "", nil, true},
		{"/invite bob", "", nil, true},
		{"/invite a b c", "", nil, true},
		{"/msg bob", "", nil, true},
		{"/msg bob   ", "", nil, true},
		{"/", "", nil, true},
		{"", "", nil, true},
		{"hello", "", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseCmd(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got.Name() != tc.name || !slices.Equal(got.Args(), tc.args) {
				t.Fatalf("got %q %q want %q %q", got.Name(), got.Args(), tc.name, tc.args)
			}
		})
	}
}

func TestCmdWordsAllParse(t *testing.T) {
	for _, w := range CmdWords {
		if _, err := ParseCmd("/" + w); err != nil && strings.Contains(err.Error(), "unknown command") {
			t.Errorf("CmdWords has %q, ParseCmd does not know it", w)
		}
	}
	if _, err := ParseCmd("/nosuch"); err == nil || !strings.Contains(err.Error(), "unknown command") { // control
		t.Fatalf("control: %v", err)
	}
}

func TestUsageOnBadArgs(t *testing.T) {
	for line, want := range map[string]string{"/join": "usage: :join &room", "/invite dave": "usage: :invite <nick> &room", "/msg": "usage: :msg <nick> <text>"} {
		if _, err := ParseCmd(line); err == nil || err.Error() != want {
			t.Errorf("%s: %v", line, err)
		}
	}
	for _, w := range CmdWords { // every command word has a usage
		c, _ := ParseCmd("/" + w)
		if n := CmdName(w); usage[n] == "" && usage[c.Name()] == "" {
			t.Errorf("no usage for %s", w)
		}
	}
}

// TestParseReact: /react takes an emoji and an event ID; the UI adds the ID of the cursor line.
func TestParseReact(t *testing.T) {
	c, err := ParseCmd("/react 👍 $ev")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 2 || got[0] != "👍" || got[1] != "$ev" {
		t.Fatalf("args %q", got)
	}
	if _, err := ParseCmd("/react 👍"); err == nil {
		t.Fatal("one arg must be an error")
	}
}

// TestParseRedact: /redact takes an event ID and a free-form reason; the reason is optional.
func TestParseRedact(t *testing.T) {
	c, err := ParseCmd("/redact $ev spam here")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 3 || got[0] != "$ev" || got[1] != "spam" || got[2] != "here" {
		t.Fatalf("args %q", got)
	}
	c, err = ParseCmd("/redact $ev")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "$ev" {
		t.Fatalf("args %q", got)
	}
}

// TestParseNameAndTag: /name takes raw text; /tag takes one word, and needs it.
func TestParseNameAndTag(t *testing.T) {
	c, err := ParseCmd("/name My  Room")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "My  Room" {
		t.Fatalf("name args %q", got)
	}
	c, err = ParseCmd("/tag fav")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "fav" {
		t.Fatalf("tag args %q", got)
	}
	if _, err := ParseCmd("/tag"); err == nil {
		t.Fatal("/tag without a group must be an error")
	}
}

// TestParseNickIgnore: /nick takes raw text; /ignore and /unignore take one user.
func TestParseNickIgnore(t *testing.T) {
	c, err := ParseCmd("/nick Vic B")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "Vic B" {
		t.Fatalf("nick args %q", got)
	}
	for _, line := range []string{"/ignore bob", "/unignore bob"} {
		c, err := ParseCmd(line)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Args(); len(got) != 1 || got[0] != "bob" {
			t.Fatalf("%s args %q", line, got)
		}
	}
}

// TestParseMe: /me takes the action text as is.
func TestParseMe(t *testing.T) {
	c, err := ParseCmd("/me waves  hello")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "waves  hello" {
		t.Fatalf("args %q", got)
	}
}

// TestParseOpen: /open takes the event ID the UI adds.
func TestParseOpen(t *testing.T) {
	c, err := ParseCmd("/open $ev")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "$ev" {
		t.Fatalf("args %q", got)
	}
}

// TestParseUpload: /upload takes the rest of the line as the path, spaces included.
func TestParseUpload(t *testing.T) {
	c, err := ParseCmd("/upload ~/a b.png")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "~/a b.png" {
		t.Fatalf("args %q", got)
	}
}

// TestParseInvites: /accept and /reject take an optional room; /invites takes none.
func TestParseInvites(t *testing.T) {
	c, err := ParseCmd("/accept #a:s")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != CmdAccept || len(c.Args()) != 1 || c.Args()[0] != "#a:s" {
		t.Fatalf("accept %v %q", c.Name(), c.Args())
	}
	for _, line := range []string{"/reject", "/invites"} {
		if _, err := ParseCmd(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
}

// TestParseRecover: /recover takes the whole recovery key as one argument.
func TestParseRecover(t *testing.T) {
	c, err := ParseCmd("/recover EsT1 abcd efgh")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "EsT1 abcd efgh" {
		t.Fatalf("args %q", got)
	}
}

// TestParseAvatar: /avatar takes the image path as one raw argument.
func TestParseAvatar(t *testing.T) {
	c, err := ParseCmd("/avatar ~/my pic.png")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Args(); len(got) != 1 || got[0] != "~/my pic.png" {
		t.Fatalf("args %q", got)
	}
}

// TestUsage: a command word's usage line; an alias gives its command's; an unknown word gives "".
func TestUsage(t *testing.T) {
	if !strings.HasPrefix(Usage("join"), ":join") {
		t.Fatalf("join: %q", Usage("join"))
	}
	if Usage("rm") != Usage("close") {
		t.Fatalf("rm %q close %q", Usage("rm"), Usage("close"))
	}
	if Usage("nope") != "" {
		t.Fatalf("nope: %q", Usage("nope"))
	}
}

// TestParseMute: /mute and /unmute take an optional room.
func TestParseMute(t *testing.T) {
	for _, line := range []string{"/mute", "/mute &x", "/unmute", "/unmute &x"} {
		if _, err := ParseCmd(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
	c, err := ParseCmd("/mute &x")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != CmdMute || len(c.Args()) != 1 || c.Args()[0] != "&x" {
		t.Fatalf("mute %v %q", c.Name(), c.Args())
	}
}

// TestParseSponsorDocs: /sponsor and /docs take no arguments.
func TestParseSponsorDocs(t *testing.T) {
	for _, line := range []string{"/sponsor", "/docs"} {
		if _, err := ParseCmd(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
	c, err := ParseCmd("/docs")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != CmdDocs {
		t.Fatalf("docs %v", c.Name())
	}
}

// TestParsePin: /pin and /unpin take one event ID, which the UI adds from the selected line.
func TestParsePin(t *testing.T) {
	c, err := ParseCmd("/pin $e")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != CmdPin || len(c.Args()) != 1 || c.Args()[0] != "$e" {
		t.Fatalf("pin %v %q", c.Name(), c.Args())
	}
	if _, err := ParseCmd("/unpin $e"); err != nil {
		t.Fatal(err)
	}
}

func TestRoomSettingsParse(t *testing.T) {
	for _, tc := range []struct {
		line string
		name CmdName
		args []string
	}{
		{"/joinrule invite", CmdJoinRule, []string{"invite"}},
		{"/history world", CmdHistory, []string{"world"}},
		{"/alias #den:mock.test", CmdAlias, []string{"#den:mock.test"}},
		{"/roomavatar ~/pic one.png", CmdRoomAvatar, []string{"~/pic one.png"}},
	} {
		c, err := ParseCmd(tc.line)
		if err != nil || c.Name() != tc.name || !slices.Equal(c.Args(), tc.args) {
			t.Errorf("%s: got %q %q %v", tc.line, c.Name(), c.Args(), err)
		}
	}
	if _, err := ParseCmd("/alias"); err == nil {
		t.Error("/alias without an alias parsed")
	}
}

func TestCreateExploreParse(t *testing.T) {
	for _, tc := range []struct {
		line string
		name CmdName
		args []string
	}{
		{"/create &den public", CmdCreate, []string{"&den", "public"}},
		{"/create &den", CmdCreate, []string{"&den"}},
		{"/explore", CmdExplore, nil},
		{"/explore cats", CmdExplore, []string{"cats"}},
	} {
		c, err := ParseCmd(tc.line)
		if err != nil || c.Name() != tc.name || !slices.Equal(c.Args(), tc.args) {
			t.Errorf("%s: got %q %q %v", tc.line, c.Name(), c.Args(), err)
		}
	}
}

func TestSearchParse(t *testing.T) {
	c, err := ParseCmd("/search hello big world")
	if err != nil || c.Name() != CmdSearch || !slices.Equal(c.Args(), []string{"hello big world"}) {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
}

func TestDevicesParse(t *testing.T) {
	c, err := ParseCmd("/devices")
	if err != nil || c.Name() != CmdDevices || len(c.Args()) != 0 {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
}

func TestGotoParse(t *testing.T) {
	c, err := ParseCmd("/goto 2026-10-21 09:00")
	if err != nil || c.Name() != CmdGoto || !slices.Equal(c.Args(), []string{"2026-10-21 09:00"}) {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
}

func TestThemeParse(t *testing.T) {
	c, err := ParseCmd("/theme base16/gruvbox-dark-hard")
	if err != nil || c.Name() != CmdTheme || !slices.Equal(c.Args(), []string{"base16/gruvbox-dark-hard"}) {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
	if c, err := ParseCmd("/theme"); err != nil || c.Name() != CmdTheme || len(c.Args()) != 0 {
		t.Fatalf("bare: %q %q %v", c.Name(), c.Args(), err)
	}
}

func TestPreviewParse(t *testing.T) {
	c, err := ParseCmd("/preview https://x.org/a")
	if err != nil || c.Name() != CmdPreview || !slices.Equal(c.Args(), []string{"https://x.org/a"}) {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
}

func TestDMParse(t *testing.T) {
	c, err := ParseCmd("/dm @bob:mock.test")
	if err != nil || c.Name() != CmdDM || !slices.Equal(c.Args(), []string{"@bob:mock.test"}) {
		t.Fatalf("got %q %q %v", c.Name(), c.Args(), err)
	}
}
