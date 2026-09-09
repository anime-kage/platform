package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"animekage/backend/internal/games"
	"animekage/backend/internal/repo"
)

const (
	attackCommand = "atac"
	attackDesc    = "Lovește monstrul din canal"
	spawnCommand  = "spawn"
	spawnDesc     = "Cheamă un monstru în canal (doar admini)"
)

// spawnMonster puts one in the channel and posts its health bar.
func (b *bot) spawnMonster(channelID string, m *games.Monster) (*repo.Monster, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return b.repo.SpawnMonster(ctx, m.Code, m.Name, channelID,
		m.HP, m.XP, m.Gold, time.Duration(m.Minutes)*time.Minute)
}

// monsterEmbed draws the fight: a health bar, who has hit it, and what it pays.
func (b *bot) monsterEmbed(row *repo.Monster, def *games.Monster, hitters []repo.Hitter, totalDmg int) *discordgo.MessageEmbed {
	colour := 0xC0392B
	art := ""
	if def != nil {
		colour = def.Colour
		art = def.ArtURL(b.cfg.PublicURL)
	}
	// The block bar is gone: it rendered as a run of shaded characters whose
	// width depends on the reader's font, and it competed with the artwork for
	// the same job. The numbers say it exactly and the picture carries the rest.
	pct := row.HPLeft * 100 / row.HPTotal
	desc := fmt.Sprintf("**%d / %d** viață · %d%%", row.HPLeft, row.HPTotal, pct)
	if row.HPLeft == 0 {
		desc = "**Învins!**"
	}

	e := &discordgo.MessageEmbed{
		Title:       row.Name,
		Description: desc,
		Color:       colour,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Recompensă", Value: fmt.Sprintf("%d XP și %d gold, împărțite după cât ai lovit",
				row.XPPot, row.GoldPot)},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Scrie /atac ca să lovești"},
	}
	if art != "" {
		e.Image = &discordgo.MessageEmbedImage{URL: art}
	}
	if len(hitters) > 0 {
		lines := []string{}
		for idx, h := range hitters {
			if idx == 5 {
				lines = append(lines, fmt.Sprintf("...și încă %d", len(hitters)-5))
				break
			}
			share := 0
			if totalDmg > 0 {
				share = h.Damage * 100 / totalDmg
			}
			lines = append(lines, fmt.Sprintf("**%s** %d dmg (%d%%)", h.Username, h.Damage, share))
		}
		e.Fields = append(e.Fields, &discordgo.MessageEmbedField{
			Name: fmt.Sprintf("Luptători (%d)", len(hitters)), Value: strings.Join(lines, "\n")})
	}
	return e
}

// onAttack is one blow against the channel's monster.
func (b *bot) onAttack(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
		return
	}

	row, err := b.repo.LiveMonster(ctx, i.ChannelID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Nu e niciun monstru aici acum. Caută unul cu /caut.")
		return
	}
	if err != nil {
		slog.Error("monster: live", "err", err)
		return
	}

	// The rest comes before the blow: claiming it is what stops two rapid /atac
	// commands both landing, so it has to happen even when the monster turns out
	// to be dead a moment later.
	next, cerr := b.repo.TryAttack(ctx, userID, games.AttackRest)
	switch {
	case errors.Is(cerr, repo.ErrExists) && b.cfg.UnlimitedIDs[du.ID]:
		// testing account: swing anyway
	case errors.Is(cerr, repo.ErrExists):
		reply(s, i, fmt.Sprintf("Îți tragi sufletul. Mai poți ataca peste **%s**.",
			humanDur(time.Until(next))))
		return
	case cerr != nil:
		slog.Error("monster: cooldown", "err", cerr)
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// The monster gets its own hit in occasionally. No damage dealt, and a much
	// longer rest, so a fight can genuinely go badly for someone.
	if rng.Intn(100) < games.CounterChance {
		next, perr := b.repo.PenaliseAttacker(ctx, userID, games.CounterRest)
		rest := games.CounterRest
		if perr == nil {
			rest = time.Until(next)
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf(
					"**%s** a fost lovit de %s! Nu ai apucat să dai niciun damage "+
						"și te retragi pentru **%s**.", username, row.Name, humanDur(rest)),
			},
		})
		return
	}

	dmg := games.AttackDamage(rng)
	left, killed, err := b.repo.HitMonster(ctx, row.ID, userID, dmg)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Monstrul a fost deja învins.")
		return
	}
	if err != nil {
		slog.Error("monster: hit", "err", err)
		return
	}
	row.HPLeft = left

	hitters, totalDmg, _ := b.repo.Hitters(ctx, row.ID)
	def := games.MonsterByCode(row.Code)

	if killed {
		// Pay everyone who took part, by share of the damage they did, and give
		// each of them their own line. A single table would make the person who
		// landed two hits read the same as the one who carried the fight.
		var lines []string
		for _, h := range hitters {
			xp := games.SplitReward(row.XPPot, h.Damage, totalDmg)
			gold := games.SplitReward(row.GoldPot, h.Damage, totalDmg)
			if err := b.repo.AwardXPGold(ctx, h.UserID, xp, gold, "monster", row.Code); err != nil {
				slog.Error("monster: award", "user", h.UserID, "err", err)
				continue
			}
			share := 0
			if totalDmg > 0 {
				share = h.Damage * 100 / totalDmg
			}
			lines = append(lines, fmt.Sprintf(
				"Felicitări, **%s**! Ai reușit să îi dai **%d** damage (%d%% din total) "+
					"și ai primit **%d XP** și **%d gold**.",
				h.Username, h.Damage, share, xp, gold))
		}
		e := b.monsterEmbed(row, def, hitters, totalDmg)
		e.Title = "Ați învins " + row.Name
		e.Color = 0x2ECC71
		e.Footer = &discordgo.MessageEmbedFooter{Text: "Lovitura de graţie: " + username}
		// Two messages, in this order: the kill is the event, and the payouts
		// are the consequence. Putting the reward lines above the card made the
		// announcement read as a footnote to a list of numbers.
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{e}},
		})
		if len(lines) > 0 {
			if _, ferr := s.FollowupMessageCreate(i.Interaction, false,
				&discordgo.WebhookParams{Content: strings.Join(lines, "\n\n")}); ferr != nil {
				slog.Error("monster: payout message", "err", ferr)
			}
		}
		return
	}

	// A plain line while the fight is on. Redrawing the whole card with the
	// health bar and the full roster after every blow buries the channel in
	// near identical embeds; the detail belongs at the spawn and at the kill.
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s** a lovit monstrul cu **%d** damage. Mai are **%d** HP.",
				username, dmg, left),
		},
	})
}

// onSpawn is the admin test hook. A slash command restricted to administrators
// rather than a "$spawn" message: reading message text needs the MESSAGE_CONTENT
// privileged intent, which this bot deliberately does not hold, so the text
// version could never have matched anything.
func (b *bot) onSpawn(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.inChannel(s, i, b.cfg.HuntChannelID) {
		return
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	def := games.RollMonster(rng)
	if opts := i.ApplicationCommandData().Options; len(opts) > 0 {
		if picked := games.MonsterByCode(opts[0].StringValue()); picked != nil {
			def = picked
		}
	}
	row, err := b.spawnMonster(i.ChannelID, def)
	if errors.Is(err, repo.ErrExists) {
		reply(s, i, "Deja e un monstru aici. Învinge-l întâi.")
		return
	}
	if err != nil {
		slog.Error("monster: spawn", "err", err)
		reply(s, i, "Nu am putut chema monstrul.")
		return
	}
	e := b.monsterEmbed(row, def, nil, 0)
	e.Title = "A apărut " + row.Name
	respondEmbed(s, i, e)
	b.rememberMonsterMessage(s, i, row.ID)
}

// rememberMonsterMessage stores which message carries the health bar.
//
// It has to be read back from the interaction, because responding to one does
// not hand you the message it created. Without this the id stayed NULL and an
// escaped monster could never have its card rewritten -- the row knew it was
// over and the channel never found out.
func (b *bot) rememberMonsterMessage(s *discordgo.Session, i *discordgo.InteractionCreate, monsterID int) {
	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil || msg == nil {
		slog.Warn("monster: could not read back the response message", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.repo.SetMonsterMessage(ctx, monsterID, msg.ID); err != nil {
		slog.Error("monster: remember message", "err", err)
	}
}

// spawnChoices lets the admin pick which one to summon.
func spawnChoices() []*discordgo.ApplicationCommandOptionChoice {
	out := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(games.Monsters))
	for _, m := range games.Monsters {
		out = append(out, &discordgo.ApplicationCommandOptionChoice{Name: m.Name, Value: m.Code})
	}
	return out
}

func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, e *discordgo.MessageEmbed) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{e}},
	})
}

// runMonsterExpiry retires fights the channel never finished.
//
// Without this the unique index works against us: it only frees a channel when
// killed_at is set, so one monster nobody bothered to kill blocks every future
// spawn forever. The expires_at column existed from the start and nothing ever
// read it, which is exactly the shape of bug that hides until someone wonders
// why nothing has spawned for a day.
func (b *bot) runMonsterExpiry(s *discordgo.Session) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		gone, err := b.repo.ExpireMonsters(ctx)
		if err != nil {
			slog.Error("monster: expire", "err", err)
		}
		for _, m := range gone {
			slog.Info("monster escaped", "name", m.Name, "hp_left", m.HPLeft, "of", m.HPTotal)
			b.settleEscape(ctx, s, m)
		}
		cancel()
		<-ticker.C
	}
}

// settleEscape pays the people who fought a monster that got away, and rewrites
// its card so the channel is not left with a health bar for something gone.
func (b *bot) settleEscape(ctx context.Context, s *discordgo.Session, m repo.Monster) {
	hitters, totalDmg, err := b.repo.Hitters(ctx, m.ID)
	if err != nil {
		slog.Error("monster: escape hitters", "err", err)
		return
	}
	lines := []string{}
	for _, h := range hitters {
		xp := games.EscapeReward(m.XPPot, h.Damage, m.HPTotal)
		gold := games.EscapeReward(m.GoldPot, h.Damage, m.HPTotal)
		if xp == 0 && gold == 0 {
			continue
		}
		if err := b.repo.AwardXPGold(ctx, h.UserID, xp, gold, "monster_escape", m.Code); err != nil {
			slog.Error("monster: escape award", "user", h.UserID, "err", err)
			continue
		}
		lines = append(lines, fmt.Sprintf("**%s** a dat %d damage și a primit **%d XP** și **%d gold**.",
			h.Username, h.Damage, xp, gold))
	}

	def := games.MonsterByCode(m.Code)
	pct := 0
	if m.HPTotal > 0 {
		pct = (m.HPTotal - m.HPLeft) * 100 / m.HPTotal
	}

	// The result goes out as a NEW message, not as an edit to the spawn card.
	//
	// A Discord edit is silent: no notification, no reordering. The first Slime
	// to escape was posted at 07:17 and closed at 00:01, and the result landed
	// on a card 24 messages up the channel that nobody had reason to scroll back
	// to. Everyone was paid and nobody could tell. A kill already announces
	// itself with a fresh message; an escape has to do the same.
	e := b.monsterEmbed(&m, def, hitters, totalDmg)
	e.Title = m.Name + " a scăpat"
	e.Color = 0x6B7280
	e.Description = fmt.Sprintf("A fugit cu **%d / %d** viață rămasă. I-ați luat %d%%.",
		m.HPLeft, m.HPTotal, pct)
	if len(lines) > 0 {
		e.Fields = append(e.Fields, &discordgo.MessageEmbedField{
			Name:  "Consolare",
			Value: strings.Join(lines, "\n"),
		})
		e.Footer = &discordgo.MessageEmbedFooter{Text: "Jumătate din recompensă, pentru cei care au lovit"}
	} else {
		e.Footer = &discordgo.MessageEmbedFooter{Text: "Nimeni nu l-a lovit, deci nu e nimic de împărțit"}
	}
	if _, err := s.ChannelMessageSendEmbed(m.ChannelID, e); err != nil {
		slog.Error("monster: escape announcement", "err", err)
	}

	// Delete the spawn card rather than closing it in place.
	//
	// Editing it left two "a scăpat" cards in the channel saying the same thing,
	// one of them buried. The result message above is the whole story, so the
	// card that advertised a fight nobody can join any more just goes.
	if m.MessageID == nil || *m.MessageID == "" {
		return
	}
	if err := s.ChannelMessageDelete(m.ChannelID, *m.MessageID); err != nil {
		slog.Error("monster: remove escape card", "err", err)
	}
}
