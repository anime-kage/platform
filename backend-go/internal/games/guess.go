package games

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// GuessMinToken is how short a single-word guess may be.
//
// Four, because "Monkey D. Luffy" tokenises to {monkey, d, luffy} and a player
// typing "d" must not win. Names shorter than this exist -- "Rem" -- but they
// are matched whole by the exact rule below, which has no minimum.
const GuessMinToken = 4

var nonAlnum = regexp.MustCompile(`[^0-9a-z]+`)

// NameTokens lowercases, strips diacritics and punctuation, and splits.
// "Übel" and "ubel" therefore agree, as do "Monkey D. Luffy" and "monkey luffy".
func NameTokens(name string) []string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	flat, _, err := transform.String(t, strings.ToLower(name))
	if err != nil {
		flat = strings.ToLower(name)
	}
	out := []string{}
	for _, part := range nonAlnum.Split(flat, -1) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// GuessMatches decides whether a typed guess names a character.
//
// Three ways to be right, in order of how sure we are:
//
//  1. the whole name, however short -- "rem" wins Rem
//  2. any single token of at least GuessMinToken -- "luffy", "monkey"
//  3. any subset of the name's tokens -- "monkey luffy", without the D.
//
// Deliberately generous. A surname can belong to several characters -- "souma"
// is fourteen of them and "zoldyck" six -- but only one character is ever up for
// grabs, so a family name is knowledge rather than a lucky hit. The minimum
// length and a per-player cooldown are what stop it being brute forced.
func GuessMatches(guess, name string) bool {
	g := NameTokens(guess)
	n := NameTokens(name)
	if len(g) == 0 || len(n) == 0 {
		return false
	}
	if strings.Join(g, " ") == strings.Join(n, " ") {
		return true
	}
	have := make(map[string]bool, len(n))
	for _, t := range n {
		have[t] = true
	}
	if len(g) == 1 {
		return len(g[0]) >= GuessMinToken && have[g[0]]
	}
	// Multi word: every word must belong to the name, and at least one of them
	// must be substantial, so "d d" cannot win.
	long := false
	for _, t := range g {
		if !have[t] {
			return false
		}
		if len(t) >= GuessMinToken {
			long = true
		}
	}
	return long
}

// Drop pacing.
//
// Hours are the server's, which runs UTC; Romania is two or three ahead
// depending on the season, so 07:00-21:00 UTC is roughly 10:00 to midnight
// there. Confining drops to waking hours is what makes six of them contested
// rather than three of them appearing while everyone is asleep.
const (
	DropTick       = 10 * time.Minute
	DropsPerDay    = 7
	DropRepeatDays = 30 // do not re-drop a character seen this recently
	GuessRest      = 8 * time.Second
	dropStartUTC   = 7
	dropEndUTC     = 21
)

// WithinDropHours keeps drops inside the part of the day people are awake.
func WithinDropHours(t time.Time) bool {
	h := t.UTC().Hour()
	return h >= dropStartUTC && h < dropEndUTC
}

// DropDue spreads the day's quota across the window instead of firing them all
// at once: by the time n have gone out, the day must be at least n/quota of the
// way through the active window.
func DropDue(t time.Time, doneToday int) bool {
	h := float64(t.UTC().Hour()) + float64(t.UTC().Minute())/60
	span := float64(dropEndUTC - dropStartUTC)
	progress := (h - float64(dropStartUTC)) / span
	return progress >= float64(doneToday)/float64(DropsPerDay)
}

// editDistance is Levenshtein, two rows rather than a full matrix: the strings
// are names, so the matrix would be tiny either way, but there is no reason to
// allocate it.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) < len(br) {
		ar, br = br, ar
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// GuessNear reports whether a wrong guess was nearly right, so the answer can
// say "close" instead of a flat no.
//
// Tolerance scales with length: one wrong letter in a short name is a different
// thing from one in a long one. "wuffy" is a typo for "luffy"; "zoro" is not.
// Deliberately not counted as a win -- knowing the name is the game, and a
// spellchecker that accepts anything within two letters would let someone brute
// force the pool with near misses.
func GuessNear(guess, name string) bool {
	// A correct guess is not a near miss. Checking here rather than trusting the
	// caller to ask in the right order keeps the two answers mutually exclusive
	// wherever the function is used.
	if GuessMatches(guess, name) {
		return false
	}
	g, n := NameTokens(guess), NameTokens(name)
	for _, gt := range g {
		if len(gt) < GuessMinToken {
			continue
		}
		for _, nt := range n {
			if len(nt) < GuessMinToken {
				continue
			}
			allowed := 1
			if len(nt) >= 6 {
				allowed = 2
			}
			if editDistance(gt, nt) <= allowed {
				return true
			}
		}
	}
	return false
}
