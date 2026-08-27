package anilist

import "testing"

// AniList's streamingEpisodes carry the number inside the title string, so this
// parse is the only thing standing between "titles arrive" and "titles land on
// the wrong episodes". A wrong number is worse than a missing title, hence the
// cases that must be rejected rather than guessed at.
func TestParseEpisodeTitle(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		wantNum   int
		wantTitle string
		wantOK    bool
	}{
		// the common Crunchyroll shape, as returned for 91 Days
		{"Episode 1 - Night of the Murder", 1, "Night of the Murder", true},
		{"Episode 12 - The End of the 91 Days", 12, "The End of the 91 Days", true},
		// capitalisation of the title must survive the case-insensitive match
		{"Episode 3 - Where the Footfalls Lead", 3, "Where the Footfalls Lead", true},
		// separator variants
		{"Episode 4: Losing to Win", 4, "Losing to Win", true},
		{"Episode 5 – Blood Will Have Blood", 5, "Blood Will Have Blood", true},
		{"Ep. 7 - Sundown", 7, "Sundown", true},
		{"E9 - Nightfall", 9, "Nightfall", true},
		{"10 - Bare bones", 10, "Bare bones", true},
		// three and four digit numbers (long runners)
		{"Episode 500 - Something", 500, "Something", true},

		// rejected: no number to anchor on
		{"Night of the Murder", 0, "", false},
		// rejected: a number but no title
		{"Episode 1", 0, "", false},
		{"Episode 1 - ", 0, "", false},
		// rejected: episode 0 is not a thing we store (CHECK > 0)
		{"Episode 0 - Prologue", 0, "", false},
		{"", 0, "", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			num, title, ok := parseEpisodeTitle(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if num != tc.wantNum {
				t.Errorf("number = %d, want %d", num, tc.wantNum)
			}
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
		})
	}
}
