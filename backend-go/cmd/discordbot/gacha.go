package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

// Command descriptions live here rather than inline so the style test can see
// them: they are member-facing text, and Discord shows them in the command bar.
const (
	dailyDesc      = "Trage un personaj, o dată pe zi"
	collectionDesc = "Vezi colecția ta de personaje"
	huntDesc       = "Caută comori, o dată la 4 ore"
)

const (
	dailyCommand   = "daily"
	gachaKeepID    = "gacha:keep:"
	gachaConvertID = "gacha:conv:"
	gachaBuyID     = "gacha:buy:"
	gachaWaitID    = "gacha:wait"
)

// onDaily is the daily gacha pull.
//
// The card pool is game_characters -- the same 900-odd portraits the site's
// character round draws from -- so the two games share one import and one
// notion of who is famous. XP and gold land in the same ledger as the arcade:
// the point of linking accounts is that Discord is a second door into one
// economy, not a parallel one.
func (b *bot) onDaily(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !b.inChannel(s, i, b.cfg.GachaChannelID) {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	userID, username, err := b.repo.UserByDiscordID(ctx, du.ID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage. "+
			"Dacă ai cont pe site, spune-i unui admin să-l lege.")
		return
	}
	if err != nil {
		slog.Error("gacha: resolve user", "discord", du.ID, "err", err)
		reply(s, i, "Ceva n-a mers. Încearcă din nou.")
		return
	}

	// The streak is still recorded for later (a pity timer, or a bonus at N
	// days), it is simply not shown while it buys nothing.
	_, err = b.repo.ClaimDailyDraw(ctx, userID)
	if errors.Is(err, repo.ErrExists) && b.cfg.UnlimitedIDs[du.ID] {
		err = nil // testing account: draw anyway
	}
	if errors.Is(err, repo.ErrExists) {
		b.offerExtraDraw(s, i, userID)
		return
	}
	if err != nil {
		slog.Error("gacha: claim daily", "user", userID, "err", err)
		reply(s, i, "Ceva n-a mers. Încearcă din nou.")
		return
	}

	b.drawAndShow(s, i, userID, username, 0)
}

// drawAndShow rolls a card, files it, and renders it. Shared by the free daily
// draw and the paid ones so the two can never drift apart in what they award or
// how a duplicate is offered; paidFor is the gold spent, 0 for the free one.
func (b *bot) drawAndShow(s *discordgo.Session, i *discordgo.InteractionCreate,
	userID int, username string, paidFor int) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rarity := games.RollRarity(rand.New(rand.NewSource(time.Now().UnixNano())))
	// The band runs from this tier's floor up to the next tier's floor. Rarities
	// is ordered rarest first, so the ceiling is the previous entry's minimum,
	// and the top tier has none.
	ceiling := 0
	for idx := range games.Rarities {
		if games.Rarities[idx].Code == rarity.Code && idx > 0 {
			ceiling = games.Rarities[idx-1].MinFavs
		}
	}
	card, err := b.repo.RandomCharacterInRange(ctx, rarity.MinFavs, ceiling)
	if err != nil {
		slog.Error("gacha: draw", "rarity", rarity.Code, "err", err)
		reply(s, i, "Nu am găsit niciun personaj de tras. Spune-i unui admin.")
		return
	}

	copies, err := b.repo.AddCard(ctx, userID, card.MalCharID)
	if err != nil {
		slog.Error("gacha: add card", "user", userID, "char", card.MalCharID, "err", err)
		reply(s, i, "Ceva n-a mers. Încearcă din nou.")
		return
	}
	owned, total, _ := b.repo.CollectionSize(ctx, userID)

	series := "necunoscută"
	if card.Series != nil && *card.Series != "" {
		series = *card.Series
	}
	// Image, not Thumbnail: a thumbnail is a 80px square in the corner, and the
	// portrait IS the card -- it should be the biggest thing in the message.
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: username + " a tras o carte"},
		Title:       card.Name,
		Description: rarity.StarBar() + "  **" + rarity.Name + "**",
		Color:       rarity.Colour,
		Image:       &discordgo.MessageEmbedImage{URL: card.ImageURL},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Serie", Value: series, Inline: false},
			{Name: "Colecție", Value: fmt.Sprintf("%d/%d", owned, total), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d favorite pe MAL/AniList", card.Favorites),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	var components []discordgo.MessageComponent
	if copies > 1 {
		// A duplicate is a choice, not a consolation: keep the spare for trading,
		// or melt it for XP now. Offering it as buttons rather than deciding for
		// them is the whole point -- a spare Legendar is worth trading, a spare
		// Comun almost never is.
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Duplicat",
			Value: fmt.Sprintf("Ai deja această carte, %d copii în total.", copies),
		})
		id := strconv.Itoa(card.MalCharID)
		components = []discordgo.MessageComponent{discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    fmt.Sprintf("Transformă în %d XP", rarity.DupXP),
					Style:    discordgo.PrimaryButton,
					CustomID: gachaConvertID + id,
				},
				discordgo.Button{
					Label:    "Păstrează pentru schimb",
					Style:    discordgo.SecondaryButton,
					CustomID: gachaKeepID + id,
				},
			},
		}}
	}

	if paidFor > 0 {
		embed.Footer.Text = fmt.Sprintf("%s · plătit %d gold", embed.Footer.Text, paidFor)
	}
	kind := discordgo.InteractionResponseChannelMessageWithSource
	if i.Type == discordgo.InteractionMessageComponent {
		// The paid draw answers the offer message, replacing the buttons so the
		// price cannot be clicked twice.
		kind = discordgo.InteractionResponseUpdateMessage
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: kind,
		Data: &discordgo.InteractionResponseData{
			Content:    "",
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	}); err != nil {
		slog.Error("gacha: respond", "err", err)
	}
}

// onGachaButton resolves the duplicate choice.
func (b *bot) onGachaButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := i.MessageComponentData().CustomID
	switch {
	case data == gachaWaitID:
		b.onWaitDraw(s, i)
		return
	case strings.HasPrefix(data, gachaBuyID):
		b.onBuyDraw(s, i)
		return
	}
	keep := strings.HasPrefix(data, gachaKeepID)
	raw := strings.TrimPrefix(strings.TrimPrefix(data, gachaKeepID), gachaConvertID)
	charID, err := strconv.Atoi(raw)
	if err != nil {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	userID, _, err := b.repo.UserByDiscordID(ctx, du.ID)
	if err != nil {
		return
	}

	if keep {
		editReply(s, i, "Ai păstrat duplicatul pentru schimb.")
		return
	}

	// Drop the spare first. If the award then fails the player has lost a copy
	// for nothing, so the order is deliberate the other way round: award, then
	// drop only if that succeeded.
	favs, err := b.repo.CharacterFavourites(ctx, charID)
	if err != nil {
		return
	}
	rarity := games.RarityFor(favs)
	if err := b.repo.AwardXPGold(ctx, userID, rarity.DupXP, rarity.DupGold,
		"gacha_duplicate", strconv.Itoa(charID)); err != nil {
		slog.Error("gacha: award duplicate", "user", userID, "err", err)
		editReply(s, i, "Nu am putut acorda XP-ul. Duplicatul e încă la tine.")
		return
	}
	if err := b.repo.DropCard(ctx, userID, charID); err != nil {
		slog.Error("gacha: drop duplicate", "user", userID, "err", err)
	}
	editReply(s, i, fmt.Sprintf("Ai transformat duplicatul în **%d XP** și **%d gold**.",
		rarity.DupXP, rarity.DupGold))
}

func reply(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func editReply(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: msg, Components: []discordgo.MessageComponent{}},
	})
}


// inChannel keeps a game in its own room.
//
// An empty id means "anywhere", so a server that has not set the channels keeps
// working. The refusal is ephemeral: telling the whole channel that someone
// typed in the wrong place is noisier than the mistake.
func (b *bot) inChannel(s *discordgo.Session, i *discordgo.InteractionCreate, want string) bool {
	if want == "" || i.ChannelID == want {
		return true
	}
	reply(s, i, "Comanda asta merge doar în <#"+want+">.")
	return false
}

// offerExtraDraw is what the free draw's refusal turned into: a price and a
// choice, rather than a door.
func (b *bot) offerExtraDraw(s *discordgo.Session, i *discordgo.InteractionCreate, userID int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bought, err := b.repo.ExtraDrawsToday(ctx, userID)
	if err != nil {
		slog.Error("gacha: extra count", "err", err)
		return
	}
	if bought >= games.ExtraDrawMax {
		reply(s, i, "Ai tras tot ce se putea azi. Revino mâine, ziua se schimbă la miezul nopții UTC (03:00 la noi).")
		return
	}
	cost := games.ExtraDrawCost(bought)
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
			Content: fmt.Sprintf(
				"Ai folosit tragerea gratuită de azi. Mai poți trage cu gold, "+
					"dar prețul se dublează de fiecare dată (%d trageri plătite pe zi).", games.ExtraDrawMax),
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    fmt.Sprintf("Trage încă o carte (%d gold)", cost),
						Style:    discordgo.SuccessButton,
						CustomID: fmt.Sprintf("%s%d", gachaBuyID, cost),
					},
					discordgo.Button{Label: "Așteaptă până mâine", Style: discordgo.SecondaryButton,
						CustomID: gachaWaitID},
				},
			}},
		},
	})
}

// onBuyDraw spends the gold and draws, or explains why it cannot.
func (b *bot) onBuyDraw(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	du := interactionUser(i)
	if du == nil {
		return
	}
	userID, username, err := b.repo.UserByDiscordID(ctx, du.ID)
	if err != nil {
		return
	}
	// The price is recomputed here rather than trusted from the button: the id
	// is client supplied, and an old message left open would otherwise buy at
	// yesterday's cheaper rate.
	bought, err := b.repo.ExtraDrawsToday(ctx, userID)
	if err != nil {
		return
	}
	if bought >= games.ExtraDrawMax {
		editReply(s, i, "Ai tras tot ce se putea azi.")
		return
	}
	cost := games.ExtraDrawCost(bought)

	if err := b.repo.SpendGold(ctx, userID, cost, "gacha_extra", ""); errors.Is(err, repo.ErrExists) {
		editReply(s, i, fmt.Sprintf("Nu ai destul gold. Îți trebuie **%d**.", cost))
		return
	} else if err != nil {
		slog.Error("gacha: spend", "err", err)
		editReply(s, i, "Ceva nu a mers.")
		return
	}
	if err := b.repo.BuyExtraDraw(ctx, userID, games.ExtraDrawMax); err != nil {
		slog.Error("gacha: record extra", "err", err)
	}
	b.drawAndShow(s, i, userID, username, cost)
}

func (b *bot) onWaitDraw(s *discordgo.Session, i *discordgo.InteractionCreate) {
	editReply(s, i, "Bine. Revino mâine pentru tragerea gratuită.")
}

// inEitherChannel allows a command in any of the game rooms.
//
// /profil belongs in both: it is the card collection and the arcade standing in
// one card, and a player checking their level after a fight should not have to
// walk to the other channel to do it.
func (b *bot) inEitherChannel(s *discordgo.Session, i *discordgo.InteractionCreate, want ...string) bool {
	named := []string{}
	for _, w := range want {
		if w == "" {
			continue // unconfigured means unrestricted
		}
		if i.ChannelID == w {
			return true
		}
		named = append(named, "<#"+w+">")
	}
	if len(named) == 0 {
		return true
	}
	reply(s, i, "Comanda asta merge doar în "+strings.Join(named, " sau ")+".")
	return false
}
