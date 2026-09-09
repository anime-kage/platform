package games

import "math/rand"

// The gacha's rarity ladder.
//
// Rarity is read off game_characters.favorites rather than stored: the harvest
// already ranks every character by how many people on MyAnimeList and AniList
// picked them as a favourite, which is a better measure of "did the player
// recognise this" than anything we would invent. It also means new imports slot
// themselves into the ladder with no curation step.
//
// The pool sizes that fall out of the current 914 characters are, conveniently,
// already pyramid-shaped: 40 / 82 / 164 / 260 / 368.
type Rarity struct {
	Code    string
	Name    string // Romanian, as shown in Discord
	MinFavs int
	Weight  int    // relative draw chance
	Colour  int    // embed side bar
	Stars   int    // shown as ★, because a word alone does not rank at a glance
	DupXP   int    // converting a duplicate pays this
	DupGold int
}

// StarBar renders the tier as gold stars.
//
// The emoji star, not the typographic one: U+2605 renders in the message's text
// colour, which is white on Discord's dark theme and grey on light, so a rarity
// bar built from it looked like disabled text. Only the filled stars are drawn
// -- an empty ☆ would fall back to text colour again and reintroduce exactly
// the mismatch, and the tier's name sits next to the bar anyway.
func (r Rarity) StarBar() string {
	out := ""
	for i := 0; i < r.Stars; i++ {
		out += "⭐"
	}
	return out
}

// Ordered rarest first, which is also the order the draw walks.
var Rarities = []Rarity{
	{"legendar", "Legendar", 20000, 2, 0xE0A51C, 5, 200, 100},
	{"epic", "Epic", 8000, 8, 0x9B59B6, 4, 90, 45},
	{"rar", "Rar", 2500, 20, 0x3498DB, 3, 40, 20},
	{"necomun", "Necomun", 700, 30, 0x2ECC71, 2, 18, 9},
	{"comun", "Comun", 0, 40, 0x95A5A6, 1, 8, 4},
}

// RarityFor maps a favourite count onto the ladder. Never returns nil: the last
// tier has MinFavs 0, so everything lands somewhere.
func RarityFor(favourites int) *Rarity {
	for i := range Rarities {
		if favourites >= Rarities[i].MinFavs {
			return &Rarities[i]
		}
	}
	return &Rarities[len(Rarities)-1]
}

// RollRarity picks a tier by weight.
//
// Weighted by tier and not by character on purpose. Drawing uniformly from 914
// characters would make rarity meaningless -- 40% of the pool is Comun, so a
// uniform draw already yields the ladder's shape by accident and a Legendar
// would arrive as often as its share of the pool. Choosing the tier first and
// the character second decouples "how rare is this" from "how many of them did
// we happen to import".
func RollRarity(r *rand.Rand) *Rarity {
	total := 0
	for i := range Rarities {
		total += Rarities[i].Weight
	}
	n := r.Intn(total)
	for i := range Rarities {
		if n < Rarities[i].Weight {
			return &Rarities[i]
		}
		n -= Rarities[i].Weight
	}
	return &Rarities[len(Rarities)-1]
}

// RarityByCode looks a tier up for rendering a stored card.
func RarityByCode(code string) *Rarity {
	for i := range Rarities {
		if Rarities[i].Code == code {
			return &Rarities[i]
		}
	}
	return nil
}

// ExtraDrawCost is what the next paid draw costs, given how many have already
// been bought today.
//
// It doubles. A flat price would let anyone sitting on a pile of gold draw
// until the collection was finished, which ends the collecting game rather than
// feeding it; doubling means the fourth draw of a day costs eight times the
// first and the pile empties on its own. The first one is priced at roughly two
// treasure hunts, so an extra card is an afternoon's play rather than a
// formality.
const (
	ExtraDrawBase = 60
	ExtraDrawMax  = 5 // per day, after which gold cannot buy another
)

func ExtraDrawCost(boughtToday int) int {
	cost := ExtraDrawBase
	for i := 0; i < boughtToday; i++ {
		cost *= 2
	}
	return cost
}
