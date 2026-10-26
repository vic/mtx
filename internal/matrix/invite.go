package matrix

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Invite describes an invitation the loop just accepted.
type Invite struct {
	Room    id.RoomID
	Alias   string // canonical alias from the invite state, "" when none
	Inviter id.UserID
}

// OnInvite sets the hook called for each pending invite in a sync.
func (c *Client) OnInvite(fn func(Invite)) { c.onInvite = fn }

// inviteOf reads the inviter (the sender of our own m.room.member invite) and the canonical alias
// from the stripped invite state.
func (c *Client) inviteOf(room id.RoomID, inv *mautrix.SyncInvitedRoom) (Invite, error) {
	in := Invite{Room: room}
	for _, st := range inv.State.Events {
		if err := st.Content.ParseRaw(st.Type); err != nil {
			return Invite{}, fmt.Errorf("invite state %s: %w", room, err)
		}
		switch {
		case st.Type == event.StateMember && st.GetStateKey() == c.user.String():
			in.Inviter = st.Sender
		case st.Type == event.StateCanonicalAlias:
			if a := st.Content.AsCanonicalAlias(); a != nil {
				in.Alias = string(a.Alias)
			}
		}
	}
	return in, nil
}

// Accept joins an invited room.
func (c *Client) Accept(ctx context.Context, room id.RoomID) error {
	_, err := c.cli.JoinRoomByID(ctx, room)
	return err
}

// Reject declines an invite (leaves the invited room).
func (c *Client) Reject(ctx context.Context, room id.RoomID) error {
	_, err := c.cli.LeaveRoom(ctx, room)
	return err
}
