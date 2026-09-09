package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

const (
	guessCommand = "ghicesc"
	guessDesc    = "Spune ce personaj a apărut"
	guessOption  = "nume"
)

// onGuess is an attempt at the live drop.
func (b *bot) onGuess(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
		reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage.")
		return
	}
	if err != nil {
		return
	}

	drop, err := b.repo.LiveDrop(ctx, i.ChannelID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Nu a apărut niciun personaj acum.")
		return
	}
	if err != nil {
		slog.Error("drop: live", "err", err)
		return
	}

	// The cooldown is claimed before the answer is checked, so a wrong guess
	// costs the same as a right one. Otherwise the fastest way to play is to
	// paste surnames until something sticks.
	next, cerr := b.repo.TryGuess(ctx, userID, games.GuessRest)
	if errors.Is(cerr, repo.ErrExists) && !b.cfg.UnlimitedIDs[du.ID] {
		reply(s, i, fmt.Sprintf("Prea repede. Mai încearcă peste **%ds**.",
			int(time.Until(next).Seconds())+1))
		return
	}

	guess := ""
	if opts := i.ApplicationCommandData().Options; len(opts) > 0 {
		guess = opts[0].StringValue()
	}
	if !games.GuessMatches(guess, drop.Name) {
		b.repo.CountGuess(ctx, drop.ID)
		// A near miss is worth saying out loud: it tells someone who knows the
		// character but typed it wrong to try again, without telling someone who
		// guessed at random anything they can use.
		if games.GuessNear(guess, drop.Name) {
			reply(s, i, fmt.Sprintf("**%s** nu e răspunsul, dar ai fost pe aproape. Mai încearcă.", guess))
			return
		}
		reply(s, i, fmt.Sprintf("**%s** nu e răspunsul. Mai încearcă.", guess))
		return
	}

	copies, err := b.repo.ClaimDrop(ctx, drop.ID, userID)
	if errors.Is(err, repo.ErrExists) {
		reply(s, i, "Cineva a fost mai rapid cu o clipă.")
		return
	}
	if err != nil {
		slog.Error("drop: claim", "err", err)
		reply(s, i, "Ceva nu a mers.")
		return
	}

	rarity := games.RarityFor(drop.Favorites)
	series := "necunoscută"
	if drop.Series != nil && *drop.Series != "" {
		series = *drop.Series
	}
	// Winning a drop you already own still pays: the race was the achievement,
	// and handing over a silent duplicate would make winning feel like losing.
	// Melted straight away rather than offered as a choice, because the card is
	// already in the collection and a spare from a drop is unambiguous.
	dup := ""
	if copies > 1 {
		if aerr := b.repo.AwardXPGold(ctx, userID, rarity.DupXP, rarity.DupGold,
			"gacha_duplicate", strconv.Itoa(drop.MalCharID)); aerr != nil {
			slog.Error("drop: duplicate award", "err", aerr)
			dup = fmt.Sprintf("\n\nAveai deja cartea, acum ai %d copii.", copies)
		} else {
			dup = fmt.Sprintf("\n\n**Duplicat**, aveai deja cartea (%d copii). "+
				"Ai primit **%d XP** și **%d gold**.", copies, rarity.DupXP, rarity.DupGold)
		}
	}
	e := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: username + " a ghicit primul"},
		Title:       drop.Name,
		Description: rarity.StarBar() + "  **" + rarity.Name + "**" + dup,
		Color:       rarity.Colour,
		Image:       &discordgo.MessageEmbedImage{URL: drop.ImageURL},
		Fields:      []*discordgo.MessageEmbedField{{Name: "Serie", Value: series}},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d încercări greșite înainte", drop.Guesses)},
	}
	respondEmbed(s, i, e)
}

// runDrops is the drop scheduler.
//
// Deliberately not a cron with fixed times. It wakes often, and each wake asks
// one question: should there be a drop right now? That keeps the rules in one
// place and means a bot restart cannot miss a scheduled moment.
//
// The rule that matters: an unclaimed drop is never replaced. It waits for its
// winner however long that takes, and only the next calendar day retires it.
func (b *bot) runDrops(s *discordgo.Session) {
	if b.cfg.GachaChannelID == "" {
		slog.Info("no gacha channel configured, card drops disabled")
		return
	}
	ticker := time.NewTicker(games.DropTick)
	defer ticker.Stop()
	for {
		b.maybeDrop(s)
		<-ticker.C
	}
}

func (b *bot) maybeDrop(s *discordgo.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Yesterday's leftovers go first: retiring one is what allows today's.
	gone, err := b.repo.ExpireStaleDrops(ctx)
	if err != nil {
		slog.Error("drop: expire", "err", err)
		return
	}
	for _, old := range gone {
		slog.Info("drop expired unclaimed", "char", old.Name, "guesses", old.Guesses)
		b.closeDropMessage(s, old)
	}

	if _, err := b.repo.LiveDrop(ctx, b.cfg.GachaChannelID); err == nil {
		return // one is already waiting to be named
	} else if !errors.Is(err, repo.ErrNotFound) {
		slog.Error("drop: live check", "err", err)
		return
	}

	if !games.WithinDropHours(time.Now()) {
		return
	}
	today, err := b.repo.DropsToday(ctx, b.cfg.GachaChannelID)
	if err != nil || today >= games.DropsPerDay {
		return
	}
	// Spread them out rather than firing the day's quota in the first hour.
	if !games.DropDue(time.Now(), today) {
		return
	}

	drop, err := b.repo.CreateDrop(ctx, b.cfg.GachaChannelID, games.DropRepeatDays)
	if errors.Is(err, repo.ErrExists) || errors.Is(err, repo.ErrNotFound) {
		return
	}
	if err != nil {
		slog.Error("drop: create", "err", err)
		return
	}

	rarity := games.RarityFor(drop.Favorites)
	e := &discordgo.MessageEmbed{
		Title:       "A apărut un personaj",
		Description: "Cine e? Răspunde cu **/" + guessCommand + "**.\nPrimul care ghicește ia cartea.",
		Color:       rarity.Colour,
		Image:       &discordgo.MessageEmbedImage{URL: drop.ImageURL},
		Footer:      &discordgo.MessageEmbedFooter{Text: rarity.Name},
	}
	msg, err := s.ChannelMessageSendEmbed(b.cfg.GachaChannelID, e)
	if err != nil {
		slog.Error("drop: post", "err", err)
		return
	}
	if err := b.repo.SetDropMessage(ctx, drop.ID, msg.ID); err != nil {
		slog.Error("drop: remember message", "err", err)
	}
	slog.Info("card dropped", "char", drop.Name, "rarity", rarity.Code, "today", today+1)
}

// closeDropMessage rewrites an expired drop's card so the channel stops
// advertising a guess nobody can win any more, and says who it was. Revealing
// the answer is the point: an unclaimed drop is the one nobody recognised, and
// leaving it a mystery teaches nothing.
func (b *bot) closeDropMessage(s *discordgo.Session, d repo.Drop) {
	if d.MessageID == nil || *d.MessageID == "" {
		return
	}
	rarity := games.RarityFor(d.Favorites)
	series := "necunoscută"
	if d.Series != nil && *d.Series != "" {
		series = *d.Series
	}
	e := &discordgo.MessageEmbed{
		Title:       "Nimeni nu a ghicit: " + d.Name,
		Description: rarity.StarBar() + "  **" + rarity.Name + "**",
		Color:       0x6B7280,
		Image:       &discordgo.MessageEmbedImage{URL: d.ImageURL},
		Fields:      []*discordgo.MessageEmbedField{{Name: "Serie", Value: series}},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d încercări greșite. Cartea s-a pierdut.", d.Guesses)},
	}
	if _, err := s.ChannelMessageEditEmbed(d.ChannelID, *d.MessageID, e); err != nil {
		slog.Error("drop: close message", "err", err)
	}
}
