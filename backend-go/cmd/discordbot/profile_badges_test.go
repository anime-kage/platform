package main

import (
	"strings"
	"testing"

	"animekage/backend/internal/games"
)

// badgeName falls through to the raw code when a badge is not in the
// catalogue, which is how a player came to see "week_theme" in their profile.
// Every code the arcade can award has to come back as a readable name.
func TestBadgeNameNeverLeaksTheRawCode(t *testing.T) {
	for _, def := range games.BadgeCatalogue {
		got := badgeName(def.Code)
		if got == def.Code {
			t.Errorf("badge %q renders as its own code instead of a name", def.Code)
		}
		if strings.Contains(got, "_") {
			t.Errorf("badge %q renders as %q, which still looks like a code", def.Code, got)
		}
	}
}

// The profile picture is the newest badge that has art. A player whose newest
// badge has none must still get a picture rather than an embed with a hole.
func TestNewestBadgeWithArtIsChosen(t *testing.T) {
	held := []string{"one_guess", "level_100", "week_theme", "week_character"}

	var pick string
	for i := len(held) - 1; i >= 0; i-- {
		if games.BadgesWithArt[held[i]] {
			pick = held[i]
			break
		}
	}
	if pick != "level_100" {
		t.Errorf("expected the newest badge with art (level_100), got %q", pick)
	}
}
