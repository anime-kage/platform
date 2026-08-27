// The honeypot channel: a trap for compromised accounts and scam bots.
//
// The attack it answers is the one every Discord server eventually sees. An
// account gets phished, and within seconds it walks the channel list posting
// the same "free Nitro" link in every channel it can write to. By the time a
// human moderator notices, the link is in thirty channels and the member who
// clicked it has already lost their own account to the same page.
//
// The defence is that the spam is indiscriminate, and a moderator is not. We
// keep one channel whose only message says, in so many words, do not post here.
// No member has a reason to type in it. Software walking the channel list does
// not read the sign, so posting there is a confession: mute first, then delete
// everything the account has said anywhere in the last few minutes.
//
// Crunchyroll's server and most large ones run some version of this. What makes
// it safe rather than clever is what it refuses to do:
//
//   - staff are exempt, checked by permission and not by a list somebody has to
//     maintain. A moderator poking the trap to see if it works does not lock
//     themselves out of their own server.
//   - it mutes, it does not ban. A false positive is undone by removing a role.
//   - the mute lands before the purge. The purge takes seconds across a big
//     server, and every one of those seconds is a channel the spammer would
//     otherwise still be posting in.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// protectionMarker identifies the sign we post in the honeypot channel, the
// same way boardMarker identifies the invite board.
const protectionMarker = "NU SCRIE ÎN ACEST CANAL"

// retiredProtectionMarkers are wordings this sign used to carry. findMessages
// still matches them, so a rewrite edits the message already in the channel
// instead of posting a second one next to a copy it no longer recognises.
var retiredProtectionMarkers = []string{"Nu scrie în acest canal"}

// punishCooldown stops one spammer from starting a dozen purges. A raid dumps
// several messages into the trap before the mute takes effect, and each of them
// arrives as its own event; the first one has already handled the account.
const punishCooldown = 10 * time.Minute

// maxTimeout is Discord's ceiling on a native timeout. It is the fallback when
// no mute role is configured, and it is why a role is preferred: 28 days from
// now, a hijacked account is quietly free to post again.
const maxTimeout = 28 * 24 * time.Hour

// purgePagesPerChannel caps how deep the purge digs in any one channel. 500
// messages is far past what a 5-minute window can hold in normal traffic, and
// the cap is what keeps a raid on a busy server from turning one mute into
// thousands of API calls.
const purgePagesPerChannel = 5

// staffPermissions are the powers that make somebody staff for our purposes.
//
// Deliberately generous: every one of these already lets its holder do worse
// than post in a honeypot, so treating them as exempt gives away nothing, while
// muting the person who can unmute them is a genuinely bad afternoon.
const staffPermissions = discordgo.PermissionAdministrator |
	discordgo.PermissionManageGuild |
	discordgo.PermissionManageMessages |
	discordgo.PermissionModerateMembers |
	discordgo.PermissionBanMembers |
	discordgo.PermissionManageRoles

// protectionNotice is the sign, and it is deliberately four lines.
//
// The audience is somebody scrolling past a channel list, not somebody reading.
// An explanation of what a honeypot is and why it exists would be more honest
// and would be read by nobody — the only sentence that has to land is that
// posting here gets you muted. So: a heading Discord renders at double size, a
// second one for the consequence, and three flagged lines. Everything else that
// used to be here (how it works, who to contact) was cut on purpose.
func (b *bot) protectionNotice() string {
	// Say which mute it actually is. Promising "permanent" while the fallback
	// timeout quietly expires after 28 days would make the sign a lie the first
	// time somebody counted the days.
	consequence := "MUT AUTOMAT"
	if b.cfg.MutedRoleID != "" {
		consequence = "MUT PERMANENT"
	}

	var sb strings.Builder
	sb.WriteString("# 🚨 " + protectionMarker + " 🚨\n")
	sb.WriteString("## 🔴 Orice mesaj de aici = " + consequence + "\n")
	sb.WriteString("🚩 Automat, instant, fără avertisment.\n")
	sb.WriteString("🚩 Mesajele tale din ultimele " + humanWait(b.cfg.PurgeWindow) + " se șterg din tot serverul.\n")
	sb.WriteString("🚩 Canalul este o capcană anti-spam. Nu răspunde nimeni aici.")
	return sb.String()
}

// ensureProtectionNotice puts the sign up, once, and updates its text on deploy.
//
// Not fatal if it fails: the trap works without a sign. The sign is what makes
// it fair — a member who wanders in has been told, and the ones who have not
// read it are exactly the software we are aiming at.
func (b *bot) ensureProtectionNotice(s *discordgo.Session) {
	if b.cfg.ProtectionChannelID == "" {
		slog.Info("no protection channel configured — honeypot disabled")
		return
	}
	if b.cfg.MutedRoleID == "" {
		slog.Warn("no DISCORD_MUTED_ROLE_ID set — the honeypot will fall back to a 28-day timeout, which expires")
	}
	id, err := b.ensureMessage(s, b.cfg.ProtectionChannelID,
		append([]string{protectionMarker}, retiredProtectionMarkers...), b.protectionNotice(), nil)
	if err != nil {
		slog.Error("post protection notice", "err", err)
		return
	}
	slog.Info("honeypot armed",
		"channel", b.cfg.ProtectionChannelID,
		"notice", id,
		"purgeWindow", b.cfg.PurgeWindow,
		"muteRole", b.cfg.MutedRoleID)
}

// punish handles one message in the honeypot channel.
func (b *bot) punish(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" {
		return // not in a server, so there is nothing to mute them in
	}
	if m.Author.Bot {
		// Another bot cannot be muted and is usually ours or a webhook posting
		// something legitimate. Worth a line, not a moderation action.
		slog.Info("bot posted in the honeypot, ignoring", "user", m.Author.ID, "username", m.Author.Username)
		return
	}
	if b.exempt(s, m) {
		slog.Info("staff posted in the honeypot, ignoring", "user", m.Author.ID, "username", m.Author.Username)
		return
	}
	if !b.claimPunish(m.Author.ID) {
		return // already handled by the message that arrived a moment ago
	}

	// Mute first. Everything below this line takes seconds, and the point of
	// the trap is that the account stops posting during them.
	muted, muteErr := b.mute(s, m.GuildID, m.Author.ID)
	if muteErr != nil {
		slog.Error("mute honeypot trigger", "user", m.Author.ID, "err", muteErr)
	}

	since := time.Now().Add(-b.cfg.PurgeWindow)
	deleted := b.purge(s, m.GuildID, m.Author.ID, since)

	slog.Info("honeypot triggered",
		"user", m.Author.ID,
		"username", m.Author.Username,
		"mute", muted,
		"deleted", deleted)
	b.modLog(s, m.Author, muted, muteErr, deleted)
}

// claimPunish returns true for the first message from an account inside the
// cooldown and false for the rest, so a burst produces one purge.
func (b *bot) claimPunish(userID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if last, ok := b.punished[userID]; ok && now.Sub(last) < punishCooldown {
		return false
	}
	// Sweep while we hold the lock. The map is only ever as big as the number
	// of accounts that tripped the trap in the last ten minutes, but a long
	// raid should not leave it growing for the life of the process.
	for id, at := range b.punished {
		if now.Sub(at) >= punishCooldown {
			delete(b.punished, id)
		}
	}
	b.punished[userID] = now
	return true
}

// mute silences the account twice over: a native timeout, and the muted role if
// one is configured. It returns a Romanian description of what actually landed,
// which goes straight into the mod log.
//
// Both, not one or the other, because they fail in opposite directions.
//
// The timeout is the half that cannot be misconfigured — Discord enforces it
// itself, server-wide, no matter how the roles are set up — but it expires after
// 28 days, which is its hard ceiling and not a choice. The muted role never
// expires, but it is only as good as the channel overrides behind it: a role's
// own toggles can only *grant* permissions, so a "Muted" role with everything
// switched off silences nobody until each category denies Send Messages to it.
//
// Applying both means a role that was set up wrong still produces 28 days of
// real silence to notice it in, and a correctly set up role still holds on day
// 29. Neither failure is silent.
func (b *bot) mute(s *discordgo.Session, guildID, userID string) (string, error) {
	var done, failed []string

	until := time.Now().Add(maxTimeout)
	if err := s.GuildMemberTimeout(guildID, userID, &until); err != nil {
		failed = append(failed, fmt.Sprintf("timeout (%v)", err))
	} else {
		done = append(done, "timeout de 28 de zile")
	}

	if b.cfg.MutedRoleID != "" {
		// Needs Manage Roles, and needs the bot's own top role to sit above the
		// muted role — Discord refuses to hand out a role at or above your own,
		// and reports it as a flat 403 that says nothing about hierarchy.
		if err := s.GuildMemberRoleAdd(guildID, userID, b.cfg.MutedRoleID); err != nil {
			failed = append(failed, fmt.Sprintf("rol de mut %s (%v)", b.cfg.MutedRoleID, err))
		} else {
			done = append(done, "rol de mut, permanent")
		}
	}

	if len(failed) > 0 {
		slog.Warn("part of the mute did not apply", "user", userID, "failed", strings.Join(failed, "; "))
	}
	if len(done) == 0 {
		return "", errors.New(strings.Join(failed, "; "))
	}
	summary := strings.Join(done, " + ")
	if len(failed) > 0 {
		summary += " (eșuat: " + strings.Join(failed, "; ") + ")"
	}
	return summary, nil
}

// exempt reports whether the author is staff or the server owner.
//
// Computed from the roles the member actually holds rather than from a list in
// the config, because the config would go stale the first time somebody is
// promoted and nobody would find out until the trap ate a moderator.
func (b *bot) exempt(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	guild, err := s.State.Guild(m.GuildID)
	if err != nil {
		if guild, err = s.Guild(m.GuildID); err != nil {
			// Cannot tell. Err towards not punishing: a scam link that survives
			// two more minutes is recoverable, a muted moderator during an
			// outage is not.
			slog.Warn("look up guild for the staff check, treating as exempt", "guild", m.GuildID, "err", err)
			return true
		}
	}
	if guild.OwnerID == m.Author.ID {
		return true
	}

	member := m.Member
	if member == nil {
		if member, err = s.GuildMember(m.GuildID, m.Author.ID); err != nil {
			slog.Warn("look up member for the staff check, treating as exempt", "user", m.Author.ID, "err", err)
			return true
		}
	}

	perms := map[string]int64{}
	for _, r := range guild.Roles {
		perms[r.ID] = r.Permissions
	}
	// @everyone is a role whose ID is the guild's, and it is the one role a
	// member never lists. A server that grants it Manage Messages has bigger
	// problems, but the check would be wrong without it.
	total := perms[m.GuildID]
	for _, id := range member.Roles {
		total |= perms[id]
	}
	return total&staffPermissions != 0
}

// purge deletes everything the account has posted since `since`, everywhere in
// the server, and returns how many messages went.
//
// There is no API for "delete this user's recent messages" short of banning
// them, which is the one thing we are not doing — so this is the honest version:
// walk every channel the bot can read, take the messages that match, bulk-delete
// them per channel. Sequential on purpose. discordgo serialises per route
// anyway, and a purge that trips the global rate limit would stall the mute of
// whoever trips the trap next.
func (b *bot) purge(s *discordgo.Session, guildID, userID string, since time.Time) int {
	channels, err := s.GuildChannels(guildID)
	if err != nil {
		slog.Error("list channels for purge", "guild", guildID, "err", err)
		return 0
	}
	// Threads are separate channels and are not in the list above. A spammer
	// that walked into an open thread would otherwise keep their message there.
	if threads, terr := s.GuildThreadsActive(guildID); terr != nil {
		slog.Warn("list active threads for purge", "guild", guildID, "err", terr)
	} else {
		channels = append(channels, threads.Threads...)
	}

	total := 0
	for _, ch := range channels {
		if !hasMessages(ch) {
			continue
		}
		ids := recentBy(s, ch.ID, userID, since)
		if len(ids) == 0 {
			continue
		}
		// Bulk delete takes 100 at a time and refuses messages older than two
		// weeks; our window is minutes, so only the batch size can bite here.
		for start := 0; start < len(ids); start += 100 {
			end := min(start+100, len(ids))
			if derr := s.ChannelMessagesBulkDelete(ch.ID, ids[start:end]); derr != nil {
				slog.Warn("bulk delete", "channel", ch.ID, "count", end-start, "err", derr)
				continue
			}
			total += end - start
		}
	}
	return total
}

// recentBy returns the IDs of the account's messages in one channel, newest
// first, back to `since`.
func recentBy(s *discordgo.Session, channelID, userID string, since time.Time) []string {
	var ids []string
	before := ""
	for page := 0; page < purgePagesPerChannel; page++ {
		batch, err := s.ChannelMessages(channelID, 100, before, "", "")
		if err != nil {
			// Almost always a channel the bot cannot read. Not worth an error:
			// on a big server this fires for every staff-only channel, every
			// time, and a log full of it would hide the real failures.
			slog.Debug("read channel for purge", "channel", channelID, "err", err)
			return ids
		}
		if len(batch) == 0 {
			return ids
		}
		for _, msg := range batch {
			// Messages come back newest first, so the first one older than the
			// window ends the search for this channel.
			if msg.Timestamp.Before(since) {
				return ids
			}
			if msg.Author != nil && msg.Author.ID == userID {
				ids = append(ids, msg.ID)
			}
		}
		before = batch[len(batch)-1].ID
	}
	return ids
}

// hasMessages reports whether a channel is one people can type in. Categories
// and stage channels are not; voice channels are, since Discord gave them a
// text chat, and a spammer's script does not skip them.
func hasMessages(ch *discordgo.Channel) bool {
	switch ch.Type {
	case discordgo.ChannelTypeGuildText,
		discordgo.ChannelTypeGuildNews,
		discordgo.ChannelTypeGuildVoice,
		discordgo.ChannelTypeGuildPublicThread,
		discordgo.ChannelTypeGuildPrivateThread,
		discordgo.ChannelTypeGuildNewsThread:
		return true
	default:
		return false
	}
}

// modLog reports the action to the moderators' channel, if there is one.
//
// Worth the extra config: an automatic permanent mute that leaves no trace is
// indistinguishable, from the muted member's side, from the server being broken.
func (b *bot) modLog(s *discordgo.Session, user *discordgo.User, muted string, muteErr error, deleted int) {
	if b.cfg.ModLogChannelID == "" {
		return
	}
	var sb strings.Builder
	sb.WriteString("🥷 **Protecție automată**\n")
	sb.WriteString(fmt.Sprintf("<@%s> (`%s`, ID `%s`) a scris în <#%s>.\n",
		user.ID, user.Username, user.ID, b.cfg.ProtectionChannelID))
	if muteErr != nil {
		sb.WriteString("⚠️ Mut **eșuat**: `" + muteErr.Error() + "`. Ocupați-vă manual.\n")
	} else {
		sb.WriteString("Aplicat: " + muted + ".\n")
	}
	sb.WriteString(fmt.Sprintf("Mesaje șterse din ultimele %s: **%d**.", humanWait(b.cfg.PurgeWindow), deleted))
	if _, err := s.ChannelMessageSend(b.cfg.ModLogChannelID, sb.String()); err != nil {
		slog.Warn("write to the mod log", "channel", b.cfg.ModLogChannelID, "err", err)
	}
}
