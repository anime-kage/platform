package repo

import (
	"fmt"
	"math/rand"
	"testing"

	"animekage/backend/internal/games"
)

// samplePool builds candidates that overlap heavily on purpose: every series
// carries a studio, a year and two genres, which is what makes a naive pick
// place the same title in two groups.
func samplePool(n int) []groupCandidate {
	out := make([]groupCandidate, 0, n)
	genres := []string{"Action", "Drama", "Comedy", "Fantasy", "Thriller"}
	for i := 1; i <= n; i++ {
		y := 2000 + i%12
		out = append(out, groupCandidate{
			ID:      i,
			Title:   fmt.Sprintf("Seria %d", i),
			Studios: []string{fmt.Sprintf("Studio %d", i%9)},
			Genres:  []string{genres[i%len(genres)], genres[(i+2)%len(genres)]},
			Year:    &y,
		})
	}
	return out
}

// The board is unsolvable if a title sits in two groups: the player removes it
// with one group and the other can never be completed.
func TestGroupsDoNotShareTitles(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for run := 0; run < 200; run++ {
		sets, ok := buildGroupSets(samplePool(200), rnd)
		if !ok {
			t.Fatalf("run %d: could not build a board from a pool that has room", run)
		}
		if len(sets) != games.GroupCount {
			t.Fatalf("run %d: got %d groups, want %d", run, len(sets), games.GroupCount)
		}
		seen := map[int]string{}
		for _, g := range sets {
			if len(g.AnimeIDs) != games.GroupSize {
				t.Fatalf("run %d: group %q has %d titles, want %d",
					run, g.Label, len(g.AnimeIDs), games.GroupSize)
			}
			if g.Label == "" {
				t.Fatalf("run %d: a group has no label", run)
			}
			for _, id := range g.AnimeIDs {
				if prev, dup := seen[id]; dup {
					t.Fatalf("run %d: title %d is in both %q and %q",
						run, id, prev, g.Label)
				}
				seen[id] = g.Label
			}
		}
		if len(seen) != games.GroupTiles {
			t.Fatalf("run %d: board has %d tiles, want %d", run, len(seen), games.GroupTiles)
		}
	}
}

// Two groups with the same label read as one group split in half.
func TestGroupLabelsAreDistinct(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	for run := 0; run < 100; run++ {
		sets, ok := buildGroupSets(samplePool(200), rnd)
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, g := range sets {
			if seen[g.Label] {
				t.Fatalf("run %d: label %q used twice", run, g.Label)
			}
			seen[g.Label] = true
		}
	}
}

// A pool too small to field four disjoint groups must report failure rather
// than emit a short board the UI cannot render.
func TestTooSmallAPoolIsRefused(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	if sets, ok := buildGroupSets(samplePool(6), rnd); ok {
		t.Errorf("expected refusal on a 6-title pool, got %d groups", len(sets))
	}
}

// A board of four studios is the same puzzle four times, and it hands the
// player the grouping rule for free. Every board must mix trait types.
func TestBoardsMixTraitTypes(t *testing.T) {
	rnd := rand.New(rand.NewSource(11))
	for run := 0; run < 200; run++ {
		sets, ok := buildGroupSets(samplePool(200), rnd)
		if !ok {
			continue
		}
		kinds := map[string]int{}
		for _, g := range sets {
			switch {
			case len(g.Label) > 7 && g.Label[:7] == "Studio:":
				kinds["studio"]++
			case len(g.Label) > 4 && g.Label[:4] == "Anul":
				kinds["an"]++
			default:
				kinds["gen"]++
			}
		}
		for kind, n := range kinds {
			if n > 2 {
				t.Fatalf("run %d: %d of %d groups are %q, at most 2 allowed",
					run, n, len(sets), kind)
			}
		}
		if len(kinds) < 2 {
			t.Fatalf("run %d: every group is the same kind of fact (%v)", run, kinds)
		}
	}
}
