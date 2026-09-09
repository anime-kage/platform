package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

const (
	collectionCommand = "colectie"
	collectionOptUser = "utilizator"
	colPrefix         = "col:"
)

// onCollection opens the card browser, and also handles its own buttons: the
// page is encoded in the custom id, so the browser needs no server-side session
// and survives a bot restart mid-flick.
//
// One card per page rather than a list. The portrait is the thing worth looking
// at, and a ten-per-page table of names would be a spreadsheet of something
// people collect for the pictures.
func (b *bot) onCollection(s *discordgo.Session, i *discordgo.InteractionCreate,
	targetID int, targetName string, offset int, filter string, edit, ephemeral bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only gate the command itself: once the browser is open its buttons must
	// keep working, and they arrive as component interactions in that channel.
	if !edit && !ephemeral && !b.inChannel(s, i, b.cfg.GachaChannelID) {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	// The viewer is whoever is driving the browser; the target is whose cards
	// are shown. They differ when someone looks up another player.
	if targetID == 0 {
		me, myName, err := b.repo.UserByDiscordID(ctx, du.ID)
		if errors.Is(err, repo.ErrNotFound) {
			reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage.")
			return
		}
		if err != nil {
			slog.Error("collection: resolve user", "err", err)
			return
		}
		targetID, targetName = me, myName
	}
	userID, username := targetID, targetName

	minF, maxF := 0, 0
	if r := games.RarityByCode(filter); r != nil {
		minF = r.MinFavs
		for idx := range games.Rarities {
			if games.Rarities[idx].Code == r.Code && idx > 0 {
				maxF = games.Rarities[idx-1].MinFavs
			}
		}
	}

	cards, total, err := b.repo.Collection(ctx, userID, minF, maxF, 1, offset)
	if err != nil {
		slog.Error("collection: fetch", "user", userID, "err", err)
		return
	}
	if total == 0 {
		msg := "Nu ai nicio carte încă. Trage una cu **/daily**."
		if filter != "" {
			msg = "Nu ai nicio carte de raritatea asta."
		}
		if du.ID != "" && targetName != "" && !b.isSelf(ctx, du.ID, targetID) {
			msg = fmt.Sprintf("**%s** nu are nicio carte încă.", targetName)
			if filter != "" {
				msg = fmt.Sprintf("**%s** nu are nicio carte de raritatea asta.", targetName)
			}
		}
		respondOrEdit(s, i, edit, ephemeral, &discordgo.InteractionResponseData{
			Content: msg, Flags: discordgo.MessageFlagsEphemeral})
		return
	}
	if offset >= total {
		offset = total - 1
		cards, _, _ = b.repo.Collection(ctx, userID, minF, maxF, 1, offset)
	}
	if len(cards) == 0 {
		return
	}

	c := cards[0]
	rarity := games.RarityFor(c.Favorites)
	series := "necunoscută"
	if c.Series != nil && *c.Series != "" {
		series = *c.Series
	}
	title := c.Name
	if c.Copies > 1 {
		title = fmt.Sprintf("%s  ×%d", c.Name, c.Copies)
	}
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: "Colecția lui " + username},
		Title:       title,
		Description: rarity.StarBar() + "  **" + rarity.Name + "**",
		Color:       rarity.Colour,
		Image:       &discordgo.MessageEmbedImage{URL: c.ImageURL},
		Fields: []*discordgo.MessageEmbedField{{Name: "Serie", Value: series}},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d din %d%s", offset+1, total, filterLabel(filter)),
		},
	}

	prev := offset - 1
	if prev < 0 {
		prev = total - 1 // wrap, so flicking never dead-ends on the first card
	}
	next := (offset + 1) % total

	opts := []discordgo.SelectMenuOption{{Label: "Toate rarităţile", Value: "all", Default: filter == ""}}
	for _, r := range games.Rarities {
		opts = append(opts, discordgo.SelectMenuOption{
			Label: r.Name, Value: r.Code, Description: r.StarBar(), Default: filter == r.Code,
		})
	}

	respondOrEdit(s, i, edit, ephemeral, &discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				// The direction is part of the id, not just the target offset.
				// With a single card prev and next both resolve to 0, the two
				// ids collided, and Discord rejected the whole message with
				// COMPONENT_CUSTOM_ID_DUPLICATED, so the browser was broken for
				// exactly the people who had drawn their first card.
				discordgo.Button{Label: "◀", Style: discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("%sb:%s:%d:%d:%s", colPrefix, du.ID, targetID, prev, filter),
					Disabled: total < 2},
				discordgo.Button{Label: "▶", Style: discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("%sn:%s:%d:%d:%s", colPrefix, du.ID, targetID, next, filter),
					Disabled: total < 2},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID: fmt.Sprintf("%sf:%s:%d", colPrefix, du.ID, targetID),
					Placeholder: "Filtrează după raritate", Options: opts,
				},
			}},
		},
	})
}

// onCollectionComponent routes the browser's own buttons and select menu.
func (b *bot) onCollectionComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	id := strings.TrimPrefix(data.CustomID, colPrefix)
	du := interactionUser(i)
	if du == nil {
		return
	}

	// f:<viewer>:<target>
	if strings.HasPrefix(id, "f:") {
		parts := strings.SplitN(strings.TrimPrefix(id, "f:"), ":", 2)
		if len(parts) < 2 || !ownsBrowser(parts[0], du.ID) {
			b.onCollection(s, i, 0, "", 0, "", false, true) // their own, privately
			return
		}
		target, err := strconv.Atoi(parts[1])
		if err != nil {
			return
		}
		filter := ""
		if len(data.Values) > 0 && data.Values[0] != "all" {
			filter = data.Values[0]
		}
		b.onCollection(s, i, target, b.nameOf(target), 0, filter, true, false)
		return
	}

	// b:<viewer>:<target>:<offset>:<filter>
	id = strings.TrimPrefix(strings.TrimPrefix(id, "b:"), "n:")
	parts := strings.SplitN(id, ":", 4)
	if len(parts) < 3 {
		return
	}
	if !ownsBrowser(parts[0], du.ID) {
		b.onCollection(s, i, 0, "", 0, "", false, true)
		return
	}
	target, terr := strconv.Atoi(parts[1])
	off, err := strconv.Atoi(parts[2])
	if terr != nil || err != nil {
		return
	}
	filter := ""
	if len(parts) > 3 {
		filter = parts[3]
	}
	b.onCollection(s, i, target, b.nameOf(target), off, filter, true, false)
}

func filterLabel(filter string) string {
	if r := games.RarityByCode(filter); r != nil {
		return " · " + r.Name
	}
	return ""
}

// respondOrEdit sends a new message for a command, or replaces the existing one
// when a button was pressed -- so paging does not leave a trail of embeds.
func respondOrEdit(s *discordgo.Session, i *discordgo.InteractionCreate, edit, ephemeral bool, data *discordgo.InteractionResponseData) {
	kind := discordgo.InteractionResponseChannelMessageWithSource
	if edit {
		kind = discordgo.InteractionResponseUpdateMessage
	}
	if ephemeral {
		// A private copy, so a bystander's click neither disturbs the owner's
		// message nor adds anything the channel has to scroll past.
		data.Flags |= discordgo.MessageFlagsEphemeral
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: kind, Data: data}); err != nil {
		slog.Error("collection: respond", "err", err)
	}
}

// ownsBrowser decides whether a click may drive this browser.
//
// The buttons previously re-resolved whoever pressed them, so a bystander
// clicking ▶ on your message got their own cards rendered inside YOUR message.
// Owning it is encoded in the button id rather than read from the interaction's
// parent, so it survives the bot restarting mid-browse.
//
// A refusal alone was worse than it sounds: the reply lands at the bottom of the
// channel, far from the message that was clicked, so it reads as noise and the
// person keeps pressing. The caller instead opens that person their own
// collection, privately, which is what they were reaching for anyway.
func ownsBrowser(owner, presser string) bool {
	return owner == presser
}

// nameOf resolves a site user id to a username for the browser's header.
func (b *bot) nameOf(userID int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	name, err := b.repo.UsernameOf(ctx, userID)
	if err != nil {
		return "jucător"
	}
	return name
}

// isSelf reports whether the Discord account driving the browser is also the
// one whose cards are shown, so the empty state can address them directly.
func (b *bot) isSelf(ctx context.Context, discordID string, targetID int) bool {
	me, _, err := b.repo.UserByDiscordID(ctx, discordID)
	return err == nil && me == targetID
}

// onCollectionCommand resolves the optional user argument before opening the
// browser: no argument means your own cards, a mention means theirs.
func (b *bot) onCollectionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	target, targetName := 0, ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name != collectionOptUser {
			continue
		}
		id, ok := o.Value.(string)
		if !ok {
			continue
		}
		uid, name, err := b.repo.UserByDiscordID(ctx, id)
		if errors.Is(err, repo.ErrNotFound) {
			reply(s, i, "Jucătorul acela nu are contul legat de site.")
			return
		}
		if err != nil {
			slog.Error("collection: resolve target", "err", err)
			return
		}
		target, targetName = uid, name
	}
	b.onCollection(s, i, target, targetName, 0, "", false, false)
}
