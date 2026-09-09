package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

const (
	profileCommand = "profil"
	profileDesc    = "Vezi statisticile tale de pe site și de pe server"
)

// onProfile is the one place the two halves of the platform are shown together:
// level and faction come from the site, the card count from Discord, and the
// avatar from whichever Discord account is asking.
func (b *bot) onProfile(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !b.inEitherChannel(s, i, b.cfg.GachaChannelID, b.cfg.HuntChannelID) {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	userID, _, err := b.repo.UserByDiscordID(ctx, du.ID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage. "+
			"Dacă ai cont pe site, spune-i unui admin să-l lege.")
		return
	}
	if err != nil {
		slog.Error("profile: resolve user", "err", err)
		return
	}

	p, err := b.repo.DiscordProfile(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Nu ai încă un profil de joc. Intră o dată în Arcade pe site și revino.")
		return
	}
	if err != nil {
		slog.Error("profile: fetch", "user", userID, "err", err)
		reply(s, i, "Ceva nu a mers. Încearcă din nou.")
		return
	}

	level := games.LevelForXP(p.Xp)
	faction, rank, colour := "fără facțiune", "", 0x5865F2
	if p.Faction != nil && *p.Faction != "" {
		if f := games.FactionByCode(*p.Faction); f != nil {
			faction = f.Name
		}
		rank = games.RankFor(*p.Faction, level)
		colour = 0xE0A51C
	}

	// One line per badge with its own name, newest first: a comma separated run
	// of names reads as a sentence rather than a list of things earned.
	badges := "niciuna încă"
	if len(p.Badges) > 0 {
		lines := make([]string, 0, len(p.Badges))
		for idx := len(p.Badges) - 1; idx >= 0; idx-- {
			lines = append(lines, "• "+badgeName(p.Badges[idx]))
		}
		badges = strings.Join(lines, "\n")
	}

	subtitle := faction
	if rank != "" {
		subtitle = faction + " · " + rank
	}
	// The avatar identifies the person, so it goes in the author line where a
	// face belongs. That frees the thumbnail for the faction crest and the main
	// image for the newest badge, which is the art actually worth looking at.
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: p.Username, IconURL: du.AvatarURL("128")},
		Title:       fmt.Sprintf("Nivel %d", level),
		Description: subtitle,
		Color:       colour,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "XP", Value: fmt.Sprintf("`%d`", p.Xp), Inline: true},
			{Name: "Gold", Value: fmt.Sprintf("`%d`", p.Gold), Inline: true},
			{Name: "Clasament", Value: fmt.Sprintf("`%d din %d`", p.Rank, p.Players), Inline: true},
			{Name: "Colecție", Value: fmt.Sprintf("`%d` din `%d` personaje", p.Cards, p.CardTotal), Inline: true},
			{Name: "Progres", Value: collectionBar(p.Cards, p.CardTotal), Inline: true},
			{Name: fmt.Sprintf("Insigne (%d)", len(p.Badges)), Value: badges, Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "anime-kage.ro"},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if p.Faction != nil && *p.Faction != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: b.cfg.PublicURL + "/arcade/factions/" + *p.Faction + ".png"}
	}
	if len(p.Badges) > 0 {
		newest := p.Badges[len(p.Badges)-1]
		// The name is always the newest badge, but the picture is the newest one
		// that has art: pointing the embed at a file that does not exist makes
		// Discord drop the image silently, leaving the profile with none at all.
		for i := len(p.Badges) - 1; i >= 0; i-- {
			if games.BadgesWithArt[p.Badges[i]] {
				embed.Image = &discordgo.MessageEmbedImage{
					URL: b.cfg.PublicURL + "/arcade/icons/" + p.Badges[i] + ".png"}
				break
			}
		}
		embed.Fields[len(embed.Fields)-1].Name = fmt.Sprintf("Insigne (%d) · cea mai nouă: %s",
			len(p.Badges), badgeName(newest))
	}
	if p.PendingXp > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "XP în așteptare",
			Value: fmt.Sprintf("%d, alege o facțiune pe site ca să îl revendici", p.PendingXp),
		})
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// badgeName turns a stored code into the name the site shows, so the two never
// disagree about what a badge is called.
func badgeName(code string) string {
	for _, def := range games.BadgeCatalogue {
		if def.Code == code {
			return def.Name
		}
	}
	return code
}

// collectionBar draws the collection as a bar, because "8/914" is a number a
// reader has to do arithmetic on before it means anything.
func collectionBar(have, total int) string {
	if total <= 0 {
		return "n/a"
	}
	const width = 10
	filled := have * width / total
	if filled == 0 && have > 0 {
		filled = 1 // never show an empty bar to someone who owns cards
	}
	pct := float64(have) * 100 / float64(total)
	return strings.Repeat("▰", filled) + strings.Repeat("▱", width-filled) +
		fmt.Sprintf("  %.1f%%", pct)
}
