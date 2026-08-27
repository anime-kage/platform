package resolver

// Dean Edwards' p.a.c.k.e.r, undone.
//
// Most XFileSharing-derived hosts ship their player config through it, and the
// media URL is only recoverable after unpacking: luluvdo's page carries the
// playlist in plain text on some loads and packed on others, so a resolver that
// only regexes the raw HTML works intermittently — which is worse than not
// working, because the failure looks random.
//
// This is a text transform, not an evaluator: it substitutes the dictionary back
// into the payload and returns the result for regexing. It never executes
// anything.

import (
	"regexp"
	"strconv"
	"strings"
)

// packedRe matches the tail of a packed block: the payload string, the radix,
// the word count, and the '|'-joined dictionary.
var packedRe = regexp.MustCompile(
	`\}\s*\(\s*'((?:[^'\\]|\\.)*)'\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*'((?:[^'\\]|\\.)*)'\s*\.split\('\|'\)`)

var wordRe = regexp.MustCompile(`\b[0-9a-zA-Z]+\b`)

// Unpack returns the decoded body of every packed block in src, joined by
// newlines. Input with no packed block yields "" — callers search the raw text
// as well, so that is not an error.
func Unpack(src string) string {
	var out []string
	for _, m := range packedRe.FindAllStringSubmatch(src, -1) {
		payload, radixStr, _, dictStr := m[1], m[2], m[3], m[4]

		radix, err := strconv.Atoi(radixStr)
		if err != nil || radix < 2 || radix > 36 {
			continue
		}
		payload = unescapeJS(payload)
		dict := strings.Split(unescapeJS(dictStr), "|")

		out = append(out, wordRe.ReplaceAllStringFunc(payload, func(tok string) string {
			// The token is a base-`radix` index into the dictionary. Anything
			// that is not a valid index in that base is ordinary source text and
			// must survive untouched.
			idx, err := strconv.ParseInt(strings.ToLower(tok), radix, 64)
			if err != nil || idx < 0 || int(idx) >= len(dict) {
				return tok
			}
			// An empty dictionary slot means "keep the token" in the original
			// algorithm, not "delete it".
			if dict[idx] == "" {
				return tok
			}
			return dict[idx]
		}))
	}
	return strings.Join(out, "\n")
}

// unescapeJS undoes the backslash escaping the packer applies to its two string
// literals. Only the sequences it actually emits.
func unescapeJS(s string) string {
	return strings.NewReplacer(
		`\\`, `\`,
		`\'`, `'`,
		`\"`, `"`,
		`\/`, `/`,
	).Replace(s)
}
