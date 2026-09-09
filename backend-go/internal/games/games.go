// Package games holds the arcade's rules: the progression curve, the factions
// and their rank ladders, and what a solved puzzle is worth.
//
// Nothing in here knows about HTTP or Discord. Both front-ends call into this
// package, so a rule change lands in the website and the bot at the same time
// rather than being implemented twice and drifting.
package games

import "math"

// XPForLevel is the TOTAL experience needed to have reached a level.
//
// 100*L^1.5 is deliberately gentle and deliberately published: level 10 is
// ~3.2k, level 50 ~35k, level 100 ~100k. Against roughly 250-400 XP a day from
// the dailies that puts level 100 near a year of steady play, which is the
// right shape for a track with no ceiling.
func XPForLevel(level int) int64 {
	if level <= 1 {
		return 0
	}
	return int64(100 * math.Pow(float64(level), 1.5))
}

// LevelForXP inverts the curve. There is no maximum level by design; the rank
// ladders simply stop naming new titles past their last entry.
func LevelForXP(xp int64) int {
	if xp <= 0 {
		return 1
	}
	lvl := int(math.Pow(float64(xp)/100.0, 1.0/1.5))
	if lvl < 1 {
		lvl = 1
	}
	// Guard against float drift at the boundaries rather than trusting Pow.
	for XPForLevel(lvl+1) <= xp {
		lvl++
	}
	for lvl > 1 && XPForLevel(lvl) > xp {
		lvl--
	}
	return lvl
}

// Progress reports how far through the current level a total sits, for the bar.
func Progress(xp int64) (level int, into int64, span int64) {
	level = LevelForXP(xp)
	base := XPForLevel(level)
	next := XPForLevel(level + 1)
	return level, xp - base, next - base
}

// Rank is one rung on a faction's ladder.
type Rank struct {
	MinLevel int    `json:"minLevel"`
	Title    string `json:"title"`
}

// Faction is a side a player picks once. It carries identity and a rank ladder
// and nothing else -- no bonuses, no combat modifiers. Keeping factions purely
// about who you are is what stops them multiplying with classes later into a
// balance matrix (see docs/GAMES-ARCHITECTURE.md §13).
type Faction struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Ranks []Rank `json:"ranks"`
}

// Factions are ordered for display. Ladders are data, not code: adding a
// faction is an entry here and nothing else.
var Factions = []Faction{
	// Ladders as specified in Logo/Ranguri_Factiuni_Anime.md. Same seven rungs
	// everywhere so no faction is a faster track than another; only the titles
	// differ, because the ladder is identity, not advantage.
	{Code: "onepiece", Name: "One Piece", Emoji: "🏴‍☠️", Ranks: []Rank{
		{1, "Marinar"}, {10, "Pirat"}, {20, "Ofițer"}, {35, "Căpitan"},
		{50, "Lord al Mării"}, {70, "Împărat al Mării"}, {99, "Regele Piraților"},
	}},
	{Code: "naruto", Name: "Naruto", Emoji: "🍥", Ranks: []Rank{
		{1, "Elev la Academie"}, {10, "Genin"}, {20, "Chunin"}, {35, "Jonin"},
		{50, "Membru ANBU"}, {70, "Sannin Legendar"}, {99, "Hokage"},
	}},
	{Code: "frieren", Name: "Frieren", Emoji: "🪄", Ranks: []Rank{
		{1, "Ucenic"}, {10, "Aventurier"}, {20, "Mag de Clasa a 3-a"},
		{35, "Mag de Clasa a 2-a"}, {50, "Mag de Clasa 1"}, {70, "Mag de Elită"},
		{99, "Mag Mitic"},
	}},
	{Code: "jjk", Name: "Jujutsu Kaisen", Emoji: "👁", Ranks: []Rank{
		{1, "Gradul 4"}, {10, "Gradul 3"}, {20, "Gradul 2"}, {35, "Gradul 1"},
		{50, "Gradul 1 Suprem"}, {70, "Gradul Special"}, {99, "Cel Mai Puternic"},
	}},
	{Code: "bleach", Name: "Bleach", Emoji: "⚔️", Ranks: []Rank{
		{1, "Elev la Academie"}, {10, "Shinigami"}, {20, "Ofițer"}, {35, "Locotenent"},
		{50, "Căpitan"}, {70, "Garda Regală"}, {99, "Regele Spiritelor"},
	}},
	{Code: "gintama", Name: "Gintama", Emoji: "🍬", Ranks: []Rank{
		{1, "Madao"}, {10, "Ucenic Samurai"}, {20, "Yorozuya"}, {35, "Patriot"},
		{50, "Membru Shinsengumi"}, {70, "Căpitan"}, {99, "Suflet de Argint"},
	}},
	{Code: "aot", Name: "Attack on Titan", Emoji: "🗡", Ranks: []Rank{
		{1, "Recrut"}, {10, "Cadet"}, {20, "Soldat"}, {35, "Ofițer de Echipă"},
		{50, "Cercetaș de Elită"}, {70, "Comandant"}, {99, "Titanul Fondator"},
	}},
	{Code: "hxh", Name: "Hunter x Hunter", Emoji: "🃏", Ranks: []Rank{
		{1, "Candidat la Examen"}, {10, "Hunter Licențiat"}, {20, "Utilizator Nen"},
		{35, "Hunter cu o Stea"}, {50, "Hunter cu Două Stele"}, {70, "Zodiac"},
		{99, "Președintele Asociației"},
	}},
	{Code: "demonslayer", Name: "Demon Slayer", Emoji: "🌊", Ranks: []Rank{
		{1, "Ucenic"}, {10, "Mizunoto"}, {20, "Kanoe"}, {35, "Kinoe"},
		{50, "Tsuguko"}, {70, "Hashira"}, {99, "Vânător Legendar"},
	}},
	{Code: "blackclover", Name: "Black Clover", Emoji: "🍀", Ranks: []Rank{
		{1, "Fără Magie"}, {10, "Cavaler Magic Junior"}, {20, "Cavaler Magic Intermediar"},
		{35, "Cavaler Magic Superior"}, {50, "Mare Cavaler Magic"},
		{70, "Căpitan de Brigadă"}, {99, "Regele Vrăjitor"},
	}},
	{Code: "codegeass", Name: "Code Geass", Emoji: "♟", Ranks: []Rank{
		{1, "Elev la Ashford"}, {10, "Soldat"}, {20, "Pilot Knightmare"},
		{35, "Lider de Escadrilă"}, {50, "Cavaler al Mesei Rotunde"},
		{70, "Zero"}, {99, "Împărat"},
	}},
	{Code: "fma", Name: "Fullmetal Alchemist", Emoji: "⚗️", Ranks: []Rank{
		{1, "Ucenic Alchimist"}, {10, "Soldat"}, {20, "Ofițer"}, {35, "Maior"},
		{50, "Alchimist de Stat"}, {70, "General"}, {99, "Führer"},
	}},
}

// FactionByCode returns the faction, or nil when the code is unknown or empty.
func FactionByCode(code string) *Faction {
	for i := range Factions {
		if Factions[i].Code == code {
			return &Factions[i]
		}
	}
	return nil
}

// RankFor names the highest rung a level has reached. Levels past the last rung
// keep its title -- the ladder runs out, the track does not.
func RankFor(code string, level int) string {
	f := FactionByCode(code)
	if f == nil {
		return ""
	}
	title := ""
	for _, r := range f.Ranks {
		if level >= r.MinLevel {
			title = r.Title
		}
	}
	return title
}

// BadgeDef describes something a player can earn. The catalogue ships to the
// client so the arcade can show what is still unearned -- a badge case with
// empty slots is a to-do list; one that only shows what you already have is a
// receipt.
type BadgeDef struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	Icon string `json:"icon"`
}

// BadgeCatalogue is every badge the arcade can currently award. Each one is
// checkable from data that already exists -- nothing here needs a feature that
// has not shipped.
var BadgeCatalogue = []BadgeDef{
	// Deliberately demanding. The first pass had a badge for solving a single
	// puzzle and one for 500 gold -- both arrive in the first session, which
	// makes the case a participation receipt rather than something to chase.
	// Everything here takes either real skill or real persistence.
	{"one_guess", "Din prima", "Ghicește o serie din prima încercare.", "⊙"},
	{"first_try_5", "Ochi format", "Ghicește din prima de cinci ori.", "◎"},
	{"clutch", "La limită", "Rezolvă cu ultima încercare.", "⧗"},
	{"perfect_day", "Zi completă", "Rezolvă toate puzzle-urile unei zile.", "◈"},
	{"week_title", "Săptămână de titluri", "Rezolvă toate titlurile săptămânii.", "❖"},
	{"week_poster", "Săptămână de postere", "Rezolvă toate posterele săptămânii.", "❉"},
	{"week_theme", "Săptămână muzicală", "Rezolvă toate opening-urile și ending-urile săptămânii.", "♪"},
	{"week_character", "Săptămână de personaje", "Rezolvă toate personajele săptămânii.", "☻"},
	{"week_groups", "Săptămână de grupe", "Rezolvă toate grupele săptămânii.", "◫"},
	{"perfect_week", "Săptămână perfectă", "Rezolvă tot ce a apărut într-o săptămână.", "✺"},
	{"no_hint_week", "Fără ajutor", "O săptămână de titluri fără nicio descriere.", "⊘"},
	{"gold_5000", "Cinci mii", "Adună 5.000 de gold.", "◉"},
	{"level_25", "Nivel 25", "Ajungi la nivelul 25 într-o facțiune.", "★"},
	{"level_50", "Nivel 50", "Ajungi la nivelul 50 într-o facțiune.", "✪"},
	{"level_100", "Nivel 100", "Ajungi la nivelul 100 într-o facțiune.", "✵"},
}

// BadgesWithArt are the badges that have a painted icon under
// frontend/static/arcade/icons. The site can do without this list because it
// falls back to the glyph when the image fails to load, but Discord cannot: an
// embed image that 404s is simply dropped and the player gets a profile with no
// art at all. So the bot consults this and shows the newest badge that actually
// has a picture. TestCatalogueArtMatchesDisk keeps it in step with the files.
var BadgesWithArt = map[string]bool{
	"one_guess": true, "first_try_5": true, "clutch": true, "perfect_day": true,
	"week_title": true, "week_poster": true, "perfect_week": true,
	"no_hint_week": true, "gold_5000": true,
	"level_25": true, "level_50": true, "level_100": true,
}

// ── Daily chest ─────────────────────────────────────────────────────────────

// Chest rewards scale with the streak and then stop. The cap matters: without
// one, a player who never misses eventually earns more from opening a box than
// from playing, and the puzzles become the thing you skip.
const (
	ChestBaseGold   = 60
	ChestStreakStep = 20
	ChestMaxGold    = 200
)

// ChestReward is what today's chest pays at a given streak length.
func ChestReward(streak int) int {
	if streak < 1 {
		streak = 1
	}
	g := ChestBaseGold + ChestStreakStep*(streak-1)
	if g > ChestMaxGold {
		return ChestMaxGold
	}
	return g
}
