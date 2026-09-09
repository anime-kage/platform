package handler

// Arcade endpoints.
//
// The one rule worth stating: a puzzle's answer is only ever put in a response
// once that player has finished with it -- solved, out of guesses, or given up.
// Everything else the board returns is a clue.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/games"
	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

// puzzleView is what the browser is allowed to see.
// groupTile is one of the sixteen cards on a Grupe board.
// tileSeed orders the board deterministically per puzzle, so the tiles keep
// their places between loads instead of moving under the player.
func tileSeed(puzzleID int64, animeID int) uint64 {
	h := uint64(puzzleID)*1469598103934665603 ^ uint64(animeID)
	h *= 1099511628211
	return h ^ (h >> 29)
}

type groupTile struct {
	AnimeID int    `json:"animeId"`
	Title   string `json:"title"`
	Image   string `json:"image,omitempty"`
}

// foundGroup is a group the player has already picked out, or one revealed
// when the round ended.
type foundGroup struct {
	Label    string `json:"label"`
	AnimeIDs []int  `json:"animeIds"`
	// The raw fact, so the browser writes the label with the catalogue's own
	// genre helper rather than showing whatever English the database holds.
	Kind  string `json:"kind"`
	Value string `json:"value"`
	// Solved separates a group the player picked out from one revealed because
	// the round ended. Without it a lost board reads exactly like a won one.
	Solved bool `json:"solved"`
}

type puzzleView struct {
	ID       int64  `json:"id"`
	Mode     string `json:"mode"`
	PlayDate string `json:"playDate"`
	IsToday  bool   `json:"isToday"`

	Solved    bool     `json:"solved"`
	GaveUp    bool     `json:"gaveUp"`
	Finished  bool     `json:"finished"`
	Remaining int      `json:"remaining"`
	Stage     int      `json:"stage"`
	AwardedXp int      `json:"awardedXp"`
	HintReady bool     `json:"hintReady"`
	HintUsed  bool     `json:"hintUsed"`
	// Only sent once bought, and only for the mode that offers it.
	Hint string `json:"hint,omitempty"`
	// Theme mode. Audio plays while guessing; the video is the paid final clue
	// and the reveal — a single frame of it usually names the series outright.
	Audio     string `json:"audio,omitempty"`
	Video     string `json:"video,omitempty"`
	ThemeSlug string `json:"themeSlug,omitempty"`
	ThemeSong string `json:"themeSong,omitempty"`
	// Grupe: sixteen tiles in a fixed shuffled order, plus the groups already
	// found. A group's label and membership only appear once it is found --
	// before that, the grouping is the answer.
	Tiles     []groupTile  `json:"tiles,omitempty"`
	Found     []foundGroup `json:"found,omitempty"`
	Mistakes  int          `json:"mistakes"`
	MaxMistakes int        `json:"maxMistakes,omitempty"`
	// Character round: five portraits, no names until it is scored.
	Chars   []charSlot `json:"chars,omitempty"`
	Scored  bool       `json:"scored"`
	CharXp  int        `json:"charXp,omitempty"`
	CharGold int       `json:"charGold,omitempty"`

	// Clues, revealed one per wrong guess. Never contains the title.
	Clues []clue `json:"clues"`
	// The comparison grid: one row per guess, plus what those rows have proved.
	Grid    []games.GuessRow    `json:"grid"`
	Summary []games.SummaryCell `json:"summary"`
	// Poster mode only: always sent, because the reveal is a CSS filter over
	// the same image. Stage caps how much is uncovered.
	Image string `json:"image,omitempty"`

	// Only populated once Finished.
	Answer *answerView `json:"answer,omitempty"`
}

// charSlot is one portrait. name/series are empty until the round is scored.
type charSlot struct {
	ID      int    `json:"id"`
	Image   string `json:"image"`
	Name    string `json:"name,omitempty"`
	Series  string `json:"series,omitempty"`
	// what the player submitted, echoed back after scoring
	GaveName   bool `json:"gaveName"`
	GaveSeries bool `json:"gaveSeries"`
}

type charAnswer struct {
	CharID  int `json:"charId"`
	AnimeID int `json:"animeId"`
}

type clue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type answerView struct {
	Title string `json:"title"`
	Image string `json:"image,omitempty"`
	Slug  string `json:"slug,omitempty"`
	Year  int    `json:"year,omitempty"`
}

// Clue labels come from the catalogue in English and lowercase. "tv" in
// particular is meaningless to a player: the whole board is anime, so the clue
// that matters is the FORM — a series, a film, an OVA.
var typeLabel = map[string]string{
	"tv": "Anime", "movie": "Film", "ova": "OVA", "special": "Special", "ona": "ONA",
}

// Genres and seasons are sent RAW. The frontend already owns the canonical
// labels in lib/types.ts -- genreRo() and seasonRo() -- and that map encodes a
// deliberate rule: only translate what Romanian genuinely has a word for, so
// "Slice of Life" and "Isekai" stay in English on purpose. Translating them
// here would fork that decision and drift from every other page.

func label(m map[string]string, v string) string {
	if out, ok := m[v]; ok {
		return out
	}
	return v
}

type profileView struct {
	Username string   `json:"username"`
	Level    int      `json:"level"`
	Xp       int64    `json:"xp"`
	XpInto   int64    `json:"xpInto"`
	XpSpan   int64    `json:"xpSpan"`
	Gold     int64    `json:"gold"`
	Faction  string   `json:"faction,omitempty"`
	Rank     string   `json:"rank,omitempty"`
	Badges   []string `json:"badges"`
	// Earned before choosing a faction, and claimed by choosing one.
	PendingXp int64 `json:"pendingXp,omitempty"`
	// Experience held in every faction the player has touched, so the picker
	// can show what switching would return them to.
	Progress map[string]int64 `json:"progress"`
}

// synopsisOf prefers the Romanian description, matching displaySynopsis() in
// lib/types.ts. Falling through to English is the fallback, not the default.
func synopsisOf(p *repo.PuzzleRow) string {
	if p.SynopsisRo != nil && strings.TrimSpace(*p.SynopsisRo) != "" {
		return *p.SynopsisRo
	}
	if p.Synopsis != nil {
		return *p.Synopsis
	}
	return ""
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// displayTitle mirrors what the catalogue shows (displayName in types.ts):
// Romanian where we have it, then English, and only then the original
// spelling. Guessing still accepts all three -- see acceptedTitles -- so
// preferring English here changes what is shown, never what is allowed.
func displayTitle(title string, ro, en *string) string {
	if ro != nil && *ro != "" {
		return *ro
	}
	if en != nil && *en != "" {
		return *en
	}
	return title
}

// acceptedTitles is every spelling that should count as naming this series.
func acceptedTitles(p *repo.PuzzleRow) []string {
	return []string{p.Title, str(p.TitleEn), str(p.TitleRo)}
}

// clueSet is ordered least to most telling, so revealing them in order is a
// difficulty ramp rather than a random dribble of facts.
func clueSet(p *repo.PuzzleRow) []clue {
	out := []clue{}
	add := func(l, v string) {
		if v != "" && v != "0" {
			out = append(out, clue{l, v})
		}
	}
	if p.Year != nil {
		add("An", strconv.Itoa(*p.Year))
	}
	add("Tip", label(typeLabel, strings.ToLower(str(p.Type))))
	if p.Episodes != nil && *p.Episodes > 0 {
		add("Episoade", strconv.Itoa(*p.Episodes))
	}
	if p.Score != nil && *p.Score > 0 {
		add("Scor", strconv.FormatFloat(*p.Score, 'f', 2, 64))
	}
	add("Studio", first(p.Studios))
	add("Gen", first(p.Genres))
	return out
}

// first takes the leading entry of a text[] column, so a clue names one studio
// rather than listing every company that touched the production.
func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return strings.TrimSpace(v[0])
}

// Guesses are stored as anime ids: a guess is a series, not a string, and the
// grid needs the row behind it. Older attempts held plain titles, so those are
// read and discarded rather than failing the whole attempt.
func decodeGuesses(raw *string) []int {
	if raw == nil || *raw == "" {
		return []int{}
	}
	var ids []int
	if err := json.Unmarshal([]byte(*raw), &ids); err == nil {
		return ids
	}
	return []int{}
}

// cardOf converts a stored row into the shape the comparison engine wants.
func cardOf(c *repo.CardRow) games.Card {
	card := games.Card{
		ID: c.ID, Title: displayTitle(c.Title, c.TitleRo, c.TitleEn), Year: c.Year, Season: c.Season,
		Type: c.Type, Episodes: c.Episodes, Score: c.Score,
		Studios: c.Studios, Genres: c.Genres,
	}
	if c.Image != nil {
		card.Image = *c.Image
	}
	return card
}

func genreLabelFn(v string) string { return v }
func typeLabelFn(v string) string  { return label(typeLabel, strings.ToLower(v)) }

// fillGroupTiles puts titles and art on the tiles toView could only number.
// Done in one query for every board on the page rather than per puzzle.
func (h *Handler) fillGroupTiles(ctx context.Context, views []puzzleView) {
	ids := []int{}
	for i := range views {
		for _, t := range views[i].Tiles {
			ids = append(ids, t.AnimeID)
		}
	}
	if len(ids) == 0 {
		return
	}
	tiles, err := h.repo.GroupTiles(ctx, ids)
	if err != nil {
		slog.Warn("could not load the grouping board tiles", "err", err)
		return
	}
	byID := map[int]repo.GroupTile{}
	for _, t := range tiles {
		byID[t.AnimeID] = t
	}
	for i := range views {
		for j, t := range views[i].Tiles {
			if got, ok := byID[t.AnimeID]; ok {
				views[i].Tiles[j].Title = got.Title
				views[i].Tiles[j].Image = str(got.Image)
			}
		}
	}
}

func toView(p *repo.PuzzleRow, today time.Time) puzzleView {
	guesses := decodeGuesses(p.Guesses)
	solved := p.SolvedAt != nil
	gaveUp := p.GaveUp != nil && *p.GaveUp
	finished := solved || gaveUp || len(guesses) >= games.MaxGuesses

	v := puzzleView{
		// Always an array, never null: a mode without clues still has to be
		// safe for the client to read .length on.
		Clues:     []clue{},
		ID:        p.ID,
		Mode:      p.Mode,
		PlayDate:  p.PlayDate.Format("2006-01-02"),
		IsToday:   p.PlayDate.Year() == today.Year() && p.PlayDate.YearDay() == today.YearDay(),
		Solved:    solved,
		GaveUp:    gaveUp,
		Finished:  finished,
		Remaining: games.MaxGuesses - len(guesses),
		Stage:     games.Stage(len(guesses)),
	}
	if p.AwardedXp != nil {
		v.AwardedXp = *p.AwardedXp
	}
	v.Grid = []games.GuessRow{}
	v.Summary = []games.SummaryCell{}
	v.HintUsed = p.UsedHint != nil && *p.UsedHint
	switch p.Mode {
	case games.ModeTitle:
		// The poster mode already reveals itself a step at a time, so it has a
		// built-in equivalent and is not offered a paid clue.
		v.HintReady = games.HintAvailable(len(guesses), finished, v.HintUsed)
		if v.HintUsed || finished {
			v.Hint = synopsisOf(p)
		}
	case games.ModeGroups:
		v.MaxMistakes = games.MaxGroupMistakes
		sets := repo.ParseGroups(p.Groups)
		ids := make([]int, 0, games.GroupTiles)
		answer := make([][]int, 0, len(sets))
		for _, g := range sets {
			ids = append(ids, g.AnimeIDs...)
			answer = append(answer, g.AnimeIDs)
		}
		// Only the ids and their order are decided here, because toView has no
		// database. fillGroupTiles puts the titles on afterwards. The order is
		// stable per puzzle so the board does not move under the player between
		// guesses.
		order := append([]int(nil), ids...)
		sort.Slice(order, func(i, j int) bool {
			return tileSeed(p.ID, order[i]) < tileSeed(p.ID, order[j])
		})
		for _, id := range order {
			v.Tiles = append(v.Tiles, groupTile{AnimeID: id})
		}
		var subs [][]int
		if p.Guesses != nil && *p.Guesses != "" {
			_ = json.Unmarshal([]byte(*p.Guesses), &subs)
		}
		hits, miss := games.GroupProgress(subs, answer)
		v.Mistakes = miss
		for _, i := range hits {
			v.Found = append(v.Found, foundGroup{Label: sets[i].Label, AnimeIDs: sets[i].AnimeIDs,
				Kind: sets[i].Kind, Value: sets[i].Value, Solved: true})
		}
		// When the round is over the rest is revealed, because a board you can
		// no longer play but cannot see the answer to teaches nothing.
		if finished || games.GroupRoundOver(len(hits), miss) {
			seen := map[int]bool{}
			for _, i := range hits {
				seen[i] = true
			}
			for i, g := range sets {
				if !seen[i] {
					v.Found = append(v.Found, foundGroup{Label: g.Label, AnimeIDs: g.AnimeIDs, Kind: g.Kind, Value: g.Value})
				}
			}
		}
	case games.ModeTheme:
		v.Audio = str(p.ThemeAudio)
		v.ThemeSlug = str(p.ThemeSlug)
		v.HintReady = games.HintAvailable(len(guesses), finished, v.HintUsed) && p.ThemeVideo != nil
		if v.HintUsed || finished {
			v.Video = str(p.ThemeVideo)
		}
		if finished {
			v.ThemeSong = str(p.ThemeSong)
		}
	}
	if v.Remaining < 0 {
		v.Remaining = 0
	}

	switch p.Mode {
	case games.ModePoster:
		v.Image = str(p.ImageURL)
		if finished {
			v.Stage = games.RevealStages
		}
	case games.ModeTitle:
		// One clue up front, then one per wrong guess.
		n := 1 + len(guesses)
		all := clueSet(p)
		if finished || n > len(all) {
			n = len(all)
		}
		v.Clues = all[:n]
	}

	if finished {
		v.Answer = &answerView{Title: displayTitle(p.Title, p.TitleRo, p.TitleEn),
			Image: str(p.ImageURL), Slug: str(p.Slug)}
		if p.Year != nil {
			v.Answer.Year = *p.Year
		}
	}
	return v
}

// gamesBoard is the arcade's single read: who you are, and every puzzle still
// in the window.
func (h *Handler) gamesBoard(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	// Generating here as well as on a schedule keeps a fresh environment (or a
	// missed cron) from showing an empty arcade. It is a no-op once the rows
	// exist, because of the (mode, play_date) unique constraint.
	if _, err := h.repo.EnsurePuzzles(ctx,
		[]string{games.ModeTitle, games.ModePoster, games.ModeTheme, games.ModeCharacter},
		games.HistoryDays); err != nil {
		httpx.Internal(w, "prepare puzzles", err)
		return
	}
	// Grupe is generated apart from the others because picking four groups whose
	// members do not overlap is a search, not a single INSERT ... SELECT. It is
	// allowed to fail quietly: a day with no board is a mode that sits out, not
	// an arcade that will not load.
	if _, err := h.repo.EnsureGroupsPuzzle(ctx, games.HistoryDays); err != nil {
		slog.Warn("could not prepare the grouping rounds", "err", err)
	}

	prof, err := h.repo.EnsureProfile(ctx, u.UserID)
	if err != nil {
		httpx.Internal(w, "load profile", err)
		return
	}
	rows, err := h.repo.Board(ctx, u.UserID, games.HistoryDays)
	if err != nil {
		httpx.Internal(w, "load board", err)
		return
	}
	chest, err := h.repo.Chest(ctx, u.UserID)
	if err != nil {
		httpx.Internal(w, "load chest", err)
		return
	}
	// The button shows what the NEXT open is worth, which is one more day of
	// streak than the player currently holds.
	chest.NextGold = games.ChestReward(chest.Streak + 1)

	badges, err := h.repo.Badges(ctx, u.UserID)
	if err != nil {
		httpx.Internal(w, "load badges", err)
		return
	}

	today := time.Now()
	views := make([]puzzleView, 0, len(rows))
	for i := range rows {
		v := toView(&rows[i], today)
		h.fillGrid(ctx, &rows[i], &v)
		h.fillChars(ctx, &rows[i], &v)
		views = append(views, v)
	}
	// One query for every Grupe board on the page rather than one per board.
	h.fillGroupTiles(ctx, views)

	activeXP, err := h.repo.ActiveXP(ctx, u.UserID)
	if err != nil {
		httpx.Internal(w, "load progress", err)
		return
	}
	perFaction, err := h.repo.FactionProgress(ctx, u.UserID)
	if err != nil {
		httpx.Internal(w, "load faction progress", err)
		return
	}
	progress := map[string]int64{}
	for _, f := range perFaction {
		progress[f.Faction] = f.Xp
	}

	level, into, span := games.Progress(activeXP)
	codes := make([]string, 0, len(badges))
	for _, b := range badges {
		codes = append(codes, b.Code)
	}
	pv := profileView{
		Username: u.Username,
		Level:    level,
		Xp:       activeXP,
		XpInto:   into,
		XpSpan:   span,
		Gold:     prof.Gold,
		Faction:  str(prof.Faction),
		Rank:     games.RankFor(str(prof.Faction), level),
		Badges:   codes,
		Progress: progress,
		PendingXp: prof.PendingXp,
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"profile":  pv,
		"puzzles":  views,
		"factions": games.Factions,
		"badgeCatalogue": games.BadgeCatalogue,
		"columns":        games.Columns,
		"chest":          chest,
		"maxGuesses": games.MaxGuesses,
	}})
}

// gamesGuess takes one guess and returns the puzzle's new state.
func (h *Handler) gamesGuess(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Puzzle invalid")
		return
	}
	var body struct {
		Guess  string `json:"guess"`
		GiveUp bool   `json:"giveUp"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Cerere invalidă")
		return
	}

	p, err := h.repo.PuzzleForUser(ctx, int64(id), u.UserID)
	if err != nil {
		notFoundOr(w, err, "Puzzle-ul nu există", "load puzzle")
		return
	}

	today := time.Now()
	if !games.InWindow(p.PlayDate, today) {
		httpx.Error(w, http.StatusGone, "Acest puzzle nu mai poate fi jucat.")
		return
	}

	guesses := decodeGuesses(p.Guesses)
	solved := p.SolvedAt != nil
	gaveUp := p.GaveUp != nil && *p.GaveUp
	if solved || gaveUp || len(guesses) >= games.MaxGuesses {
		gv := toView(p, today)
		h.fillGrid(ctx, p, &gv)
		httpx.JSON(w, http.StatusOK, map[string]any{"data": gv})
		return
	}

	if body.GiveUp {
		if err := h.repo.SaveAttempt(ctx, p.ID, u.UserID, guesses, false, true, 0, 0); err != nil {
			httpx.Internal(w, "save attempt", err)
			return
		}
		p.GaveUp = &body.GiveUp
		uv := toView(p, today)
		h.fillGrid(ctx, p, &uv)
		httpx.JSON(w, http.StatusOK, map[string]any{"data": uv})
		return
	}

	guess := strings.TrimSpace(body.Guess)
	if guess == "" {
		httpx.Error(w, http.StatusBadRequest, "Scrie un titlu.")
		return
	}
	// A guess names a series, and the grid compares that series against the
	// answer. So it has to resolve to a real row -- an unrecognised string is a
	// typo to correct, not an attempt to spend.
	card, err := h.repo.CardByTitle(ctx, games.NormaliseKey(guess))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound,
				"Nu găsesc seria asta în catalog. Alege una din sugestii.")
			return
		}
		httpx.Internal(w, "resolve guess", err)
		return
	}
	// Guessing the same series twice is a slip, not an attempt -- the same
	// reasoning as an unrecognised title above. It also used to append a second
	// grid row for that series, and the grid is keyed by series, so the repeat
	// crashed the page's rendering outright.
	if slices.Contains(guesses, card.ID) {
		httpx.Error(w, http.StatusConflict, "Ai încercat deja seria asta.")
		return
	}
	guesses = append(guesses, card.ID)
	hit := card.ID == p.AnimeID

	xp, gold := 0, 0
	if hit {
		xp, gold = games.Award(p.PlayDate, today)
		xp, gold = games.ApplyHint(xp, gold, p.UsedHint != nil && *p.UsedHint)
		if err := h.repo.AwardXPGold(ctx, u.UserID, xp, gold,
			"puzzle_solve", strconv.FormatInt(p.ID, 10)); err != nil {
			httpx.Internal(w, "award", err)
			return
		}
	}
	if err := h.repo.SaveAttempt(ctx, p.ID, u.UserID, guesses, hit, false, xp, gold); err != nil {
		httpx.Internal(w, "save attempt", err)
		return
	}
	if hit {
		h.checkPuzzleBadges(ctx, u.UserID, p.Mode, len(guesses))
	}

	// Re-read so the response reflects exactly what was stored.
	fresh, err := h.repo.PuzzleForUser(ctx, p.ID, u.UserID)
	if err != nil {
		httpx.Internal(w, "reload puzzle", err)
		return
	}
	view := toView(fresh, today)
	h.fillGrid(ctx, fresh, &view)
	h.fillChars(ctx, fresh, &view)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": view,
		"award": map[string]int{"xp": xp, "gold": gold},
	})
}

// checkPuzzleBadges awards what the player has just become eligible for.
// Failures are swallowed on purpose: a badge is a garnish, and losing one is
// not a reason to fail the guess that earned it.
func (h *Handler) checkPuzzleBadges(ctx context.Context, userID int, mode string, guesses int) {
	grant := func(code string) { _, _ = h.repo.GrantBadge(ctx, userID, code) }

	if guesses == 1 {
		grant("one_guess")
	}
	if guesses >= games.MaxGuesses {
		grant("clutch")
	}
	if n, err := h.repo.SolvedWithGuesses(ctx, userID, 1); err == nil && n >= 5 {
		grant("first_try_5")
	}
	if s, t, err := h.repo.SolvedToday(ctx, userID); err == nil && t > 0 && s == t {
		grant("perfect_day")
	}
	if s, t, err := h.repo.SolvedInWindow(ctx, userID, mode, games.HistoryDays); err == nil && t > 0 && s == t {
		grant("week_" + mode)
	}
	if s, t, err := h.repo.SolvedWeekAllModes(ctx, userID, games.HistoryDays); err == nil && t > 0 && s == t {
		grant("perfect_week")
	}
	if c, t, err := h.repo.HintFreeWindow(ctx, userID, games.ModeTitle, games.HistoryDays); err == nil && t > 0 && c == t {
		grant("no_hint_week")
	}
	if prof, err := h.repo.EnsureProfile(ctx, userID); err == nil && prof.Gold >= 5000 {
		grant("gold_5000")
	}
	if xp, err := h.repo.ActiveXP(ctx, userID); err == nil {
		lvl := games.LevelForXP(xp)
		for _, m := range []int{25, 50, 100} {
			if lvl >= m {
				grant("level_" + strconv.Itoa(m))
			}
		}
	}
}

// gamesSetFaction also claims any XP banked before a faction was chosen.
func (h *Handler) gamesSetFaction(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	var body struct {
		Faction string `json:"faction"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Cerere invalidă")
		return
	}
	if games.FactionByCode(body.Faction) == nil {
		httpx.Error(w, http.StatusBadRequest, "Facțiune necunoscută")
		return
	}
	if _, err := h.repo.EnsureProfile(r.Context(), u.UserID); err != nil {
		httpx.Internal(w, "load profile", err)
		return
	}
	claimed, err := h.repo.SetFaction(r.Context(), u.UserID, body.Faction)
	if err != nil {
		httpx.Internal(w, "set faction", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Facțiune schimbată", "claimedXp": claimed,
	})
}


// gamesHint buys the synopsis clue. Deliberately its own endpoint rather than a
// flag on a guess: taking the hint is a decision with a price, and it should be
// possible to take it and then think, instead of being forced to guess in the
// same breath.
func (h *Handler) gamesHint(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Puzzle invalid")
		return
	}
	p, err := h.repo.PuzzleForUser(ctx, int64(id), u.UserID)
	if err != nil {
		notFoundOr(w, err, "Puzzle-ul nu există", "load puzzle")
		return
	}
	today := time.Now()
	if !games.InWindow(p.PlayDate, today) {
		httpx.Error(w, http.StatusGone, "Acest puzzle nu mai poate fi jucat.")
		return
	}
	if p.Mode != games.ModeTitle && p.Mode != games.ModeTheme {
		httpx.Error(w, http.StatusBadRequest, "Jocul acesta nu are indiciu suplimentar.")
		return
	}
	guesses := decodeGuesses(p.Guesses)
	finished := p.SolvedAt != nil || (p.GaveUp != nil && *p.GaveUp) || len(guesses) >= games.MaxGuesses
	used := p.UsedHint != nil && *p.UsedHint
	if !games.HintAvailable(len(guesses), finished, used) {
		httpx.Error(w, http.StatusConflict, "Descrierea nu este disponibilă acum.")
		return
	}
	if p.Mode == games.ModeTitle && synopsisOf(p) == "" {
		httpx.Error(w, http.StatusNotFound, "Seria asta nu are descriere.")
		return
	}
	if p.Mode == games.ModeTheme && (p.ThemeVideo == nil || *p.ThemeVideo == "") {
		httpx.Error(w, http.StatusNotFound, "Nu există video pentru acest cântec.")
		return
	}
	if err := h.repo.SetHintUsed(ctx, p.ID, u.UserID); err != nil {
		httpx.Internal(w, "save hint", err)
		return
	}
	fresh, err := h.repo.PuzzleForUser(ctx, p.ID, u.UserID)
	if err != nil {
		httpx.Internal(w, "reload puzzle", err)
		return
	}
	hv := toView(fresh, today)
	h.fillGrid(ctx, fresh, &hv)
	httpx.JSON(w, http.StatusOK, map[string]any{"data": hv})
}


// fillGrid replays a player's guesses into the comparison table. Best-effort by
// design: a grid that fails to build should cost the player their table, never
// their answer, so every error here leaves the puzzle playable and empty.
func (h *Handler) fillGrid(ctx context.Context, p *repo.PuzzleRow, v *puzzleView) {
	// Built for both modes. The poster puzzle's mechanic is the reveal, but a
	// player still needs to see what they have already tried, and the same
	// comparison is useful there for free.
	ids := decodeGuesses(p.Guesses)
	if len(ids) == 0 {
		return
	}
	answer, err := h.repo.CardByID(ctx, p.AnimeID)
	if err != nil {
		return
	}
	cards, err := h.repo.Cards(ctx, ids)
	if err != nil {
		return
	}
	ans := cardOf(answer)
	rows := make([]games.GuessRow, 0, len(ids))
	for _, id := range ids {
		c, ok := cards[id]
		if !ok {
			continue
		}
		rows = append(rows, games.Compare(cardOf(&c), ans, genreLabelFn, typeLabelFn))
	}
	v.Grid = rows
	v.Summary = games.Summarise(rows)
}


// gamesClaimChest opens today's chest.
func (h *Handler) gamesClaimChest(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	if _, err := h.repo.EnsureProfile(ctx, u.UserID); err != nil {
		httpx.Internal(w, "load profile", err)
		return
	}
	streak, err := h.repo.ClaimChest(ctx, u.UserID)
	if errors.Is(err, repo.ErrExists) {
		httpx.Error(w, http.StatusConflict, "Ai deschis deja cufărul azi.")
		return
	}
	if err != nil {
		httpx.Internal(w, "claim chest", err)
		return
	}
	gold := games.ChestReward(streak)
	if err := h.repo.AwardXPGold(ctx, u.UserID, 0, gold, "daily_chest", ""); err != nil {
		httpx.Internal(w, "award chest", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"gold": gold, "streak": streak},
	})
}


// sameSeries decides whether a guessed series counts for a character.
//
// Both the scorer and the result panel call this. They used to answer the
// question separately -- the scorer walked the franchise, the panel compared
// ids -- so a player who answered "Re:Zero season 1" for a 4th-season
// character was paid for it and simultaneously told they were wrong.
func (h *Handler) sameSeries(ctx context.Context, trueID, guessed int) bool {
	if guessed == 0 {
		return false
	}
	if guessed == trueID {
		return true
	}
	fam, err := h.repo.FamilyOf(ctx, trueID)
	return err == nil && fam[guessed]
}

// fillChars builds the character round: five portraits, and — once submitted —
// the correct answers beside what the player gave.
func (h *Handler) fillChars(ctx context.Context, p *repo.PuzzleRow, v *puzzleView) {
	if p.Mode != games.ModeCharacter || len(p.CharIDs) == 0 {
		return
	}
	cards, err := h.repo.CharsByIDs(ctx, p.CharIDs)
	if err != nil {
		return
	}
	var given []charAnswer
	if p.CharAnswers != nil && *p.CharAnswers != "" {
		_ = json.Unmarshal([]byte(*p.CharAnswers), &given)
	}
	v.Scored = len(given) > 0
	if v.Scored {
		v.Finished = true
		if p.AwardedXp != nil {
			v.CharXp = *p.AwardedXp
		}
	}

	titles := map[int]string{}
	if v.Scored {
		ids := make([]int, 0, len(cards))
		for _, c := range cards {
			ids = append(ids, c.AnimeID)
		}
		if rows, err := h.repo.Cards(ctx, ids); err == nil {
			for id, c := range rows {
				titles[id] = displayTitle(c.Title, c.TitleRo, c.TitleEn)
			}
		}
	}

	for i, id := range p.CharIDs {
		c, ok := cards[id]
		if !ok {
			continue
		}
		slot := charSlot{ID: c.MalCharID, Image: c.ImageURL}
		if v.Scored {
			slot.Name = c.Name
			slot.Series = titles[c.AnimeID]
			if i < len(given) {
				slot.GaveName = given[i].CharID == c.MalCharID
				slot.GaveSeries = h.sameSeries(ctx, c.AnimeID, given[i].AnimeID)
			}
		}
		v.Chars = append(v.Chars, slot)
	}
}

// gamesCharSearch powers the name autocomplete.
func (h *Handler) gamesCharSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.JSON(w, http.StatusOK, map[string]any{"data": []repo.CharRow{}})
		return
	}
	rows, err := h.repo.SearchChars(r.Context(), q, 8)
	if err != nil {
		httpx.Internal(w, "search characters", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// gamesSubmitGroups takes one set of four titles and says whether they form a
// group. Unlike the character round this is played a set at a time, so the
// whole submission history lives in `guesses` and the score is settled only
// when the round ends -- by clearing the board or spending the mistakes.
func (h *Handler) gamesSubmitGroups(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Puzzle invalid")
		return
	}
	var body struct {
		AnimeIDs []int `json:"animeIds"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Cerere invalidă")
		return
	}
	if len(body.AnimeIDs) != games.GroupSize {
		httpx.Error(w, http.StatusBadRequest, "Alege exact trei titluri.")
		return
	}

	p, err := h.repo.PuzzleForUser(ctx, int64(id), u.UserID)
	if err != nil {
		notFoundOr(w, err, "Puzzle-ul nu există", "load puzzle")
		return
	}
	if p.Mode != games.ModeGroups {
		httpx.Error(w, http.StatusBadRequest, "Jocul acesta nu are grupe.")
		return
	}
	today := time.Now()
	if !games.InWindow(p.PlayDate, today) {
		httpx.Error(w, http.StatusGone, "Runda asta nu mai poate fi jucată.")
		return
	}

	sets := repo.ParseGroups(p.Groups)
	answer := make([][]int, 0, len(sets))
	valid := map[int]bool{}
	for _, g := range sets {
		answer = append(answer, g.AnimeIDs)
		for _, aid := range g.AnimeIDs {
			valid[aid] = true
		}
	}
	// A submission has to be four distinct tiles from this board. Without this
	// the endpoint accepts any four integers and the round can be cleared by
	// sending the answer straight from another day.
	seen := map[int]bool{}
	for _, aid := range body.AnimeIDs {
		if !valid[aid] || seen[aid] {
			httpx.Error(w, http.StatusBadRequest, "Titluri invalide pentru runda asta.")
			return
		}
		seen[aid] = true
	}

	var subs [][]int
	if p.Guesses != nil && *p.Guesses != "" {
		_ = json.Unmarshal([]byte(*p.Guesses), &subs)
	}
	if hits, miss := games.GroupProgress(subs, answer); games.GroupRoundOver(len(hits), miss) {
		httpx.Error(w, http.StatusConflict, "Runda s-a terminat deja.")
		return
	}

	subs = append(subs, body.AnimeIDs)
	hits, miss := games.GroupProgress(subs, answer)
	over := games.GroupRoundOver(len(hits), miss)

	xp, gold := 0, 0
	if over {
		xp, gold = games.ScoreGroups(len(hits))
		// Catching up later in the window pays half, the same rule as the
		// other modes.
		if base, _ := games.Award(p.PlayDate, today); base < games.BaseXP {
			xp, gold = xp/2, gold/2
		}
	}
	if err := h.repo.RecordGroupRound(ctx, int64(id), u.UserID, subs, over, len(hits) >= games.GroupCount, xp, gold); err != nil {
		httpx.Internal(w, "record group round", err)
		return
	}
	if over {
		h.checkPuzzleBadges(ctx, u.UserID, games.ModeGroups, len(hits))
	}

	found := make([]foundGroup, 0, len(hits))
	for _, i := range hits {
		found = append(found, foundGroup{Label: sets[i].Label, AnimeIDs: sets[i].AnimeIDs,
			Kind: sets[i].Kind, Value: sets[i].Value, Solved: true})
	}
	if over {
		already := map[int]bool{}
		for _, i := range hits {
			already[i] = true
		}
		for i, g := range sets {
			if !already[i] {
				found = append(found, foundGroup{Label: g.Label, AnimeIDs: g.AnimeIDs, Kind: g.Kind, Value: g.Value})
			}
		}
	}
	// How close this submission was: the most tiles it had in common with any
	// one group. Four means it was the group. Three is the answer worth saying
	// out loud, because the idea was right and one title was not.
	near := games.GroupNearMiss(body.AnimeIDs, answer)
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"found": found, "mistakes": miss, "maxMistakes": games.MaxGroupMistakes,
		"finished": over, "solved": len(hits) >= games.GroupCount,
		"closest": near,
		"award":   map[string]int{"xp": xp, "gold": gold},
	}})
}

// gamesSubmitChars scores a character round. One submission, no retries: the
// mode is a single considered answer rather than a sequence of attempts, so
// there is nothing to resume and nothing to spend.
func (h *Handler) gamesSubmitChars(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	ctx := r.Context()

	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Puzzle invalid")
		return
	}
	var body struct {
		Answers []charAnswer `json:"answers"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Cerere invalidă")
		return
	}

	p, err := h.repo.PuzzleForUser(ctx, int64(id), u.UserID)
	if err != nil {
		notFoundOr(w, err, "Puzzle-ul nu există", "load puzzle")
		return
	}
	if p.Mode != games.ModeCharacter {
		httpx.Error(w, http.StatusBadRequest, "Jocul acesta nu are personaje.")
		return
	}
	today := time.Now()
	if !games.InWindow(p.PlayDate, today) {
		httpx.Error(w, http.StatusGone, "Runda asta nu mai poate fi jucată.")
		return
	}
	if p.CharAnswers != nil && *p.CharAnswers != "" {
		httpx.Error(w, http.StatusConflict, "Ai trimis deja răspunsurile.")
		return
	}

	cards, err := h.repo.CharsByIDs(ctx, p.CharIDs)
	if err != nil {
		httpx.Internal(w, "load characters", err)
		return
	}
	nameHits, showHits := 0, 0
	for i, cid := range p.CharIDs {
		c, ok := cards[cid]
		if !ok || i >= len(body.Answers) {
			continue
		}
		if body.Answers[i].CharID == c.MalCharID {
			nameHits++
		}
		// Any entry in the same franchise counts: naming "Re:Zero" for a
		// character the catalogue files under its 4th Season is a correct
		// answer. Same helper the result panel uses.
		if h.sameSeries(ctx, c.AnimeID, body.Answers[i].AnimeID) {
			showHits++
		}
	}
	xp, gold := games.ScoreCharRound(nameHits, showHits)
	// The day multiplier applies here too, so catching up is worth less than
	// showing up — same rule as every other mode.
	if !p.PlayDate.Equal(today.Truncate(24*time.Hour)) {
		if base, _ := games.Award(p.PlayDate, today); base < games.BaseXP {
			xp, gold = xp/2, gold/2
		}
	}

	blob, _ := json.Marshal(body.Answers)
	if err := h.repo.SaveCharAnswers(ctx, p.ID, u.UserID, string(blob), xp, gold); err != nil {
		httpx.Internal(w, "save answers", err)
		return
	}
	if xp > 0 || gold > 0 {
		if err := h.repo.AwardXPGold(ctx, u.UserID, xp, gold,
			"character_round", strconv.FormatInt(p.ID, 10)); err != nil {
			httpx.Internal(w, "award", err)
			return
		}
	}

	fresh, err := h.repo.PuzzleForUser(ctx, p.ID, u.UserID)
	if err != nil {
		httpx.Internal(w, "reload puzzle", err)
		return
	}
	v := toView(fresh, today)
	h.fillChars(ctx, fresh, &v)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":  v,
		"score": map[string]int{"names": nameHits, "series": showHits, "xp": xp, "gold": gold},
	})
}

// leaderRow is one ranked player as the arcade draws them.
type leaderRow struct {
	Rank      int     `json:"rank"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
	Faction   string  `json:"faction,omitempty"`
	FactionNm string  `json:"factionName,omitempty"`
	Level     int     `json:"level"`
	Xp        int64   `json:"xp"`
	Gold      int64   `json:"gold"`
	RankTitle string  `json:"rankTitle,omitempty"`
}

// gamesLeaderboard: GET /api/games/leaderboard
//
// Its own endpoint rather than another field on the board, because the board is
// already the heaviest response in the arcade and every player loads it on every
// visit; this is read only when the tab is opened.
func (h *Handler) gamesLeaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.Leaderboard(r.Context(), games.LeaderboardSize)
	if err != nil {
		httpx.Internal(w, "leaderboard", err)
		return
	}
	out := make([]leaderRow, 0, len(rows))
	for i, x := range rows {
		lvl := games.LevelForXP(x.Xp)
		lr := leaderRow{
			Rank: i + 1, Username: x.Username, AvatarURL: x.AvatarURL,
			Level: lvl, Xp: x.Xp, Gold: x.Gold,
		}
		if x.Faction != nil && *x.Faction != "" {
			lr.Faction = *x.Faction
			lr.RankTitle = games.RankFor(*x.Faction, lvl)
			if f := games.FactionByCode(*x.Faction); f != nil {
				lr.FactionNm = f.Name
			}
		}
		out = append(out, lr)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}
