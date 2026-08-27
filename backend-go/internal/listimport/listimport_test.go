package listimport

import "testing"

// Both sources have to land on the same five statuses, and the anime/manga
// split matters: "reading" in a watchlist row is invalid.
func TestStatusMapping(t *testing.T) {
	tests := []struct {
		name          string
		fromAniList   bool
		in            string
		manga         bool
		want          string
		wantSupported bool
	}{
		{"anilist current anime", true, "CURRENT", false, StatusWatching, true},
		{"anilist current manga", true, "CURRENT", true, StatusReading, true},
		{"anilist planning anime", true, "PLANNING", false, StatusPlanWatch, true},
		{"anilist planning manga", true, "PLANNING", true, StatusPlanRead, true},
		{"anilist paused is on-hold", true, "PAUSED", false, StatusOnHold, true},
		{"anilist dropped", true, "DROPPED", false, StatusDropped, true},
		{"anilist completed", true, "COMPLETED", false, StatusCompleted, true},
		// a rewatch is closer to watching than to completed, and we have no
		// third state for it
		{"anilist repeating collapses to watching", true, "REPEATING", false, StatusWatching, true},
		{"anilist unknown", true, "SOMETHING_NEW", false, "", false},

		{"mal watching", false, "Watching", false, StatusWatching, true},
		{"mal reading", false, "Reading", true, StatusReading, true},
		{"mal plan to watch", false, "Plan to Watch", false, StatusPlanWatch, true},
		{"mal plan to read", false, "Plan to Read", true, StatusPlanRead, true},
		{"mal on-hold keeps its hyphen", false, "On-Hold", false, StatusOnHold, true},
		{"mal dropped", false, "Dropped", false, StatusDropped, true},
		{"mal completed", false, "Completed", false, StatusCompleted, true},
		{"mal unknown", false, "Rewatching", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			var ok bool
			if tt.fromAniList {
				got, ok = FromAniList(tt.in, tt.manga)
			} else {
				got, ok = FromMAL(tt.in, tt.manga)
			}
			if ok != tt.wantSupported || got != tt.want {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantSupported)
			}
		})
	}
}
