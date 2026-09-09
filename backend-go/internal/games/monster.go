package games

import (
	"math/rand"
	"time"
)

// Monsters are a hand written roster, not drawn from game_characters.
//
// The imported characters are people: searching them for anything monstrous
// turns up the cast of the anime "Monster" and Monkey D. Dragon, which is a
// pun rather than a boss. So the roster lives here, small and obvious, with
// Art pointing at whatever file is dropped into the site's static folder. The
// intent is that names and pictures get replaced with real ones later and the
// only edit needed is in this table.
type Monster struct {
	Code   string
	Name   string
	Tier   string // shown on the card, so a player knows what they walked into
	Weight int    // relative spawn chance
	HP     int    // total damage the channel must deal together
	XP     int    // split between everyone who landed a hit
	Gold   int
	Colour int
	Art    string // under the site's static folder
	Minutes int   // how long the channel has to bring it down
}

// Three tiers. The weight lives on the monster rather than in a parallel slice:
// the old version indexed a []int by position, so adding a monster silently
// shifted every chance below it.
//
// HP is expressed in hits: AttackDamage averages 55, and a player may swing
// once every five minutes, so a Slime is about twelve hits and Kaido about
// seventy-six. That is the difference between something two people finish over a
// coffee and something the channel has to turn up for.
var Monsters = []Monster{
	// Mic: common, quick, modest. What most spawns will be.
	{"slime", "Slime", "Mic", 20, 650, 150, 75, 0x3FB27F, "/arcade/monsters/slime.png", 20},
	{"hollow", "Hollow", "Mic", 20, 700, 160, 80, 0x6B7280, "/arcade/monsters/hollow.png", 20},
	{"salibaman", "Salibaman", "Mic", 20, 800, 170, 85, 0x8E7CC3, "/arcade/monsters/salibaman.png", 20},

	// Mediu: needs a few people, pays accordingly.
	{"goblin", "Goblin", "Mediu", 11, 1550, 400, 200, 0x2ECC71, "/arcade/monsters/goblin.png", 35},
	{"titan", "Titan", "Mediu", 11, 1800, 450, 225, 0xC0392B, "/arcade/monsters/titan.png", 35},
	{"chimera", "Chimera Ant", "Mediu", 10, 2100, 500, 250, 0xE67E22, "/arcade/monsters/chimera.png", 35},

	// Mare: rare, long, and worth showing up for.
	{"kaido", "Kaido", "Mare", 4, 4200, 1100, 550, 0x2980B9, "/arcade/monsters/kaido.png", 60},
	{"kurama", "Kurama", "Mare", 4, 3900, 1000, 500, 0xE0A51C, "/arcade/monsters/kurama.png", 60},
}

// AttackDamage is one player's hit.
//
// A wide spread on purpose: a narrow one would make the fight arithmetic, where
// everyone can work out exactly how many hits are left and the last person to
// arrive knows they are wasting their time.
func AttackDamage(r *rand.Rand) int {
	return 25 + r.Intn(61) // 25..85, mean 55
}

// RollMonster picks a spawn, weighted towards the smaller ones so the channel
// is not permanently besieged by something it cannot finish.
func RollMonster(r *rand.Rand) *Monster {
	total := 0
	for i := range Monsters {
		total += Monsters[i].Weight
	}
	n := r.Intn(total)
	for i := range Monsters {
		if n < Monsters[i].Weight {
			return &Monsters[i]
		}
		n -= Monsters[i].Weight
	}
	return &Monsters[0]
}

// MonsterByCode is for spawning a specific one by hand.
func MonsterByCode(code string) *Monster {
	for i := range Monsters {
		if Monsters[i].Code == code {
			return &Monsters[i]
		}
	}
	return nil
}

// ArtVersion busts Discord's image cache.
//
// Discord proxies embed images through its own CDN and caches them by URL, so
// replacing a file on the server leaves every client showing the old one
// indefinitely. Bump this whenever the art changes and the URL changes with it.
const ArtVersion = "3"

// ArtURL is a monster's picture, cache-busted.
func (m Monster) ArtURL(publicURL string) string {
	if m.Art == "" {
		return ""
	}
	return publicURL + m.Art + "?v=" + ArtVersion
}

// AttackRest is the wait between blows, and CounterRest the longer one imposed
// when the monster gets its own hit in. CounterChance is in percent.
//
// The rest is what makes a fight take the channel rather than one determined
// person: without it whoever typed fastest could solo a Balaur in ninety
// seconds and everyone else would arrive to a corpse.
const (
	AttackRest    = 5 * time.Minute
	CounterRest   = 30 * time.Minute
	CounterChance = 1
)

// HuntSpawnChance is how often a /caut wakes something up, in percent. Low
// enough that the hunt stays a hunt, high enough that a busy afternoon sees a
// fight without anyone having to arrange one.
const HuntSpawnChance = 12

// SplitReward divides a pot by damage share, with a floor so that landing a
// hit is always worth more than watching. Returns the amount for one player.
func SplitReward(pot, mine, total int) int {
	if total <= 0 || mine <= 0 {
		return 0
	}
	share := pot * mine / total
	if share < 1 {
		share = 1
	}
	return share
}

// EscapeRate is the fraction of the pot paid out when a monster gets away.
//
// Half. Landing hits on something the channel could not finish should be worth
// something -- the cooldown was spent either way, and paying nothing makes
// turning up for a fight you might lose the wrong choice. But finishing has to
// be worth more per point of damage, or a channel is indifferent between
// killing a monster and letting it escape.
const EscapeRate = 0.5

// EscapeReward is one player's share when the monster escapes: their damage as
// a fraction of the monster's FULL health, not of the damage actually dealt.
// Scaling by total damage would pay a channel that removed 4% of the health the
// same as one that removed 99%.
func EscapeReward(pot, mine, hpTotal int) int {
	if hpTotal <= 0 || mine <= 0 {
		return 0
	}
	return int(float64(pot) * float64(mine) / float64(hpTotal) * EscapeRate)
}
