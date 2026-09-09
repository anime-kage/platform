package games

import "testing"

// TestEscapeReward: showing up to a fight the channel loses has to be worth
// something, but never as much per hit as finishing it.
func TestEscapeReward(t *testing.T) {
	// The Kurama that started this: 83 damage of 3900, 1000 XP in the pot.
	if got := EscapeReward(1000, 83, 3900); got != 10 {
		t.Errorf("Crefi's share of the escaped Kurama = %d, want 10", got)
	}

	// Scaled by full health, not by damage dealt: a channel that removed 4%
	// must not be paid like one that removed 99%.
	small := EscapeReward(1000, 83, 3900)
	if big := EscapeReward(1000, 3800, 3900); big <= small*10 {
		t.Errorf("nearly finishing (%d) should pay far more than barely trying (%d)", big, small)
	}

	// Finishing must beat escaping at the same damage, or the channel is
	// indifferent between killing a monster and letting it walk.
	dmg, hp, pot := 1950, 3900, 1000
	kill := pot * dmg / dmg // sole killer takes the whole pot
	if EscapeReward(pot, dmg, hp) >= kill {
		t.Error("escaping pays as well as killing")
	}

	// Degenerate inputs must not divide by zero or pay for nothing.
	if EscapeReward(1000, 0, 3900) != 0 || EscapeReward(1000, 50, 0) != 0 {
		t.Error("no damage or no health should pay nothing")
	}
}
