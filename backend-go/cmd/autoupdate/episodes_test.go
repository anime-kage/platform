package main

import "testing"

// AniList's streamingEpisodes is a flat listing that keeps counting across
// seasons. The Tower of God season 1 entry (MAL says 13) lists all 26 episodes
// of seasons 1 and 2, and before this bound episodes 14-26 were created against
// season 1 carrying season 2's titles, with no sources on any of them.
func TestAniListEpisodesStopAtTheSeriesLength(t *testing.T) {
	titles := map[int]string{}
	for i := 1; i <= 26; i++ {
		titles[i] = "titlu"
	}
	thirteen := 13

	eps, skipped := episodesFromAniList(titles, &thirteen)

	if len(eps) != 13 {
		t.Fatalf("expected 13 episodes, got %d", len(eps))
	}
	if skipped != 13 {
		t.Errorf("expected 13 dropped, got %d", skipped)
	}
	for _, e := range eps {
		if e.Number > 13 {
			t.Errorf("episode %d is past the series length and should not have been kept", e.Number)
		}
	}
}

// A series still airing has no length on MAL, and bounding by zero there would
// throw away every episode.
func TestUnknownLengthKeepsEverything(t *testing.T) {
	titles := map[int]string{1: "a", 2: "b", 3: "c"}

	for _, max := range []*int{nil, new(int)} { // nil and 0
		eps, skipped := episodesFromAniList(titles, max)
		if len(eps) != 3 || skipped != 0 {
			t.Errorf("max=%v: expected all 3 kept, got %d kept and %d dropped",
				max, len(eps), skipped)
		}
	}
}

// The map has no order, and the caller inserts in the order given.
func TestEpisodesComeBackInOrder(t *testing.T) {
	eps, _ := episodesFromAniList(map[int]string{3: "c", 1: "a", 2: "b"}, nil)
	for i, e := range eps {
		if e.Number != i+1 {
			t.Fatalf("position %d holds episode %d, expected %d", i, e.Number, i+1)
		}
	}
}

// A zero or negative number means the title did not parse, and inventing an
// episode 0 puts a row on the page that no source will ever fill.
func TestNonPositiveNumbersAreDropped(t *testing.T) {
	eps, _ := episodesFromAniList(map[int]string{0: "zero", -1: "neg", 1: "ok"}, nil)
	if len(eps) != 1 || eps[0].Number != 1 {
		t.Errorf("expected only episode 1, got %+v", eps)
	}
}
