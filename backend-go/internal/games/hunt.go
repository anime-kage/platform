package games

import (
	"math/rand"
	"time"
)

// HuntCooldown is how long /caut rests between attempts.
const HuntCooldown = 4 * time.Hour

// Find is one possible outcome of a treasure hunt.
type Find struct {
	Weight  int
	Name    string
	Emoji   string
	// Art is a path under the site's static arcade folder, so the hunt reuses
	// the badge set that already exists rather than inventing a second visual
	// language for the same currency.
	Art     string
	MinGold int
	MaxGold int
	MinXP   int
	MaxXP   int
	Colour  int
}

// The loot table.
//
// Sized against the site's daily chest, which pays 60 gold at streak 0. The
// expected value here is ~21 gold a hunt, so two or three hunts are worth about
// one chest -- the ratio asked for, and the reason a large share of hunts find
// nothing at all. Coming up empty is what makes it a hunt rather than a payout
// button, and it keeps the average down without making any single find feel
// stingy.
//
// Worth knowing: a four-hour cooldown allows six hunts a day, so a player who
// is present all day earns roughly 126 gold from Discord against the site's 60.
// That is deliberate -- Discord asks for attendance the site does not -- but it
// is the number to change if the site should stay ahead.
var Finds = []Find{
	// Coming up empty is now rare rather than common: at 40% the hunt spent most
	// of its turns telling people it had nothing for them, which reads as broken
	// rather than as a hunt. Payouts were left where they are, so the weight
	// that moved out of "nothing" went into the small find, and a hunt is worth
	// about 25 gold against the site chest's 60.
	{5, "Nimic", "🍃", "", 0, 0, 0, 0, 0x6B7280},
	{55, "o pungă mică", "🪙", "/arcade/icons/gold_5000.png", 8, 14, 4, 8, 0x95A5A6},
	{28, "un cufăr de lemn", "📦", "/arcade/chest-closed.png", 22, 36, 15, 25, 0x2ECC71},
	{10, "un cufăr de aur", "💰", "/arcade/chest-open.png", 60, 95, 40, 60, 0x3498DB},
	{2, "o comoară legendară", "👑", "/arcade/aur.png", 180, 180, 120, 120, 0xE0A51C},
}

// RollFind picks an outcome and the amounts inside it.
func RollFind(r *rand.Rand) (Find, int, int) {
	total := 0
	for i := range Finds {
		total += Finds[i].Weight
	}
	n := r.Intn(total)
	for i := range Finds {
		if n < Finds[i].Weight {
			f := Finds[i]
			return f, span(r, f.MinGold, f.MaxGold), span(r, f.MinXP, f.MaxXP)
		}
		n -= Finds[i].Weight
	}
	f := Finds[0]
	return f, 0, 0
}

func span(r *rand.Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.Intn(hi-lo+1)
}
