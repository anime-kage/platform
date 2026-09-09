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
	tradeCommand  = "schimb"
	tradeDesc     = "Schimbă un duplicat cu alt jucător"
	tradeOptUser  = "utilizator"
	tradeOptGive  = "ofer"
	tradeOptWant  = "cer"
	tradeAcceptID = "trade:ok:"
	tradeDeclineID = "trade:no:"
)

// onTradeAutocomplete suggests cards as the player types.
//
// Only spares are offered on both sides. Typing a character's name exactly is
// miserable, and the list is the difference between a command people use and one
// they give up on.
func (b *bot) onTradeAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	data := i.ApplicationCommandData()
	du := interactionUser(i)
	if du == nil {
		return
	}
	me, _, err := b.repo.UserByDiscordID(ctx, du.ID)
	if err != nil {
		return
	}

	var focused *discordgo.ApplicationCommandInteractionDataOption
	var partner string
	for _, o := range data.Options {
		if o.Focused {
			focused = o
		}
		if o.Name == tradeOptUser {
			partner = o.Value.(string)
		}
	}
	if focused == nil {
		return
	}
	term, _ := focused.Value.(string)

	owner := me
	if focused.Name == tradeOptWant {
		// The other side's spares, so you cannot ask for something they cannot give.
		if partner == "" {
			return
		}
		other, _, oerr := b.repo.UserByDiscordID(ctx, partner)
		if oerr != nil {
			return
		}
		owner = other
	}
	cards, err := b.repo.Spares(ctx, owner, term, 25)
	if err != nil {
		return
	}
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(cards))
	for _, c := range cards {
		r := games.RarityFor(c.Favorites)
		// Stars rather than the tier's name: the list is scanned, not read, and
		// a row of stars sorts itself out at a glance where "Necomun" has to be
		// compared against "Comun" a word at a time.
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  fmt.Sprintf("%s %s ×%d", c.Name, r.StarBar(), c.Copies),
			Value: strconv.Itoa(c.MalCharID),
		})
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
}

// onTrade puts an offer to another player.
func (b *bot) onTrade(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !b.inChannel(s, i, b.cfg.GachaChannelID) {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	me, myName, err := b.repo.UserByDiscordID(ctx, du.ID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Contul tău de Discord nu e legat de un cont Anime-Kage.")
		return
	}
	if err != nil {
		return
	}

	var targetID, giveRaw, wantRaw string
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case tradeOptUser:
			targetID = o.Value.(string)
		case tradeOptGive:
			giveRaw = o.StringValue()
		case tradeOptWant:
			wantRaw = o.StringValue()
		}
	}
	if targetID == du.ID {
		reply(s, i, "Nu poți schimba cu tine însuți.")
		return
	}
	them, theirName, err := b.repo.UserByDiscordID(ctx, targetID)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Jucătorul acela nu are contul legat de site.")
		return
	}
	if err != nil {
		return
	}
	give, err1 := strconv.Atoi(giveRaw)
	want, err2 := strconv.Atoi(wantRaw)
	if err1 != nil || err2 != nil {
		reply(s, i, "Alege cărțile din lista sugerată.")
		return
	}

	mine, err := b.repo.CardDetail(ctx, me, give)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, "Nu ai un duplicat din cartea aia. Poți da doar cărți din care ai cel puțin două.")
		return
	}
	theirs, err := b.repo.CardDetail(ctx, them, want)
	if errors.Is(err, repo.ErrNotFound) {
		reply(s, i, fmt.Sprintf("**%s** nu are un duplicat din cartea aia.", theirName))
		return
	}
	if err != nil {
		return
	}

	rm, rt := games.RarityFor(mine.Favorites), games.RarityFor(theirs.Favorites)
	gold := games.TradeGold(rm, rt)
	fromPays := games.TradeDebtor(rm, rt)

	// Check the money before anyone is pinged. An offer that cannot possibly
	// complete is worse than no offer: it pulls the other person into a decision
	// that was never available, and they find out only after accepting.
	if gold > 0 {
		payerID, payerName := them, theirName
		if fromPays {
			payerID, payerName = me, myName
		}
		have, gerr := b.repo.GoldOf(ctx, payerID)
		if gerr != nil {
			slog.Error("trade: gold check", "err", gerr)
			return
		}
		if have < gold {
			if fromPays {
				reply(s, i, fmt.Sprintf(
					"Schimbul cere **%d gold** de la tine pentru diferența de raritate, "+
						"dar ai doar **%d**.", gold, have))
			} else {
				reply(s, i, fmt.Sprintf(
					"**%s** ar trebui să plătească **%d gold** pentru diferența de raritate, "+
						"dar are doar **%d**. Alege alt schimb ca să nu îl deranjezi degeaba.",
					payerName, gold, have))
			}
			return
		}
	}

	tr, err := b.repo.CreateTrade(ctx, me, them, give, want, gold, fromPays, games.TradeOfferTTL)
	if err != nil {
		slog.Error("trade: create", "err", err)
		reply(s, i, "Nu am putut crea schimbul.")
		return
	}

	goldLine := "Fără gold, aceeași raritate."
	if gold > 0 {
		payer := theirName
		if fromPays {
			payer = myName
		}
		goldLine = fmt.Sprintf("**%s** plătește **%d gold** pentru diferența de raritate.", payer, gold)
	}
	e := &discordgo.MessageEmbed{
		Title: "Propunere de schimb",
		Description: fmt.Sprintf("**%s** oferă %s **%s** și cere %s **%s**.\n\n%s",
			myName, rm.StarBar(), mine.Name, rt.StarBar(), theirs.Name, goldLine),
		Color: rt.Colour,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%s are %s ca să răspundă", theirName, humanDur(games.TradeOfferTTL))},
	}
	id := strconv.Itoa(tr.ID)
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "<@" + targetID + ">",
			Embeds:  []*discordgo.MessageEmbed{e},
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "Accept", Style: discordgo.SuccessButton,
						CustomID: tradeAcceptID + id},
					discordgo.Button{Label: "Refuz", Style: discordgo.DangerButton,
						CustomID: tradeDeclineID + id},
				},
			}},
		},
	})
}

// onTradeButton is the other player answering.
func (b *bot) onTradeButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	raw := i.MessageComponentData().CustomID
	accept := strings.HasPrefix(raw, tradeAcceptID)
	id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(raw, tradeAcceptID), tradeDeclineID))
	if err != nil {
		return
	}
	du := interactionUser(i)
	if du == nil {
		return
	}
	me, _, err := b.repo.UserByDiscordID(ctx, du.ID)
	if err != nil {
		return
	}
	tr, err := b.repo.OpenTrade(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		editReply(s, i, "Schimbul nu mai e valabil.")
		return
	}
	if err != nil {
		return
	}
	// Only the person being asked may answer. Anyone else pressing gets nothing
	// but a private note, the same rule the collection browser uses.
	if me != tr.ToUserID {
		reply(s, i, "Schimbul ăsta nu e pentru tine.")
		return
	}

	if !accept {
		if derr := b.repo.DeclineTrade(ctx, id); derr != nil {
			slog.Error("trade: decline", "err", derr)
		}
		editReply(s, i, fmt.Sprintf("**%s** a refuzat schimbul.", tr.ToName))
		return
	}

	switch err := b.repo.AcceptTrade(ctx, id); {
	case errors.Is(err, repo.ErrNotFound):
		editReply(s, i, "Schimbul nu mai e valabil.")
	case errors.Is(err, repo.ErrExists):
		editReply(s, i, "Unul dintre voi nu mai are duplicatul. Schimbul a picat.")
	case errors.Is(err, repo.ErrConflictGold):
		editReply(s, i, fmt.Sprintf("Nu e destul gold pentru diferență (%d).", tr.Gold))
	case err != nil:
		slog.Error("trade: accept", "err", err)
		editReply(s, i, "Ceva nu a mers.")
	default:
		editReply(s, i, fmt.Sprintf(
			"Schimb făcut. **%s** a primit **%s**, **%s** a primit **%s**.",
			tr.FromName, tr.WantName, tr.ToName, tr.GiveName))
	}
}
