//go:build seed

package main

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// scripted is one line of the demo talk. in replies in a thread (by name); root starts a thread (by name).
type scripted struct {
	who, text string
	in, root  string
	react     string
	pin       bool
}

// conversation is the #mtx talk: people chat on the main line, with reactions and several pins.
var conversation = []scripted{
	{who: "alice", text: "morning all. is the release still on for today?", pin: true},
	{who: "dan", text: "morning! yes, if the docs land first. i am on them now", react: "👍"},
	{who: "bob", text: "docs are the long pole. the gifs take forever to render"},
	{who: "carol", text: "i can take the screenshots for the roadmap page if that helps"},
	{who: "dan", text: "that would be great, thanks. drop them in docs/public/demo"},
	{who: "alice", text: "is anyone else seeing a stuck cursor on the verify box?"},
	{who: "bob", text: "not me. i was in zen mode most of the morning though"},
	{who: "carol", text: "my terminal is 80 columns. does the ? help still fit?"},
	{who: "dan", text: "it wraps at 80, the key column is too wide. i will fix it today", pin: true},
	{who: "alice", text: "thanks. also the tour gif shows the wrong room at the start, did you see?"},
	{who: "dan", text: "yes, it opens the lounge before sync is done. fixed in the tape now"},
	{who: "bob", text: "nice. the picker is fast now, % opens threads in a blink"},
	{who: "carol", text: "what does ctrl+g do exactly? i keep typing it by accident"},
	{who: "alice", text: "it greps the lines you see. esc clears it, same as the search"},
	{who: "bob", text: "i still reach for / first, habit from vim"},
	{who: "dan", text: "/ is the search that highlights hits. ctrl+g hides everything that does not match", react: "🎉", pin: true},
	{who: "carol", text: "ok that makes sense. i will use it on the release log"},
	{who: "alice", text: "has anyone tried verifying a new device with the emojis? mine said mismatch first time"},
	{who: "bob", text: "retry it. the first try sometimes gets the wrong device name"},
	{who: "dan", text: "if it still fails, :verify cancel and start again from the account menu"},
	{who: "alice", text: "worked on the second try. thanks"},
	{who: "carol", text: "tea or coffee at the 3pm check-in? i can bring the biscuits"},
	{who: "bob", text: "coffee. the biscuits are a must"},
	{who: "dan", text: "tea for me please"},
	{who: "alice", text: "i am on the release notes now, will send the list by lunch"},
	{who: "dan", text: "the image of my cat is for the readme, it is in the pinned thread"},
	{who: "carol", text: "lol the cat is fine, but the readme needs the tour gif first"},
	{who: "bob", text: "how do people feel about the roadmap page? too long?"},
	{who: "dan", text: "it is one screen. the planned list is short, that is on purpose", pin: true},
	{who: "alice", text: "agreed. short is better for a first read"},
	{who: "carol", text: "i found a typo in the menus, 'invites' says 'invite'. who owns that"},
	{who: "dan", text: "mine. fixing it now, thanks for catching it"},
	{who: "bob", text: "pushed the theme picker change, live preview works on dracula now"},
	{who: "alice", text: "nice, that one was annoying"},
	{who: "dan", text: "the nix build failed on the goolm tag for me last night. did it work for anyone else?"},
	{who: "bob", text: "works here. are you on the flake lock from main?"},
	{who: "carol", text: "same here, fine on linux"},
	{who: "dan", text: "ok, it was my old lock file. all good now"},
	{who: "alice", text: "how many members does the lounge have? i keep seeing the count change"},
	{who: "bob", text: "it only counts the people who joined since the last sync"},
	{who: "carol", text: "i think the count is right, two of us joined late"},
	{who: "dan", text: "yes, the seed invites are still pending for some of you"},
	{who: "alice", text: "pending invites show in the invites menu, space i l if you want to check"},
	{who: "bob", text: "the image popup is great. any key closes it, no fuss"},
	{who: "carol", text: "i keep pressing y on it by habit, lol"},
	{who: "dan", text: "y and n are only for the verify box. the image box does not care"},
	{who: "alice", text: "random question: does mtx keep my drafts if i quit mid line?"},
	{who: "dan", text: "not yet. it is on the list, roadmap has the rest"},
	{who: "bob", text: "polls are on the list too. parity with the other clients"},
	{who: "carol", text: "i would love polls for the standup. no more chat poll bots"},
	{who: "alice", text: "+1 for polls. and a config file so i stop typing the homeserver"},
	{who: "dan", text: "both are planned. the config file is the one i want first"},
	{who: "bob", text: "release notes drafted. link is in the release thread"},
	{who: "carol", text: "reading them now"},
	{who: "alice", text: "the zen mode line in the list is missing the z key, can someone check"},
	{who: "dan", text: "checked, z is there. it was the wrap, the line got cut"},
	{who: "bob", text: "ok good. i am going for lunch, back in an hour"},
	{who: "carol", text: "bring back something for me too"},
	{who: "alice", text: "what is the plan for the old ircc code? still deleting it?"},
	{who: "dan", text: "yes, it is the top priority of the planned list. after the release", pin: true},
	{who: "bob", text: "back. the gifs are rendering in the background now, check the folder in a bit"},
	{who: "carol", text: "the seen by list is neat. it shows who read up to a line"},
	{who: "alice", text: "does it update live when someone reads? i want to test that"},
	{who: "dan", text: "it reads the receipts from the server. live enough"},
	{who: "bob", text: "found a bug: the search hit count is off by one after a grep. looking now"},
	{who: "carol", text: "let me know, i use grep a lot on long rooms"},
	{who: "dan", text: "fixed in the next commit. thanks bob"},
	{who: "alice", text: "release is on. tag goes out after the docs are live", pin: true},
	{who: "bob", text: "great. good work everyone"},
	{who: "carol", text: "cheers. see you tomorrow"},
	{who: "dan", text: "thanks all. i will close the docs ticket tonight"},
	{who: "alice", text: "the lounge is open if anyone wants to chat after"},
	{who: "bob", text: "will do, in a bit"},
	{who: "carol", text: "me too, i have the biscuits"},
}

// threads are the side talks: each root starts a thread and the replies go in it.
var threads = []scripted{
	{who: "dan", text: "release notes: who wants to write the changelog? i can do the first draft", root: "release"},
	{who: "bob", text: "i will do the fixes part", in: "release"},
	{who: "carol", text: "i can proofread after you", in: "release"},
	{who: "alice", text: "send me the commit list and i will turn it into bullets", in: "release"},
	{who: "dan", text: "commits are in the next message, i will paste them now", in: "release"},
	{who: "bob", text: "thanks. the search fix goes at the top", in: "release"},
	{who: "carol", text: "posted. please check the wording on the search line", in: "release", pin: true},

	{who: "carol", text: "does #lounge need a topic? it is empty right now", root: "topic"},
	{who: "alice", text: "set one. something short", in: "topic"},
	{who: "dan", text: ":topic casual talk, no meetings in here", in: "topic"},
	{who: "carol", text: "ha, that is perfect", in: "topic"},

	{who: "bob", text: "anyone seen the emoji verify on a second device hang?", root: "verify"},
	{who: "dan", text: "once, it hung on the sas step. cancel and retry worked", in: "verify"},
	{who: "alice", text: "mine just timed out, no error", in: "verify"},
	{who: "bob", text: "ok so it is not only me", in: "verify"},
	{who: "dan", text: "i will add a timeout message to the box", in: "verify", pin: true},

	{who: "alice", text: "which theme is everyone on? i am on dracula", root: "theme"},
	{who: "bob", text: "catppuccin mocha, the default one", in: "theme"},
	{who: "carol", text: "nord for me, it is calm at night", in: "theme"},
	{who: "dan", text: "base16 default dark for me. space T lets you try them all", in: "theme"},
	{who: "alice", text: "i forgot space T exists. it is a good preview", in: "theme"},
	{who: "carol", text: "the preview is live, so you can flick through without committing", in: "theme"},

	{who: "dan", text: "nix build fails on aarch64 with the goolm tag, anyone?", root: "nix"},
	{who: "bob", text: "not on x86. try the lock file from main", in: "nix"},
	{who: "dan", text: "that fixed it, my lock was stale", in: "nix"},
	{who: "carol", text: "good. i will update my flake input too", in: "nix"},

	{who: "carol", text: "threads in the picker are fast. % opens them in a blink", root: "picker"},
	{who: "alice", text: "and ]t jumps to the next thread on the main line", in: "picker"},
	{who: "bob", text: "i like that the thread view keeps the main line visible", in: "picker"},

	{who: "bob", text: "is :search server side only? it missed a message from yesterday", root: "search"},
	{who: "dan", text: "yes, it is server side. it only finds what the server has indexed", in: "search"},
	{who: "alice", text: "so use ctrl+g for what is loaded and :search for the rest", in: "search"},
	{who: "dan", text: "exactly. :search jumps you to the message if it is in the buffer", in: "search"},
	{who: "carol", text: "noted. i will keep both in my head", in: "search"},

	{who: "alice", text: "cat.png is from my cat, she sleeps on the keyboard", root: "cat"},
	{who: "carol", text: "she looks very warm", in: "cat"},
	{who: "bob", text: "can we get a second photo for the roadmap page?", in: "cat"},
	{who: "alice", text: "maybe. she only sleeps in one place", in: "cat"},

	{who: "carol", text: "lunch? the soup place by the station is open again", root: "lunch"},
	{who: "dan", text: "yes please, 12:30", in: "lunch"},
	{who: "bob", text: "i can come after the release call", in: "lunch"},
	{who: "alice", text: "count me in, i will bring the list", in: "lunch"},

	{who: "bob", text: "encrypted room key backup: has anyone run :recover?", root: "recover"},
	{who: "carol", text: "i did, it asked for the key from my notes", in: "recover"},
	{who: "dan", text: "keep that key somewhere safe, it is the only way back in", in: "recover", pin: true},
	{who: "alice", text: "i store mine in the password manager now", in: "recover"},
	{who: "bob", text: "good idea. will do the same", in: "recover"},

	{who: "alice", text: "mouse: a click on a line selects it, did i get that right?", root: "mouse"},
	{who: "dan", text: "yes. the second click replies, that is the one people miss", in: "mouse"},
	{who: "carol", text: "wheel scrolls the chat, the nicks pane is clickable too", in: "mouse"},

	{who: "dan", text: "pinned the release line so it stays on top for the team", root: "pin"},
	{who: "alice", text: "pins show in the picker with ^", in: "pin"},
	{who: "bob", text: "and enter jumps to the pinned line", in: "pin"},
	{who: "carol", text: "nice, that is what i wanted", in: "pin"},

	{who: "bob", text: "what should the default channel be on first run? #mtx or the lounge?", root: "default"},
	{who: "dan", text: "mtx, it is where the work is", in: "default"},
	{who: "carol", text: "lounge, so new people get a friendly room", in: "default"},
	{who: "alice", text: "mtx for now, lounge for the chat. both work", in: "default"},

	{who: "carol", text: "does the yank key copy the whole message or just the line?", root: "yank"},
	{who: "bob", text: "the cursor line. y again copies the next one", in: "yank"},
	{who: "alice", text: "and Y copies a matrix.to link to the message", in: "yank"},
	{who: "dan", text: "right, Y is the link one. handy to paste into the team channel", in: "yank"},

	{who: "alice", text: "who is still on the old ircc code? it is planned for removal", root: "ircc"},
	{who: "bob", text: "not me, i never touched it", in: "ircc"},
	{who: "dan", text: "i will remove it after the release. it is the top bead", in: "ircc", pin: true},
	{who: "carol", text: "let me know which files, i can help", in: "ircc"},

	{who: "dan", text: "polls: who would use them most? the standup needs one", root: "polls"},
	{who: "carol", text: "me. every day, no more emoji votes in chat", in: "polls"},
	{who: "alice", text: "the team lunch vote would be perfect", in: "polls"},
	{who: "bob", text: "i vote for a poll on the next theme, lol", in: "polls"},

	{who: "bob", text: "the status bar shows the keys for each mode, nice touch", root: "statusbar"},
	{who: "alice", text: "it changes when you press space, the menu is right above it", in: "statusbar"},
	{who: "carol", text: "i read it every time now, it is easy to forget keys otherwise", in: "statusbar"},

	{who: "alice", text: "file finder: ctrl+t inserts a path, ctrl+f a fenced block. which one do you use?", root: "files"},
	{who: "dan", text: "ctrl+f when i paste code, ctrl+t for a path in a message", in: "files"},
	{who: "bob", text: "same. ctrl+f saves me a copy and paste", in: "files"},

	{who: "carol", text: "the release call moved to 14:00, same room", root: "call"},
	{who: "dan", text: "ok, i will move the reminder", in: "call"},
	{who: "alice", text: "thanks for the heads up", in: "call"},
}

// dmPair is one direct message: a opens it and invites b, and talk runs between them.
type dmPair struct {
	a, b string
	talk []scripted
}

// dms are the direct messages of the demo: many pairs, each with its own short talk.
var dms = []dmPair{
	{a: "dan", b: "alice", talk: []scripted{
		{who: "dan", text: "dinner friday?"},
		{who: "alice", text: "friday works. the place with the noodles?"},
		{who: "dan", text: "that one. 7:30?"},
		{who: "alice", text: "7:30 is good. i will book if you cannot"},
		{who: "dan", text: "i will book it, you bring the wine"},
		{who: "alice", text: "deal. how is the release going?"},
		{who: "dan", text: "almost there. docs are the last thing, the gifs are rendering"},
		{who: "alice", text: "i saw the tour gif. the room opens properly now"},
		{who: "dan", text: "yes, it waits for sync first. it was a race before"},
		{who: "alice", text: "nice. and the cat photo stays in the readme?"},
		{who: "dan", text: "your cat is the readme star, sorry"},
		{who: "alice", text: "she will be thrilled"},
	}},
	{a: "dan", b: "bob", talk: []scripted{
		{who: "bob", text: "can you review the seen by change before i merge?"},
		{who: "dan", text: "looking now. the list is sorted by read time, right?"},
		{who: "bob", text: "yes, newest first. tell me if it reads oddly"},
		{who: "dan", text: "it reads fine. one nit: the header says read, not seen"},
		{who: "bob", text: "good catch, changing it. thanks"},
		{who: "dan", text: "approved. merge when the tests are green"},
		{who: "bob", text: "they are green, merged"},
		{who: "dan", text: "nice. take the afternoon off after the release"},
		{who: "bob", text: "planning to. the garden needs me"},
	}},
	{a: "dan", b: "carol", talk: []scripted{
		{who: "carol", text: "the screenshots are in docs/public, do you want them all?"},
		{who: "dan", text: "only the ones for the roadmap. the rest are in the tapes"},
		{who: "carol", text: "ok, i will pick three"},
		{who: "dan", text: "three is perfect. keep them small, the page loads fast that way"},
		{who: "carol", text: "done. 40 kb each"},
		{who: "dan", text: "great, thanks carol"},
	}},
	{a: "alice", b: "bob", talk: []scripted{
		{who: "alice", text: "bob, the zen mode key feels odd to me. z hides the panes, z again brings them back"},
		{who: "bob", text: "it is quick once you get used to it"},
		{who: "alice", text: "i keep pressing it twice by accident"},
		{who: "bob", text: "same. i do z s to toggle the times too, which is worse"},
		{who: "alice", text: "maybe we should write that in the docs"},
		{who: "bob", text: "already on my list. one page for zen, one for times"},
	}},
	{a: "alice", b: "carol", talk: []scripted{
		{who: "carol", text: "did you get the invite for the lunch?"},
		{who: "alice", text: "yes, count me in. what time?"},
		{who: "carol", text: "12:30 at the soup place. the window seat if we get there first"},
		{who: "alice", text: "i will be there at 12:25"},
		{who: "carol", text: "perfect, see you"},
	}},
	{a: "bob", b: "carol", talk: []scripted{
		{who: "bob", text: "carol, can you check the menu text for the members key?"},
		{who: "carol", text: "it says members under room. that is right"},
		{who: "bob", text: "ok good. i was not sure it was under the room menu"},
		{who: "carol", text: "it is space m w. the room menu has everything about the room"},
		{who: "bob", text: "thanks. adding it to the keys page"},
	}},
}

// loginAll logs in every demo user and returns their clients, keyed by name.
func loginAll(ctx context.Context, home string, ids map[string]string) (map[string]*mautrix.Client, error) {
	clients := map[string]*mautrix.Client{}
	for name, uid := range ids {
		cli, err := mautrix.NewClient(home, id.UserID(uid), "")
		if err != nil {
			return nil, err
		}
		if _, err := cli.Login(ctx, &mautrix.ReqLogin{
			Type:             mautrix.AuthTypePassword,
			Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: name},
			Password:         demoPass,
			StoreCredentials: true,
		}); err != nil {
			return nil, fmt.Errorf("login %s: %w", name, err)
		}
		clients[name] = cli
	}
	return clients, nil
}

// playConversation posts the main talk and the threads in roomID as the people in clients, and pins every
// line marked pin. The clients must have joined roomID already.
func playConversation(ctx context.Context, clients map[string]*mautrix.Client, roomID id.RoomID) error {
	var pins []id.EventID
	roots := map[string]id.EventID{}
	// the main talk first; threads are played after, so their roots sit after the talk and read as side talks
	for _, m := range append(conversation, threads...) {
		cli := clients[m.who]
		var resp *mautrix.RespSendEvent
		var err error
		if m.in != "" {
			resp, err = cli.SendMessageEvent(ctx, roomID, event.EventMessage, &event.MessageEventContent{
				MsgType:   event.MsgText,
				Body:      m.text,
				RelatesTo: &event.RelatesTo{Type: event.RelThread, EventID: roots[m.in]},
			})
		} else {
			resp, err = cli.SendText(ctx, roomID, m.text)
		}
		if err != nil {
			return fmt.Errorf("send %q: %w", m.text, err)
		}
		if m.root != "" {
			roots[m.root] = resp.EventID
		}
		if m.pin {
			pins = append(pins, resp.EventID)
		}
		if m.react != "" {
			if _, err := clients["dan"].SendReaction(ctx, roomID, resp.EventID, m.react); err != nil {
				return fmt.Errorf("react: %w", err)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := clients["dan"].SendStateEvent(ctx, roomID, event.StatePinnedEvents, "", &event.PinnedEventsEventContent{
		Pinned: pins,
	}); err != nil {
		return fmt.Errorf("pin: %w", err)
	}
	return nil
}

// playDMs opens each direct message in dms (the first person creates it and invites the second, who joins)
// and posts its talk.
func playDMs(ctx context.Context, clients map[string]*mautrix.Client) error {
	for _, d := range dms {
		room, err := clients[d.a].CreateRoom(ctx, &mautrix.ReqCreateRoom{
			Preset:   "trusted_private_chat",
			IsDirect: true,
			Invite:   []id.UserID{id.UserID(clients[d.b].UserID)},
		})
		if err != nil {
			return fmt.Errorf("create dm %s-%s: %w", d.a, d.b, err)
		}
		if _, err := clients[d.b].JoinRoomByID(ctx, room.RoomID); err != nil {
			return fmt.Errorf("join dm %s: %w", d.b, err)
		}
		// m.direct tells each client the room is their DM with that person; "@user" looks it up there
		for _, pair := range [][2]string{{d.a, d.b}, {d.b, d.a}} {
			cli := clients[pair[0]]
			other := id.UserID(clients[pair[1]].UserID)
			direct := map[id.UserID][]id.RoomID{}
			if err := cli.GetAccountData(ctx, "m.direct", &direct); err != nil {
				direct = map[id.UserID][]id.RoomID{}
			}
			direct[other] = append(direct[other], room.RoomID)
			if err := cli.SetAccountData(ctx, "m.direct", direct); err != nil {
				return fmt.Errorf("set m.direct %s: %w", pair[0], err)
			}
		}
		for _, m := range d.talk {
			if _, err := clients[m.who].SendText(ctx, room.RoomID, m.text); err != nil {
				return fmt.Errorf("dm %q: %w", m.text, err)
			}
		}
		fmt.Println("dm", d.a, d.b, room.RoomID)
	}
	return nil
}
