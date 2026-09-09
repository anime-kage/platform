package subs

import "testing"

func ev(idx, start, end int, text string) Event {
	return Event{Idx: idx, StartMs: start, EndMs: end, Text: text}
}

// The case that started this: the counts differ, which used to throw the whole
// English column away.
func TestPairByTimeSurvivesDifferentCounts(t *testing.T) {
	en := []Event{
		ev(0, 1000, 5000, "I have been waiting for you."),
		ev(1, 6000, 8000, "Then let us begin."),
	}
	// The translator split the first line in two and kept the second whole.
	ro := []Event{
		ev(0, 1000, 3000, "Te-am așteptat."),
		ev(1, 3000, 5000, "De mult timp."),
		ev(2, 6000, 8000, "Atunci să începem."),
	}
	got := PairByTime(ro, en)
	if got[0] != "I have been waiting for you." || got[1] != "I have been waiting for you." {
		t.Errorf("a split line should carry the source onto both halves, got %q and %q", got[0], got[1])
	}
	if got[2] != "Then let us begin." {
		t.Errorf("row 2 = %q", got[2])
	}
}

// A merge is the same problem in reverse.
func TestPairByTimeMergedLine(t *testing.T) {
	en := []Event{
		ev(0, 1000, 3000, "Stop."),
		ev(1, 3000, 5000, "It is dangerous."),
	}
	ro := []Event{ev(0, 1000, 5000, "Oprește-te, e periculos.")}
	if got := PairByTime(ro, en); got[0] != "Stop. It is dangerous." {
		t.Errorf("both source lines should be joined, got %q", got[0])
	}
}

// A translated line with nothing under it must stay empty rather than borrow
// its neighbour's text.
func TestPairByTimeNoCounterpart(t *testing.T) {
	en := []Event{ev(0, 1000, 2000, "Hello.")}
	ro := []Event{
		ev(0, 1000, 2000, "Salut."),
		ev(1, 50000, 52000, "Semn: Konoha"), // a sign the source never had
	}
	got := PairByTime(ro, en)
	if got[0] != "Hello." {
		t.Errorf("row 0 = %q", got[0])
	}
	if got[1] != "" {
		t.Errorf("a line with no source must stay empty, got %q", got[1])
	}
}

// Lines that merely abut must not pair, or every row picks up its neighbour.
func TestPairByTimeAbuttingLinesDoNotPair(t *testing.T) {
	en := []Event{ev(0, 1000, 2000, "First."), ev(1, 2000, 3000, "Second.")}
	ro := []Event{ev(0, 2000, 3000, "Al doilea.")}
	if got := PairByTime(ro, en); got[0] != "Second." {
		t.Errorf("touching at the boundary must not pair, got %q", got[0])
	}
}

func TestPairByTimeEmptySource(t *testing.T) {
	ro := []Event{ev(0, 1000, 2000, "Salut.")}
	if got := PairByTime(ro, nil); len(got) != 1 || got[0] != "" {
		t.Errorf("no source should give one empty string, got %v", got)
	}
}
