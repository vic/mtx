//go:build seed

package main

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// seedRoom is a public room in the demo space: every demo user joins it and some talk happens in it.
type seedRoom struct {
	name, topic string
	talk        []scripted
}

// seedRooms fill the space and the channel list, so the UI never looks empty.
var seedRooms = []seedRoom{
	{name: "lounge", topic: "casual talk, no meetings in here", talk: []scripted{
		{who: "erin", text: "hi all, new here. is this where the off topic stuff goes?"},
		{who: "frank", text: "yes, anything that is not the release. welcome"},
		{who: "grace", text: "what is the best coffee shop near the office?"},
		{who: "heidi", text: "the corner one with the blue door, do not get the filter"},
		{who: "erin", text: "noted, thanks heidi"},
		{who: "frank", text: "anyone watching the cycling this weekend?"},
		{who: "grace", text: "only the last stage, the rest is too long for me"},
	}},
	{name: "random", topic: "links, pictures and nonsense", talk: []scripted{
		{who: "heidi", text: "this font pairing is lovely https://example.org/fonts"},
		{who: "erin", text: "the spacing on the second line is off, though"},
		{who: "heidi", text: "true, but the mood is right"},
		{who: "frank", text: "my plant finally flowered after three years"},
		{who: "grace", text: "send a picture, i need proof"},
		{who: "frank", text: "it is on the windowsill, you cannot see it from here"},
	}},
	{name: "nix", topic: "flakes, modules and the odd build failure", talk: []scripted{
		{who: "bob", text: "anyone else get a hash mismatch after the last channel bump?"},
		{who: "dan", text: "yes, pin the input for now and update next week"},
		{who: "erin", text: "pinning worked. the overlay still breaks on aarch64"},
		{who: "bob", text: "open an issue with the log, i will look at it tomorrow"},
		{who: "frank", text: "the nix shell from the readme is the quickest start i have seen"},
		{who: "dan", text: "thanks. that was the goal"},
	}},
	{name: "docs", topic: "the docs site and the tapes", talk: []scripted{
		{who: "carol", text: "the keys page is long. should it split by mode?"},
		{who: "dan", text: "it is one table on purpose, so you can search it with /"},
		{who: "alice", text: "fine by me. the roadmap page is the one people read first"},
		{who: "carol", text: "i will try a quick read of it tonight"},
		{who: "dan", text: "great, tell me what is confusing"},
	}},
	{name: "release", topic: "release day, shared notes", talk: []scripted{
		{who: "alice", text: "the release checklist is in the pinned message"},
		{who: "bob", text: "checked the tag names, all good"},
		{who: "dan", text: "docs are live once the site builds. i will post the link"},
		{who: "grace", text: "i will test the install from a clean machine"},
		{who: "heidi", text: "the flake check passed on my side too"},
		{who: "grace", text: "install works. the first login prompt is clear"},
	}},
	{name: "games", topic: "after work games", talk: []scripted{
		{who: "frank", text: "friday game night? same time as always"},
		{who: "erin", text: "i am in, bring the board game this time"},
		{who: "grace", text: "last time was the card game, you lost badly"},
		{who: "frank", text: "i was practicing. this time i win"},
		{who: "heidi", text: "i will bring snacks, the chips are the real prize"},
	}},
	{name: "music", topic: "what is on the speakers", talk: []scripted{
		{who: "erin", text: "the new album from last month is really good"},
		{who: "frank", text: "which track? i only got through side a"},
		{who: "erin", text: "the slow one in the middle, it grows on you"},
		{who: "heidi", text: "i will put it on the list for the drive home"},
		{who: "grace", text: "keep the volume low, the open office is quiet today"},
	}},
	{name: "cats", topic: "for the good of all cats", talk: []scripted{
		{who: "alice", text: "my cat sleeps on the keyboard every day now"},
		{who: "heidi", text: "mine sits on the printer, the printer is never on time"},
		{who: "frank", text: "no pictures from me, but the neighbour cat visits the balcony"},
		{who: "alice", text: "send it over, we need evidence"},
		{who: "frank", text: "it ran away when i got the phone out"},
	}},
	{name: "coffee", topic: "coffee, tea and the odd biscuit", talk: []scripted{
		{who: "grace", text: "the new beans from the roaster are very nice"},
		{who: "bob", text: "which origin? the last ones were too acidic"},
		{who: "grace", text: "ethiopian, light roast. fruity but balanced"},
		{who: "carol", text: "i will get a bag on friday"},
		{who: "erin", text: "the machine in the kitchen needs descaling, again"},
	}},
	{name: "travel", topic: "trips, trains and bad hotels", talk: []scripted{
		{who: "erin", text: "the train to the coast is cancelled tomorrow"},
		{who: "frank", text: "take the bus through the hills, it is slower but fine"},
		{who: "heidi", text: "i did that last year, bring a jacket, the wind is cold"},
		{who: "erin", text: "good tip. the bus it is"},
	}},
	{name: "books", topic: "read anything good lately?", talk: []scripted{
		{who: "carol", text: "just finished a long novel about a lighthouse keeper"},
		{who: "alice", text: "that one is on my list, is it slow at the start?"},
		{who: "carol", text: "a bit, but the second half makes up for it"},
		{who: "grace", text: "i am reading the nix manual again, it never ends"},
		{who: "bob", text: "lol, the good part is at the end of the chapter"},
	}},
	{name: "help", topic: "ask here, someone will answer", talk: []scripted{
		{who: "erin", text: "how do i change my display name? the nick command does not work for me"},
		{who: "alice", text: ":nick followed by the new name, then enter"},
		{who: "erin", text: "it works, thanks. i typed the colon wrong"},
		{who: "dan", text: "it also shows in the nicks pane after the next sync"},
		{who: "erin", text: "perfect, the pane already changed"},
	}},
	{name: "offtopic", topic: "anything goes, be kind", talk: []scripted{
		{who: "frank", text: "which is the best pen for notes? i am tired of the cheap ones"},
		{who: "heidi", text: "a fine liner, 0.4 mm, it does not smudge"},
		{who: "grace", text: "i still use a pencil. nothing beats it"},
		{who: "erin", text: "pencil on a laptop, nice try grace"},
	}},
	{name: "garden", topic: "tomatoes, slugs and the shed", talk: []scripted{
		{who: "bob", text: "the tomatoes are finally red, first harvest this week"},
		{who: "heidi", text: "the slugs got my lettuce again"},
		{who: "bob", text: "beer traps work, the slugs love it"},
		{who: "heidi", text: "i will try that, the shed is full of empty bottles anyway"},
	}},
	{name: "standup", topic: "daily notes, short please", talk: []scripted{
		{who: "alice", text: "yesterday: release notes. today: the search copy"},
		{who: "bob", text: "yesterday: the theme picker. today: the seen by list"},
		{who: "carol", text: "yesterday: screenshots. today: the keys page"},
		{who: "dan", text: "yesterday: the seed. today: pages and the site build"},
		{who: "erin", text: "yesterday: testing installs. today: the nix shell"},
	}},
}

// playRooms creates each seedRoom in the space, has every client join it, posts its talk, and adds it as a child.
func playRooms(ctx context.Context, clients map[string]*mautrix.Client, space id.RoomID) error {
	dan := clients["dan"]
	for _, r := range seedRooms {
		room, err := dan.CreateRoom(ctx, &mautrix.ReqCreateRoom{
			Name:          r.name,
			Topic:         r.topic,
			Preset:        "public_chat",
			RoomAliasName: r.name,
		})
		if err != nil {
			return fmt.Errorf("create %s: %w", r.name, err)
		}
		for name, cli := range clients {
			if name == "dan" {
				continue
			}
			if _, err := cli.JoinRoomByID(ctx, room.RoomID); err != nil {
				return fmt.Errorf("join %s to %s: %w", name, r.name, err)
			}
		}
		for _, m := range r.talk {
			if _, err := clients[m.who].SendText(ctx, room.RoomID, m.text); err != nil {
				return fmt.Errorf("send to %s: %w", r.name, err)
			}
		}
		if _, err := dan.SendStateEvent(ctx, space, event.StateSpaceChild, string(room.RoomID), map[string]any{
			"via": []string{"oeiuwq.com"},
		}); err != nil {
			return fmt.Errorf("space child %s: %w", r.name, err)
		}
		fmt.Println("room", r.name, room.RoomID)
	}
	return nil
}
