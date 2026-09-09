package games

import "testing"

var board = [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10, 11, 12}}

// Order within a submission is whatever the player clicked in, so a group has
// to be recognised as a set rather than a sequence.
func TestOrderWithinAGuessDoesNotMatter(t *testing.T) {
	found, miss := GroupProgress([][]int{{3, 1, 2}}, board)
	if len(found) != 1 || found[0] != 0 || miss != 0 {
		t.Errorf("got found=%v mistakes=%d, want the first group and no mistakes", found, miss)
	}
}

func TestWrongSetCountsAsAMistake(t *testing.T) {
	_, miss := GroupProgress([][]int{{1, 2, 4}}, board)
	if miss != 1 {
		t.Errorf("got %d mistakes, want 1", miss)
	}
}

// Re-sending a group already found is a double click, not a fifth mistake.
func TestResendingAFoundGroupIsNotAMistake(t *testing.T) {
	found, miss := GroupProgress([][]int{{1, 2, 3}, {1, 2, 3}}, board)
	if len(found) != 1 {
		t.Errorf("got %d groups found, want 1", len(found))
	}
	if miss != 0 {
		t.Errorf("got %d mistakes, want 0", miss)
	}
}

func TestClearingTheBoardEndsTheRound(t *testing.T) {
	found, miss := GroupProgress([][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10, 11, 12}}, board)
	if len(found) != GroupCount || miss != 0 {
		t.Fatalf("got found=%d mistakes=%d", len(found), miss)
	}
	if !GroupRoundOver(len(found), miss) {
		t.Error("a cleared board should end the round")
	}
}

func TestRunningOutOfMistakesEndsTheRound(t *testing.T) {
	if !GroupRoundOver(0, MaxGroupMistakes) {
		t.Error("the round should end once the mistakes are spent")
	}
	if GroupRoundOver(1, MaxGroupMistakes-1) {
		t.Error("the round should still be live with a mistake left")
	}
}

// Paying for three of four is the point of scoring per group.
func TestPartialBoardsStillPay(t *testing.T) {
	xp3, gold3 := ScoreGroups(3)
	if xp3 == 0 || gold3 == 0 {
		t.Error("three groups should pay something")
	}
	xp4, gold4 := ScoreGroups(4)
	if xp4 <= xp3 || gold4 <= gold3 {
		t.Error("clearing the board should beat three groups")
	}
	if xp, gold := ScoreGroups(0); xp != 0 || gold != 0 {
		t.Errorf("an empty board should pay nothing, got %d/%d", xp, gold)
	}
}

// Three of four is the answer worth telling a player about: the idea was right
// and one title was wrong.
func TestNearMissCountsTheBestOverlap(t *testing.T) {
	if got := GroupNearMiss([]int{1, 2, 4}, board); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := GroupNearMiss([]int{1, 2, 3}, board); got != GroupSize {
		t.Errorf("an exact group should report %d, got %d", GroupSize, got)
	}
	if got := GroupNearMiss([]int{1, 4, 7}, board); got != 1 {
		t.Errorf("one from each group should report 1, got %d", got)
	}
}
