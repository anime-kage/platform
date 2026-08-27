package slug

import "testing"

// Every case here is a real title from the catalog, because the awkward ones are
// all real: titles that run words together, titles with ordinals, titles with a
// star in the middle.
func TestMake(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		// the case that prompted this: no space in the title
		{"91Days", "91-days"},
		// …but an ordinal must not be split the same way
		{"Sousou no Frieren 2nd Season", "sousou-no-frieren-2nd-season"},
		{"Grand Blue Season 3", "grand-blue-season-3"},
		{"Kimi no Koto ga Daidaidaidaidaisuki na 100-nin no Kanojo 3rd Season",
			"kimi-no-koto-ga-daidaidaidaidaisuki-na-100-nin-no-kanojo-3rd-season"},
		{"Youjo Senki II", "youjo-senki-ii"},

		// an all-caps run stays one word
		{"Hagane no Renkinjutsushi: FULLMETAL ALCHEMIST",
			"hagane-no-renkinjutsushi-fullmetal-alchemist"},

		// punctuation, symbols and trailing dots collapse away
		{"Kimi no Na wa.", "kimi-no-na-wa"},
		{"Bleach: Sennen Kessen-hen - Kashin-tan", "bleach-sennen-kessen-hen-kashin-tan"},
		{"Mahou Shoujo Madoka★Magica Movie 4: Walpurgis no Kaiten",
			"mahou-shoujo-madoka-magica-movie-4-walpurgis-no-kaiten"},
		{"Toumei na Yoru ni Kakeru Kimi to, Me ni Mienai Koi wo Shita.",
			"toumei-na-yoru-ni-kakeru-kimi-to-me-ni-mienai-koi-wo-shita"},

		// diacritics fold to ASCII
		{"Otome Kaijuu Caraméliser", "otome-kaijuu-carameliser"},
		{"Împăratul Șoarecilor", "imparatul-soarecilor"},

		// camelCase is deliberately NOT split: these are the cases that make the
		// rule a net loss on this catalog
		{"JoJo no Kimyou na Bouken", "jojo-no-kimyou-na-bouken"},
		{"ReLIFE", "relife"},
		{"XxxHOLiC", "xxxholic"},
		// but the digit seam still fires, which is all "91Days" needed
		{"5Toubun no Hanayome", "5-toubun-no-hanayome"},

		// degenerate input
		{"", ""},
		{"★★★", ""},
		{"   ", ""},
		{"---", ""},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := Make(tc.in); got != tc.want {
				t.Errorf("Make(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A slug must be stable: generating it twice, or generating it from a slug,
// gives the same answer. Otherwise a re-run of the backfill would churn URLs.
func TestMakeIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"91Days", "Sousou no Frieren 2nd Season", "Kimi no Na wa.", "ReLIFE",
	} {
		once := Make(in)
		if twice := Make(once); twice != once {
			t.Errorf("Make(%q) = %q, but Make(%q) = %q", in, once, once, twice)
		}
	}
}
