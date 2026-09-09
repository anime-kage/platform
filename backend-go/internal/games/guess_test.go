package games

import "testing"

func TestGuessMatches(t *testing.T) {
	cases := []struct {
		guess, name string
		want        bool
	}{
		{"Monkey D. Luffy", "Monkey D. Luffy", true},
		{"luffy", "Monkey D. Luffy", true},
		{"monkey luffy", "Monkey D. Luffy", true},
		{"MONKEY   luffy", "Monkey D. Luffy", true},
		{"monkey", "Monkey D. Luffy", true},
		{"d", "Monkey D. Luffy", false},   // too short to stand alone
		{"zoro", "Monkey D. Luffy", false},
		{"rem", "Rem", true},              // short, but it is the whole name
		{"ubel", "Übel", true},            // diacritics folded
		{"Übel", "Ubel", true},
		{"frieren", "Frieren", true},
		{"fern", "Frieren", false},
		{"", "Frieren", false},
		{"luffy monkey zoro", "Monkey D. Luffy", false}, // a word that is not his
	}
	for _, c := range cases {
		if got := GuessMatches(c.guess, c.name); got != c.want {
			t.Errorf("GuessMatches(%q, %q) = %v, want %v", c.guess, c.name, got, c.want)
		}
	}
}

func TestGuessNear(t *testing.T) {
	cases := []struct {
		guess, name string
		want        bool
	}{
		{"wuffy", "Monkey D. Luffy", true},      // one letter out
		{"lufy", "Monkey D. Luffy", true},       // a dropped letter
		{"monkeu", "Monkey D. Luffy", true},
		{"zoro", "Monkey D. Luffy", false},      // a different character
		{"biscut", "Biscuit Krueger", true},
		{"kruger", "Biscuit Krueger", true},
		{"naruto", "Biscuit Krueger", false},
		{"frieren", "Frieren", false},           // exact, so it is a win not a near miss
	}
	for _, c := range cases {
		if got := GuessNear(c.guess, c.name); got != c.want {
			t.Errorf("GuessNear(%q, %q) = %v, want %v", c.guess, c.name, got, c.want)
		}
	}
}
