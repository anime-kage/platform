package games

import (
	"strings"
	"time"
	"unicode"
)

// Modes the arcade can generate. All four have generators and run daily.
const (
	ModeTitle     = "title"
	ModePoster    = "poster"
	ModeTheme     = "theme"
	ModeCharacter = "character"
	ModeGroups    = "groups"
)

// AllModes is every mode that can be played, and so every mode that can earn a
// week badge. It exists so the badge catalogue can be checked against it: the
// week badge code is built as "week_"+mode, and a mode added without its
// catalogue entry shows players the raw code instead of a name.
var AllModes = []string{ModeTitle, ModePoster, ModeTheme, ModeCharacter, ModeGroups}

// HistoryDays is how far back the arcade stays playable: today plus six.
// Anything older disappears from the board, which is what makes a streak worth
// keeping and stops the page growing without bound.
const HistoryDays = 7

// MaxGuesses before a puzzle is spent. Six is the familiar number from this
// genre and it fits the six clues the title mode reveals.
const MaxGuesses = 6

// Base award for solving on the day. Two modes at this rate is ~240 XP and 120
// gold daily, which is the budget the rest of the economy is priced against
// (docs/GAMES-ARCHITECTURE.md §3).
const (
	BaseXP   = 120
	BaseGold = 60
)

// Award is what solving a puzzle pays.
//
// Full value on the day, half for anything caught up later in the window. The
// catch-up rate exists so that missing a day is a real loss without being a
// permanent one -- coming back and clearing the week is worth doing, but never
// as good as showing up.
func Award(playDate, today time.Time) (xp int, gold int) {
	if sameDay(playDate, today) {
		return BaseXP, BaseGold
	}
	return BaseXP / 2, BaseGold / 2
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// InWindow reports whether a date is still playable.
func InWindow(playDate, today time.Time) bool {
	d := today.Sub(playDate).Hours() / 24
	return d >= -0.5 && d < float64(HistoryDays)
}

// Normalise reduces a title to something two humans typing the same show will
// agree on: lower case, accents and punctuation dropped, articles removed,
// runs of space collapsed.
//
// The catalogue stores several spellings of the same series (romaji, English,
// Romanian), and players type a fourth. Without this, "Shingeki no Kyojin" and
// "shingeki no kyojin!" are different answers and the game feels broken.
func Normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(foldAccent(r))
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
		// everything else -- punctuation, symbols -- is dropped
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	for _, a := range []string{"the ", "a ", "an "} {
		out = strings.TrimPrefix(out, a)
	}
	return out
}

func foldAccent(r rune) rune {
	switch r {
	case 'ă', 'â', 'à', 'á', 'ä', 'ã':
		return 'a'
	case 'î', 'ì', 'í', 'ï':
		return 'i'
	case 'ș', 'š':
		return 's'
	case 'ț', 'ť':
		return 't'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'ô', 'ö', 'ò', 'ó', 'õ':
		return 'o'
	case 'û', 'ü', 'ù', 'ú':
		return 'u'
	}
	return r
}

// Matches reports whether a guess names the answer. Any of the stored spellings
// counts, so a player is never punished for knowing the show under a different
// name than the row happens to lead with.
func Matches(guess string, accepted []string) bool {
	g := Normalise(guess)
	if g == "" {
		return false
	}
	for _, a := range accepted {
		if a == "" {
			continue
		}
		if Normalise(a) == g {
			return true
		}
	}
	return false
}

// RevealStages is how many steps the poster silhouette moves through before it
// is fully visible. Stage 0 is the flat silhouette; the last is the plain
// image. Kept here rather than in CSS so the server can cap what a spent
// attempt is allowed to see.
const RevealStages = 5

// Stage maps guesses used to how much of the poster has been uncovered.
func Stage(guesses int) int {
	if guesses >= RevealStages {
		return RevealStages
	}
	return guesses
}

// HintAfter is how many wrong guesses must be spent before the synopsis is
// offered. With MaxGuesses at 6 that puts it on the fifth attempt: late enough
// that it is a rescue rather than a shortcut.
const HintAfter = 4

// HintPenalty is the share of the award given up for taking it.
const HintPenalty = 0.25

// HintAvailable reports whether the clue can still be bought for this attempt.
func HintAvailable(guesses int, finished, used bool) bool {
	return !finished && !used && guesses >= HintAfter
}

// ApplyHint reduces an award for a player who took the synopsis.
func ApplyHint(xp, gold int, used bool) (int, int) {
	if !used {
		return xp, gold
	}
	keep := 1 - HintPenalty
	return int(float64(xp) * keep), int(float64(gold) * keep)
}

// NormaliseKey is the lookup form: letters and digits only, no spaces.
//
// It must agree exactly with the expression CardByTitle uses in SQL. Normalise
// above keeps spaces and trims articles, which is right for comparing what a
// person typed to what they meant -- but it is NOT the same string, and using
// one against the other silently matches nothing.
func NormaliseKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(foldAccent(r))
		}
	}
	return b.String()
}

// ── Character round ─────────────────────────────────────────────────────────

// LeaderboardSize caps the arcade ranking. Long enough that a regular player
// can find themselves on it, short enough to stay one glance rather than a
// directory of everyone who ever opened the page.
const LeaderboardSize = 50

// CharSlots is how many portraits a round shows.
const CharSlots = 5

// Character scoring is per answer, not per round: naming the person without
// placing the series is still knowledge, and a round that pays nothing unless
// both land would make four-fifths of a good attempt worth zero.
//
// A full sweep is 160 XP against 120 for a single-answer puzzle -- more, because
// it is ten answers, but not so much that the other modes stop mattering.
const (
	CharNameXP   = 20
	CharNameGold = 10
	CharShowXP   = 12
	CharShowGold = 6
)

// ScoreCharRound totals one submission. Each slot contributes independently.
func ScoreCharRound(nameHits, showHits int) (xp int, gold int) {
	xp = nameHits*CharNameXP + showHits*CharShowXP
	gold = nameHits*CharNameGold + showHits*CharShowGold
	return
}

// ── Grupe ───────────────────────────────────────────────────────────────────

// A Grupe round is sixteen titles hiding four groups of four. The numbers are
// the ones the format is known by, and they are load-bearing: fewer than four
// groups makes the last one free, and more than four tiles per group turns the
// board into a search rather than a read.
const (
	GroupCount = 4 // groups hidden in the board
	// Three per group, not four. Sixteen unfamiliar romaji titles is a wall to
	// read before you can even start comparing them, and the traits here
	// (studio, year, genre) are fuzzier than the wordplay the format was built
	// on, so the round was already harder than it looked. Twelve also leaves
	// the posters room to be large enough to recognise, which is most of how a
	// player actually spots a group.
	GroupSize = 3 // titles per group
	GroupTiles = GroupCount * GroupSize
	// Mistakes allowed before the round ends. Four is the familiar number and
	// it leaves room to be wrong about the group you were least sure of.
	MaxGroupMistakes = 4
)

// ScoreGroups pays for the groups actually found rather than all-or-nothing.
// Finding three of four is most of the work, and a round that pays nothing for
// it teaches people not to finish -- the same reason the character round scores
// each answer separately.
func ScoreGroups(found int) (xp, gold int) {
	if found <= 0 {
		return 0, 0
	}
	if found > GroupCount {
		found = GroupCount
	}
	xp = BaseXP * found / GroupCount
	gold = BaseGold * found / GroupCount
	// Clearing the board is worth more than the sum of its groups: the last
	// group is the one you had to be right about everything else to reach.
	if found == GroupCount {
		xp += BaseXP / 4
		gold += BaseGold / 4
	}
	return xp, gold
}

// GroupProgress replays a round's submissions against the answer.
//
// Kept pure and here rather than in the handler so the rules -- what counts as
// finding a group, what counts as a mistake, when the round is over -- can be
// tested without a database.
//
// `submitted` is every set the player has sent, oldest first. `answer` is the
// four hidden groups. Returns the indices of the groups found, in the order
// they were found, and the number of submissions that matched nothing.
func GroupProgress(submitted [][]int, answer [][]int) (found []int, mistakes int) {
	sameSet := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		seen := make(map[int]int, len(a))
		for _, v := range a {
			seen[v]++
		}
		for _, v := range b {
			seen[v]--
			if seen[v] < 0 {
				return false
			}
		}
		return true
	}
	already := map[int]bool{}
	for _, sub := range submitted {
		hit := -1
		for i, ans := range answer {
			if !already[i] && sameSet(sub, ans) {
				hit = i
				break
			}
		}
		if hit >= 0 {
			already[hit] = true
			found = append(found, hit)
			continue
		}
		// A set that repeats one already found is not a new mistake: the board
		// should not punish a double click.
		dup := false
		for i, ans := range answer {
			if already[i] && sameSet(sub, ans) {
				dup = true
				break
			}
		}
		if !dup {
			mistakes++
		}
	}
	return found, mistakes
}

// GroupRoundOver reports whether a Grupe round has ended, either by clearing
// the board or by running out of mistakes.
func GroupRoundOver(found int, mistakes int) bool {
	return found >= GroupCount || mistakes >= MaxGroupMistakes
}

// GroupNearMiss reports how many of a submission belong to one and the same
// group, which is the only useful thing to say about a wrong answer: three of
// four means the idea was right and one title was not.
func GroupNearMiss(sub []int, answer [][]int) int {
	best := 0
	for _, ans := range answer {
		in := map[int]bool{}
		for _, v := range ans {
			in[v] = true
		}
		n := 0
		for _, v := range sub {
			if in[v] {
				n++
			}
		}
		if n > best {
			best = n
		}
	}
	return best
}
