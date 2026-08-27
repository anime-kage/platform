// Package slug turns a series title into the URL segment that stands in for its
// id, so /anime/36/episode/3 can read /anime/91-days/episode/3 instead.
package slug

import (
	"strings"
	"unicode"
)

// ordinalSuffixes are the letter runs that must stay attached to the digits
// before them. Without this, the digit→letter rule below turns "Frieren 2nd
// Season" into "frieren-2-nd-season".
var ordinalSuffixes = map[string]bool{"st": true, "nd": true, "rd": true, "th": true}

// diacritics maps the accented letters our Romanian titles actually use onto
// ASCII. A full Unicode normalisation would pull in golang.org/x/text for a
// handful of characters; this covers Romanian plus the Latin-1 vowels that turn
// up in romanised Japanese ("Caraméliser").
var diacritics = map[rune]string{
	'ă': "a", 'â': "a", 'á': "a", 'à': "a", 'ä': "a", 'å': "a", 'ã': "a",
	'î': "i", 'í': "i", 'ì': "i", 'ï': "i",
	'ș': "s", 'ş': "s", 'š': "s",
	'ț': "t", 'ţ': "t",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
	'ó': "o", 'ò': "o", 'ô': "o", 'ö': "o", 'õ': "o", 'ø': "o",
	'ú': "u", 'ù': "u", 'û': "u", 'ü': "u",
	'ñ': "n", 'ç': "c", 'ý': "y", 'ÿ': "y",
	'æ': "ae", 'œ': "oe", 'ß': "ss",
}

// Make builds a URL slug from a title.
//
// Beyond the usual lowercase-and-hyphenate it inserts one word boundary the
// title does not spell out: digit→letter, so "91Days" becomes "91-days" rather
// than "91days". Ordinals are exempt, so "Frieren 2nd Season" keeps its "2nd".
//
// It deliberately does NOT split camelCase. That would also catch "91Days", but
// it is wrong far more often than it is right on this catalog — "JoJo" becomes
// "jo-jo" and "ReLIFE" becomes "re-life", and neither is what anyone links to.
// The digit rule alone covers the case that prompted this.
//
// Returns "" when nothing survives (a title made only of symbols), which the
// caller treats as "no slug" rather than storing an empty one.
func Make(title string) string {
	var b strings.Builder
	runes := []rune(title)

	for i, r := range runes {
		// Fold accents first so the class checks below see plain ASCII.
		if repl, ok := diacritics[unicode.ToLower(r)]; ok {
			if boundaryBefore(runes, i) {
				b.WriteByte('-')
			}
			b.WriteString(repl)
			continue
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if boundaryBefore(runes, i) {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte('-')
		}
	}

	// Collapse runs of hyphens and trim the ends.
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

// boundaryBefore reports whether a hyphen belongs before runes[i].
func boundaryBefore(runes []rune, i int) bool {
	if i == 0 {
		return false
	}
	prev, cur := runes[i-1], runes[i]

	// digit → letter, unless the letters are an ordinal suffix ("2nd", "3rd").
	// Nothing splits letter runs: see Make's comment on camelCase.
	return unicode.IsLetter(cur) && unicode.IsDigit(prev) && !startsOrdinal(runes, i)
}

// startsOrdinal reports whether the two letters at i form an ordinal suffix that
// is not itself followed by more letters — "2nd" yes, "91Days" no.
func startsOrdinal(runes []rune, i int) bool {
	if i+1 >= len(runes) {
		return false
	}
	pair := strings.ToLower(string(runes[i : i+2]))
	if !ordinalSuffixes[pair] {
		return false
	}
	// "2ndarily" would be a word, not an ordinal.
	if i+2 < len(runes) && unicode.IsLetter(runes[i+2]) {
		return false
	}
	return true
}
