package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

const huntCommand = "caut"

// onHunt is the treasure hunt: a rolling four-hour cooldown, gold and XP only.
//
// No cards here on purpose. The gacha is the collection game and the hunt is
// the currency game; letting the hunt drop cards too would make /daily the
// lesser button and collapse two loops into one.
func (b *bot) onHunt(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !b.inChannel(s, i, b.cfg.HuntChannelID) {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	userID, username, err := b.repo.UserByDiscordID(ctx, du.ID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage.")
		return
	}
	if err != nil {
		slog.Error("hunt: resolve user", "discord", du.ID, "err", err)
		reply(s, i, "Ceva n-a mers. Încearcă din nou.")
		return
	}

	when, err := b.repo.ClaimHunt(ctx, userID, games.HuntCooldown)
	if errors.Is(err, repo.ErrExists) && b.cfg.UnlimitedIDs[du.ID] {
		err = nil
	}
	if errors.Is(err, repo.ErrExists) {
		// Not rounded. Rounding to the minute turned 3h59m59s into "4h", so the
		// message said the full cooldown no matter how little was actually left.
		reply(s, i, fmt.Sprintf("Ai căutat de curând. Mai poți căuta peste **%s**.",
			humanDur(time.Until(when))))
		return
	}
	if err != nil {
		slog.Error("hunt: claim", "user", userID, "err", err)
		reply(s, i, "Ceva n-a mers. Încearcă din nou.")
		return
	}

	// Waking something up IS the outcome of the search. Handing out a chest and
	// a monster in the same breath made the find read as a consolation prize
	// stapled to the real event.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	if rng.Intn(100) < games.HuntSpawnChance {
		def := games.RollMonster(rng)
		row, serr := b.spawnMonster(i.ChannelID, def)
		if serr == nil {
			e := b.monsterEmbed(row, def, nil, 0)
			e.Author = &discordgo.MessageEmbedAuthor{Name: username}
			e.Title = "Ai găsit un monstru: " + row.Name
			respondEmbed(s, i, e)
			b.rememberMonsterMessage(s, i, row.ID)
			return
		}
		if errors.Is(serr, repo.ErrExists) {
			// One is already out. Say so rather than quietly handing over loot:
			// silently swapping the outcome made it look like the hunt had found
			// the monster that was already standing there.
			if live, lerr := b.repo.LiveMonster(ctx, i.ChannelID); lerr == nil {
				reply(s, i, fmt.Sprintf(
					"**%s** e încă în viață aici (%d/%d HP). Doboară-l cu /%s înainte să cauți altceva.",
					live.Name, live.HPLeft, live.HPTotal, attackCommand))
				return
			}
		} else {
			slog.Error("hunt: spawn monster", "err", serr)
		}
	}

	find, gold, xp := games.RollFind(rng)
	if gold > 0 || xp > 0 {
		if err := b.repo.AwardXPGold(ctx, userID, xp, gold, "hunt", ""); err != nil {
			slog.Error("hunt: award", "user", userID, "err", err)
		}
	}

	title := "Ai găsit " + find.Name
	desc := "Ai scotocit prin teritoriul neexplorat și nu ai găsit nimic de data asta."
	if gold > 0 || xp > 0 {
		desc = fmt.Sprintf("**+%d gold**\n**+%d XP**", gold, xp)
	} else {
		title = "Nu ai găsit nimic"
	}
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: username},
		Title:       title,
		Description: desc,
		Color:       find.Colour,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Poți căuta din nou peste %s", humanDur(games.HuntCooldown)),
		},
	}
	if find.Art != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: b.cfg.PublicURL + find.Art}
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})

}

// humanDur reads like Romanian rather than Go's "3h0m0s".
// humanDur reads like Romanian rather than Go's "3h0m0s", and keeps seconds
// once the wait is short enough for them to matter.
func humanDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	// Seconds only once they matter. "3h 24min 07s" is false precision on a
	// four hour wait, and "30min 0s" reads like a bug.
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dmin", h, m)
	case m >= 10:
		return fmt.Sprintf("%dmin", m)
	case m > 0:
		return fmt.Sprintf("%dmin %ds", m, sec)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}
