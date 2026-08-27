// Sticky messages: keeping one bot-owned message at the bottom of a channel.
//
// Discord has no "always show this last" feature. Pinning is the closest thing
// it ships, and it is not close: a pin lives behind a button most members never
// press, and the pinned message itself still scrolls away with everything else.
// The bots that solve this (StickyBot and its clones) all do the same crude
// thing, and so do we — when somebody posts in the channel, delete our copy and
// post it again underneath. The message is therefore never more than one post
// from the bottom, at the cost of a new message ID every time.
//
// Two things keep that from turning into channel spam:
//
//   - a debounce. A burst of ten messages produces one repost, not ten, because
//     the signal channel below holds at most one pending restick.
//   - a check that the board is not already the last message, which is the
//     common case after the debounce has swallowed a burst.
package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// stickyDelay is how long we wait after a member posts before moving the board
// back down. Long enough that a conversation does not fight the bot for the
// bottom of the channel, short enough that the board is back before anyone
// scrolls up looking for it.
const stickyDelay = 4 * time.Second

// signalRestick asks the sticky loop to move the board to the bottom.
//
// Non-blocking on purpose: the channel is buffered to one, so a raid that posts
// a hundred messages leaves exactly one pending restick behind rather than a
// hundred queued API calls.
func (b *bot) signalRestick() {
	select {
	case b.restick <- struct{}{}:
	default:
	}
}

// runSticky owns every repost. Keeping it in one goroutine is what makes the
// bookkeeping safe: nothing else deletes or replaces the board, so there is no
// window where two reposts both think they hold the current message ID.
func (b *bot) runSticky(s *discordgo.Session) {
	for range b.restick {
		// Let the burst finish before reacting. Anything that lands during the
		// wait is covered by the repost we are about to do, so drain it.
		time.Sleep(stickyDelay)
		select {
		case <-b.restick:
		default:
		}
		b.moveBoardToBottom(s)
	}
}

func (b *bot) moveBoardToBottom(s *discordgo.Session) {
	b.mu.Lock()
	old := b.boardID
	b.mu.Unlock()
	if old == "" {
		return // no board to move; ensureInviteBoard could not post one
	}

	// Already at the bottom — the usual outcome once the debounce has folded a
	// burst into one restick, and worth one cheap read to avoid the churn.
	if last, err := s.ChannelMessages(b.cfg.ChannelID, 1, "", "", ""); err == nil &&
		len(last) == 1 && last[0].ID == old {
		return
	}

	// Post first, delete second. The reverse order leaves the channel with no
	// board at all if the send fails, and the send is the half that can fail
	// for reasons outside our control (rate limit, outage, lost permission).
	msg, err := s.ChannelMessageSendComplex(b.cfg.ChannelID, &discordgo.MessageSend{
		Content:    b.boardText(),
		Components: inviteComponents(),
	})
	if err != nil {
		slog.Error("restick invite board", "err", err)
		return
	}
	b.mu.Lock()
	b.boardID = msg.ID
	b.mu.Unlock()

	// Deleting our own message needs no Manage Messages, so this failing means
	// somebody deleted it first. Not worth more than a line in the log.
	if err := s.ChannelMessageDelete(b.cfg.ChannelID, old); err != nil {
		slog.Warn("delete previous invite board", "message", old, "err", err)
	}
}

// ensureMessage leaves exactly one copy of a bot-owned message in a channel and
// returns its ID.
//
// Idempotent across restarts and across text changes, which is the point. The
// bot's own messages cannot be edited from the Discord client by anybody, not
// even the server owner, so a copy edit that only took effect on a *fresh* post
// would mean deleting the message by hand before every deploy. Here the deploy
// does it: same marker, different body, so we edit in place.
func (b *bot) ensureMessage(s *discordgo.Session, channelID string, markers []string, content string, components []discordgo.MessageComponent) (string, error) {
	existing := b.findMessages(s, channelID, markers)
	if len(existing) == 0 {
		msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:    content,
			Components: components,
		})
		if err != nil {
			return "", err
		}
		slog.Info("posted bot message", "marker", markers[0], "message", msg.ID, "channel", channelID)
		return msg.ID, nil
	}

	// Newest wins. Older copies are leftovers from a deploy that could not
	// delete its predecessor, and a channel with two live invite buttons is a
	// channel where one of them is wrong.
	keep := existing[0]
	for _, stale := range existing[1:] {
		if err := s.ChannelMessageDelete(channelID, stale.ID); err != nil {
			slog.Warn("delete duplicate bot message", "message", stale.ID, "err", err)
			continue
		}
		slog.Info("removed duplicate bot message", "marker", markers[0], "message", stale.ID)
	}

	if keep.Content != content {
		edit := discordgo.NewMessageEdit(channelID, keep.ID).SetContent(content)
		if components != nil {
			edit.Components = &components
		}
		if _, err := s.ChannelMessageEditComplex(edit); err != nil {
			return keep.ID, err
		}
		slog.Info("updated bot message text", "marker", markers[0], "message", keep.ID)
	}
	return keep.ID, nil
}

// findMessages returns our copies of a marked message still present in a
// channel, newest first.
//
// Recent history is the authoritative source because it is ordered; the pins
// are checked second to catch a message from before sticky mode, which was
// pinned and may by now have scrolled past the history window we read.
//
// Several markers are accepted so that a rewrite which changes the very line we
// match on still recognises the copy already sitting in the channel. Without
// that, the first deploy after such a rewrite adopts nothing, posts a second
// message, and leaves both there for good — each restart matching only the new
// one. Retired markers stay in the list until the old message is certainly gone.
func (b *bot) findMessages(s *discordgo.Session, channelID string, markers []string) []*discordgo.Message {
	mine := func(m *discordgo.Message) bool {
		if m.Author == nil || m.Author.ID != s.State.User.ID {
			return false
		}
		for _, marker := range markers {
			if strings.Contains(m.Content, marker) {
				return true
			}
		}
		return false
	}

	var found []*discordgo.Message
	seen := map[string]bool{}

	if recent, err := s.ChannelMessages(channelID, 50, "", "", ""); err != nil {
		// Needs Read Message History. Without it we cannot tell what is there,
		// and posting is the better failure: a duplicate beats no message.
		slog.Warn("read recent messages (needs Read Message History)", "channel", channelID, "err", err)
	} else {
		for _, m := range recent {
			if mine(m) {
				found = append(found, m)
				seen[m.ID] = true
			}
		}
	}

	if pinned, err := s.ChannelMessagesPinned(channelID); err != nil {
		slog.Warn("list pinned messages", "channel", channelID, "err", err)
	} else {
		for _, m := range pinned {
			if mine(m) && !seen[m.ID] {
				found = append(found, m)
			}
		}
	}
	return found
}
