// The Discord bot — the front door to an
// invite-only community, and the trap at the side entrance.
//
// A member runs /invitatie in the invites channel, or presses the button on the
// board the bot keeps at the bottom of that channel, and gets a single-use
// registration code visible only to them. Separately, anyone who posts in the
// honeypot channel is muted and their last few minutes of messages are deleted
// server-wide — see protection.go for why that is a trap and not a rule.
//
// It is a sibling binary in this module rather than a separate service so it
// shares internal/repo with the API: the invite rules — quota, expiry, the
// atomic claim at registration — have exactly one implementation, and the
// bot cannot drift from what the site actually enforces.
//
// Run:
//
//	DISCORD_TOKEN=… DISCORD_GUILD_ID=… DISCORD_INVITE_CHANNEL_ID=… \
//	DATABASE_URL=… go run ./cmd/discordbot
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"

	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/repo"
)

func main() {
	if err := run(); err != nil {
		slog.Error("discord bot failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // optional .env, like every other binary here

	cfg, err := config.LoadBot()
	if err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return fmt.Errorf("discord session: %w", err)
	}
	// Both the sticky board and the honeypot need to know that a message
	// landed, and neither needs to know what it said — so MESSAGE_CONTENT, the
	// privileged intent that would need justifying to Discord and would make
	// this bot worth stealing, stays off. IntentsGuilds keeps the role list in
	// the cache, which is what the honeypot's staff check reads.
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	b := newBot(cfg, repo.New(pool))
	session.AddHandler(b.onInteraction)
	session.AddHandler(b.onMessage)

	if err := session.Open(); err != nil {
		return fmt.Errorf("open gateway: %w", err)
	}
	defer session.Close()
	slog.Info("connected", "user", session.State.User.Username)

	go b.runSticky(session)

	registered, err := session.ApplicationCommandCreate(session.State.User.ID, cfg.GuildID, &discordgo.ApplicationCommand{
		Name:        cfg.CommandName,
		Description: "Generează un cod de invitație pentru Anime-Kage",
	})
	if err != nil {
		return fmt.Errorf("register command /%s: %w", cfg.CommandName, err)
	}
	scope := "global (propagarea durează până la o oră)"
	if cfg.GuildID != "" {
		scope = "guild " + cfg.GuildID
	}
	slog.Info("command registered", "name", "/"+registered.Name, "scope", scope)

	// Drop any command we registered under a previous name.
	//
	// Renaming the command does not retire the old one: Discord keeps every
	// command this application ever registered until it is explicitly deleted.
	// Members would still see the stale entry, and picking it would hang with
	// "the application did not respond" — onInteraction ignores names it does
	// not recognise, so nothing ever answers.
	if existing, lerr := session.ApplicationCommands(session.State.User.ID, cfg.GuildID); lerr != nil {
		slog.Warn("list existing commands", "err", lerr)
	} else {
		for _, c := range existing {
			if c.ID == registered.ID {
				continue
			}
			if derr := session.ApplicationCommandDelete(session.State.User.ID, cfg.GuildID, c.ID); derr != nil {
				slog.Warn("delete stale command", "name", "/"+c.Name, "err", derr)
				continue
			}
			slog.Info("removed stale command", "name", "/"+c.Name)
		}
	}

	// The button half of the front door. Not fatal if it fails — the slash
	// command still works, and a bot that refuses to start because it could not
	// post a message would be worse than one missing a convenience.
	b.ensureInviteBoard(session)
	b.ensureProtectionNotice(session)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	return nil
}

type bot struct {
	cfg  *config.BotConfig
	repo *repo.Repo

	// mu guards the two pieces of state the gateway handlers share. Discord
	// delivers events concurrently, so "the board we posted last" and "who we
	// have already muted" are both read and written from several goroutines.
	mu sync.Mutex
	// boardID is the invite board's current message ID. It changes on every
	// restick, because sticky messages are deleted and reposted, not moved.
	boardID string
	// punished remembers who tripped the honeypot recently, so a burst of five
	// messages does not start five purges. See claimPunish.
	punished map[string]time.Time

	// restick is a one-slot signal to the sticky loop. Buffered, never closed,
	// and written to without blocking: a raid should cost one repost, not one
	// per message.
	restick chan struct{}
}

func newBot(cfg *config.BotConfig, r *repo.Repo) *bot {
	return &bot{
		cfg:      cfg,
		repo:     r,
		punished: map[string]time.Time{},
		restick:  make(chan struct{}, 1),
	}
}

// onMessage is the whole gateway-side surface: two channels, two reactions.
func (b *bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.ID == s.State.User.ID {
		return // our own board, our own notice, our own mod-log lines
	}
	switch m.ChannelID {
	case b.cfg.ChannelID:
		// Anything else posted here pushed the board up, including another
		// bot's message — so this deliberately does not skip bots.
		b.signalRestick()
	case b.cfg.ProtectionChannelID:
		// Off the gateway goroutine: a purge walks every channel in the server
		// and takes seconds, and discordgo dispatches handlers in order.
		go b.punish(s, m)
	}
}

// inviteButtonID identifies our button on the board. Discord echoes it
// back on every click, and it is how we tell our component apart from anyone
// else's in the same channel.
const inviteButtonID = "invite:request"

// boardMarker is the line we look for to recognise a board we already posted.
//
// Matching on text rather than on the button component is deliberate: message
// components come back from the API as interface values that need type
// assertions to inspect, and a one-line content check cannot break when
// discordgo changes how it unmarshals them.
const boardMarker = "Invitație Anime-Kage"

func (b *bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Two doors into the same room: the slash command for people who know
	// Discord, the sticky button for people who do not. Both mint a code the
	// same way — same quota, same expiry, same attribution to the clicker — so
	// neither can become a way around the rules the other enforces.
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if i.ApplicationCommandData().Name != b.cfg.CommandName {
			return
		}
	case discordgo.InteractionMessageComponent:
		if i.MessageComponentData().CustomID != inviteButtonID {
			return
		}
	default:
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	b.reply(s, i, b.handleInvite(ctx, i))
}

// ensureInviteBoard puts the "press this" board in the invite channel and hands
// it to the sticky loop.
//
// Idempotent across restarts and across copy changes: ensureMessage adopts a
// board we posted before, edits its text if this build's wording differs, and
// deletes any duplicates a previous deploy left behind. Without that, every
// restart would add another board and the channel would fill with dead buttons.
//
// Note what is missing: nothing is pinned any more. Sticky mode keeps the board
// one message from the bottom at all times, which is strictly better than a pin
// nobody clicks, and re-pinning after every restick would post a "so-and-so
// pinned a message" system line into the channel each time.
func (b *bot) ensureInviteBoard(s *discordgo.Session) {
	if b.cfg.ChannelID == "" {
		slog.Info("no invite channel configured — skipping the button board")
		return
	}

	id, err := b.ensureMessage(s, b.cfg.ChannelID, []string{boardMarker}, b.boardText(), inviteComponents())
	if err != nil {
		slog.Error("post invite board", "err", err)
		return
	}

	b.mu.Lock()
	b.boardID = id
	b.mu.Unlock()
	slog.Info("invite board ready", "message", id, "channel", b.cfg.ChannelID)

	// The board we just adopted is almost certainly buried under whatever was
	// said while the bot was down. One restick brings it back to the bottom;
	// if it is already there, moveBoardToBottom notices and does nothing.
	b.signalRestick()
}

func inviteComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Cere o invitație",
				Style:    discordgo.PrimaryButton,
				CustomID: inviteButtonID,
				Emoji:    &discordgo.ComponentEmoji{Name: "🎟️"},
			},
		}},
	}
}

func (b *bot) boardText() string {
	var sb strings.Builder
	sb.WriteString("**" + boardMarker + "**\n\n")
	sb.WriteString("Apasă butonul de mai jos și primești un cod de invitație. ")
	sb.WriteString("Îl vezi doar tu, nimeni altcineva din canal nu îl poate citi.\n\n")
	sb.WriteString("Codul merge o singură dată, la înregistrare pe " + b.cfg.PublicURL + "/register.")
	if b.cfg.InviteTTL > 0 {
		sb.WriteString(fmt.Sprintf(" Expiră după %s.", humanWait(b.cfg.InviteTTL)))
	}
	if b.cfg.QuotaWindow > 0 {
		sb.WriteString(fmt.Sprintf("\nPoți cere un cod nou %s.", everyPhrase(b.cfg.QuotaWindow)))
	}
	sb.WriteString("\n\n_Dacă preferi, merge și cu comanda `/" + b.cfg.CommandName + "`._")
	return sb.String()
}

// reply always answers ephemerally: the code is a secret, and an invite
// channel full of other people's codes would defeat the point.
func (b *bot) reply(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		slog.Error("respond to interaction", "err", err)
	}
}

func (b *bot) handleInvite(ctx context.Context, i *discordgo.InteractionCreate) string {
	if b.cfg.ChannelID != "" && i.ChannelID != b.cfg.ChannelID {
		return fmt.Sprintf("Comanda merge doar în <#%s>.", b.cfg.ChannelID)
	}

	user := interactionUser(i)
	if user == nil { // no user on the interaction — nothing sane to do
		return "Nu am putut identifica contul tău Discord. Încearcă din nou."
	}

	// An unspent code comes back instead of a new one. Otherwise running the
	// command twice would either burn the daily quota for nothing or leave a
	// trail of dead codes, and the member would not know which one still works.
	switch existing, err := b.repo.OutstandingInvite(ctx, user.ID); {
	case err == nil:
		return "Ai deja un cod nefolosit:\n" + formatCode(existing.Code, existing.ExpiresAt)
	case errors.Is(err, repo.ErrNotFound):
		// nothing outstanding — carry on and mint
	default:
		slog.Error("look up outstanding invite", "discordUser", user.ID, "err", err)
		return "Ceva n-a mers. Încearcă din nou peste câteva minute."
	}

	last, err := b.repo.LastInviteAt(ctx, user.ID)
	switch {
	case err == nil:
		if wait := b.cfg.QuotaWindow - time.Since(last); wait > 0 {
			return fmt.Sprintf("Ai generat deja o invitație. Mai poți cere una peste %s.", humanWait(wait))
		}
	case errors.Is(err, repo.ErrNotFound):
		// first invite ever for this member
	default:
		slog.Error("look up invite quota", "discordUser", user.ID, "err", err)
		return "Ceva n-a mers. Încearcă din nou peste câteva minute."
	}

	code, err := repo.NewInviteCode()
	if err != nil {
		slog.Error("generate invite code", "err", err)
		return "Ceva n-a mers. Încearcă din nou peste câteva minute."
	}
	var expires *time.Time
	if b.cfg.InviteTTL > 0 {
		t := time.Now().Add(b.cfg.InviteTTL)
		expires = &t
	}

	inv, err := b.repo.CreateInvite(ctx, repo.CreateInviteInput{
		Code:            code,
		DiscordUserID:   user.ID,
		DiscordUsername: user.Username,
		ExpiresAt:       expires,
	})
	if err != nil {
		slog.Error("create invite", "discordUser", user.ID, "err", err)
		return "Ceva n-a mers. Încearcă din nou peste câteva minute."
	}
	slog.Info("invite issued", "discordUser", user.ID, "code", inv.Code)

	return "Poftim invitația ta:\n" + formatCode(inv.Code, inv.ExpiresAt)
}

// interactionUser reads the member off a guild interaction, falling back to
// the DM shape — Member is nil in DMs, User is nil in guilds.
func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil {
		return i.Member.User
	}
	return i.User
}

func formatCode(code string, expires *time.Time) string {
	var sb strings.Builder
	sb.WriteString("**`" + code + "`**\n")
	sb.WriteString("Folosește-l o singură dată la înregistrare pe Anime-Kage.")
	if expires != nil {
		// Discord renders <t:unix:R> as a live "in 7 days" in the reader's
		// own locale and timezone — better than us guessing either.
		sb.WriteString(fmt.Sprintf(" Expiră <t:%d:R>.", expires.Unix()))
	}
	return sb.String()
}

// everyPhrase renders a quota window as a frequency: "o dată pe zi".
//
// humanWait alone is not enough here. It renders a single unit with its article
// ("o zi", "o oră"), which is right for "expiră după o zi" and wrong for the
// frequency the board used to print: "la fiecare o zi". Romanian says that with
// "pe", not "la fiecare", and only for a bare unit.
func everyPhrase(d time.Duration) string {
	switch d {
	case 24 * time.Hour:
		return "o dată pe zi"
	case time.Hour:
		return "o dată pe oră"
	default:
		return "o dată la " + humanWait(d)
	}
}

// humanWait renders a Romanian "2 ore și 15 minute" for the quota message.
func humanWait(d time.Duration) string {
	if d < time.Minute {
		return "mai puțin de un minut"
	}
	// Days, because the invite TTL is 7d and without this the board would
	// advertise "168 de ore". Quota waits never reach here.
	if d >= 24*time.Hour {
		days, rem := int(d.Hours())/24, int(d.Hours())%24
		out := plural(days, "zi", "zile")
		if rem > 0 {
			out += " și " + plural(rem, "oră", "ore")
		}
		return out
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h == 0:
		return plural(m, "minut", "minute")
	case m == 0:
		return plural(h, "oră", "ore")
	default:
		return plural(h, "oră", "ore") + " și " + plural(m, "minut", "minute")
	}
}

// Romanian uses the plural form for 2–19 and again from 20 with "de";
// quota waits never exceed a day, so the "de" case only matters for minutes.
func plural(n int, one, many string) string {
	switch {
	case n == 1:
		return "o " + one
	case n >= 20:
		return fmt.Sprintf("%d de %s", n, many)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}
