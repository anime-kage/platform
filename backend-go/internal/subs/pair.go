package subs

// Pairing a translated subtitle against its source track.

import "strings"

// pairMinOverlap is the share of the shorter line that must be covered before
// two lines count as the same moment.
//
// Not zero: subtitle lines routinely abut to the millisecond, and a plain
// "overlap > 0" test attaches every line to its neighbour as well as itself.
// A quarter is loose enough to survive the small timing differences between a
// translated file and the track it was translated from, and tight enough that a
// line which merely touches at the boundary is not pulled in.
const pairMinOverlap = 0.25

// PairByTime returns, for each row of ro, the source text that occupies the
// same moment in en. The result is parallel to ro; a row with no counterpart
// gets "".
//
// Pairing was originally by row index, which required the two files to have
// exactly the same number of lines and silently discarded the entire English
// column when they did not. That is a bad assumption to hang the feature on: a
// translator who splits one long line in two, merges two short ones, or adds a
// sign or song line changes the count without changing the timing. Release 11
// lost all 315 of its English lines that way against a 450 line translation.
//
// Time is the thing the two files genuinely agree on, so match on that. One
// English line covering two translated lines is attached to both, which is
// exactly what an editor comparing a split line wants to see.
func PairByTime(ro, en []Event) []string {
	out := make([]string, len(ro))
	if len(en) == 0 {
		return out
	}

	// Both sides arrive in start order, so a single advancing cursor is enough:
	// j only ever moves forward past lines that can no longer reach the current
	// row. Restarting the scan per row would be O(n*m) on files that routinely
	// carry a thousand lines each.
	j := 0
	for i, r := range ro {
		for j < len(en) && en[j].EndMs <= r.StartMs {
			j++
		}
		var parts []string
		for k := j; k < len(en) && en[k].StartMs < r.EndMs; k++ {
			if overlaps(r, en[k]) {
				if t := strings.TrimSpace(en[k].Text); t != "" {
					parts = append(parts, t)
				}
			}
		}
		out[i] = strings.Join(parts, " ")
	}
	return out
}

// overlaps reports whether two lines share enough time to be the same moment.
func overlaps(a, b Event) bool {
	start := max(a.StartMs, b.StartMs)
	end := min(a.EndMs, b.EndMs)
	shared := end - start
	if shared <= 0 {
		return false
	}
	shortest := min(a.EndMs-a.StartMs, b.EndMs-b.StartMs)
	if shortest <= 0 {
		return false
	}
	return float64(shared)/float64(shortest) >= pairMinOverlap
}
