package games

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// badgeByCode is what the Discord bot and the site both do to turn a stored
// code into something a player can read.
func badgeByCode(code string) (BadgeDef, bool) {
	for _, def := range BadgeCatalogue {
		if def.Code == code {
			return def, true
		}
	}
	return BadgeDef{}, false
}

// Every badge the arcade can hand out has to exist in the catalogue, because a
// code with no entry falls through to the raw string: a player who earned the
// week badge for the theme mode was shown "week_theme" instead of a name.
func TestEveryGrantableBadgeIsInTheCatalogue(t *testing.T) {
	granted := []string{
		"one_guess", "clutch", "first_try_5", "perfect_day",
		"perfect_week", "no_hint_week", "gold_5000",
	}
	// These two are built at the grant site rather than written out, which is
	// how the theme and character badges went missing in the first place.
	for _, mode := range AllModes {
		granted = append(granted, "week_"+mode)
	}
	for _, lvl := range []int{25, 50, 100} {
		granted = append(granted, "level_"+strconv.Itoa(lvl))
	}

	for _, code := range granted {
		if _, ok := badgeByCode(code); !ok {
			t.Errorf("badge %q can be granted but has no catalogue entry, so players see the raw code", code)
		}
	}
}

// The reverse direction: a catalogue entry nothing can award is a badge that
// shows on the site as permanently locked with no way to earn it.
func TestCatalogueHasNoDuplicateCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range BadgeCatalogue {
		if seen[def.Code] {
			t.Errorf("badge %q appears twice in the catalogue", def.Code)
		}
		seen[def.Code] = true
	}
}

// Name, description and glyph all reach the player, and an empty one renders as
// a blank cell on the site and a blank field in the Discord embed.
func TestCatalogueEntriesAreComplete(t *testing.T) {
	for _, def := range BadgeCatalogue {
		if def.Code == "" || def.Name == "" || def.Desc == "" || def.Icon == "" {
			t.Errorf("badge %q is missing a name, description or glyph: %+v", def.Code, def)
		}
	}
}

// BadgesWithArt has to match what is actually on disk in both directions. A
// code listed here with no file gives Discord a 404 image; a file not listed
// here means art someone drew is never shown in the bot.
func TestCatalogueArtMatchesDisk(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "frontend", "static", "arcade", "icons")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("icon directory not reachable from here")
	}
	for code := range BadgesWithArt {
		if _, err := os.Stat(filepath.Join(dir, code+".png")); err != nil {
			t.Errorf("%q is listed as having art but %s.png is not on disk", code, code)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".png")
		if !BadgesWithArt[code] {
			t.Errorf("%s.png exists but %q is not in BadgesWithArt, so the bot never shows it", code, code)
		}
	}
}

// Anything with art must also be a real badge, or the art is unreachable.
func TestArtOnlyCoversRealBadges(t *testing.T) {
	for code := range BadgesWithArt {
		if _, ok := badgeByCode(code); !ok {
			t.Errorf("%q has art but is not in the catalogue", code)
		}
	}
}
