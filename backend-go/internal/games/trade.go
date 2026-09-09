package games

import "time"

// TradeOfferTTL is how long an offer waits before it lapses. Short, because an
// offer holds two players' spares in limbo and a stale one is worse than none.
const TradeOfferTTL = 10 * time.Minute

// cardValue is what a card is worth when the two sides of a trade are not the
// same rarity. Roughly 2.2x per tier, so a one step gap is pocket money and a
// legendary for a common is a week of saving.
//
// A separate ladder from the duplicate melt values on Rarity: melting is the
// game paying you to destroy a card, trading is two players agreeing what one
// is worth, and those should not be the same number. Melt stays deliberately
// poor so that trading is the better use of a spare.
func cardValue(r *Rarity) int {
	switch r.Code {
	case "legendar":
		return 500
	case "epic":
		return 225
	case "rar":
		return 100
	case "necomun":
		return 45
	default:
		return 20
	}
}

// TradeGold is what the player giving the WEAKER card must add, and who pays.
// Zero when the two are the same tier.
func TradeGold(mine, theirs *Rarity) int {
	d := cardValue(theirs) - cardValue(mine)
	if d < 0 {
		d = -d
	}
	return d
}

// TradeDebtor reports whether the offering side is the one who owes gold.
func TradeDebtor(mine, theirs *Rarity) bool {
	return cardValue(mine) < cardValue(theirs)
}
