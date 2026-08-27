package repo

// The team-decided weekly programme (migration 0035). Coordinators and admins
// write it; every member reads it.

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// slotCols joins the series a slot points at, plus whether that episode is
// already on the site — which is what decides if a card can link straight to
// the player instead of only to the series page.
const slotCols = `
	s.id, s.anime_id, s.episode_number, s.scheduled_at, s.note,
	u.username AS created_by_name,
	a.title, a.title_english, a.title_romanian, a.image_url,
	EXISTS(
		SELECT 1 FROM episodes e
		WHERE e.anime_id = s.anime_id AND e.episode_number = s.episode_number
	) AS published`

// ScheduleWindow returns slots between two instants, soonest first.
func (r *Repo) ScheduleWindow(ctx context.Context, from, to time.Time) ([]model.ScheduleSlot, error) {
	rows := []model.ScheduleSlot{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+slotCols+`
		FROM schedule_slots s
		JOIN anime a ON a.id = s.anime_id
		LEFT JOIN users u ON u.id = s.created_by
		WHERE s.scheduled_at >= $1 AND s.scheduled_at < $2
		ORDER BY s.scheduled_at, a.title`, from, to)
	return rows, err
}

// UpcomingSchedule is the admin view: everything from `from` onwards, so a
// coordinator sees the whole plan rather than one week of it.
func (r *Repo) UpcomingSchedule(ctx context.Context, from time.Time, limit int) ([]model.ScheduleSlot, error) {
	rows := []model.ScheduleSlot{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+slotCols+`
		FROM schedule_slots s
		JOIN anime a ON a.id = s.anime_id
		LEFT JOIN users u ON u.id = s.created_by
		WHERE s.scheduled_at >= $1
		ORDER BY s.scheduled_at, a.title
		LIMIT $2`, from, limit)
	return rows, err
}

// SlotByID reads one row back after a write, so the client gets the stored
// version with its joins rather than its own input echoed.
func (r *Repo) SlotByID(ctx context.Context, id int) (model.ScheduleSlot, error) {
	var s model.ScheduleSlot
	err := pgxscan.Get(ctx, r.pool, &s, `
		SELECT `+slotCols+`
		FROM schedule_slots s
		JOIN anime a ON a.id = s.anime_id
		LEFT JOIN users u ON u.id = s.created_by
		WHERE s.id = $1`, id)
	if pgxscan.NotFound(err) {
		return s, ErrNotFound
	}
	return s, err
}

// UpsertSlot creates a slot, or moves the existing one for that episode.
//
// Upsert rather than insert-and-fail: "schedule episode 5 for Friday" when
// episode 5 is already on Thursday is a reschedule, not a conflict, and making
// the caller delete-then-create would leave a window with no slot at all.
func (r *Repo) UpsertSlot(ctx context.Context, animeID, episode int, at time.Time, note *string, by int) (model.ScheduleSlot, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO schedule_slots (anime_id, episode_number, scheduled_at, note, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (anime_id, episode_number) DO UPDATE
		SET scheduled_at = EXCLUDED.scheduled_at,
		    note         = EXCLUDED.note,
		    created_by   = EXCLUDED.created_by,
		    updated_at   = now()
		RETURNING id`, animeID, episode, at, note, by).Scan(&id)
	if err != nil {
		return model.ScheduleSlot{}, err
	}
	return r.SlotByID(ctx, id)
}

// UpdateSlot edits an existing slot by its own id — the admin list's "save".
func (r *Repo) UpdateSlot(ctx context.Context, id, episode int, at time.Time, note *string) (model.ScheduleSlot, error) {
	res, err := r.pool.Exec(ctx, `
		UPDATE schedule_slots
		SET episode_number = $2, scheduled_at = $3, note = $4, updated_at = now()
		WHERE id = $1`, id, episode, at, note)
	if err != nil {
		return model.ScheduleSlot{}, err
	}
	if res.RowsAffected() == 0 {
		return model.ScheduleSlot{}, ErrNotFound
	}
	return r.SlotByID(ctx, id)
}

func (r *Repo) DeleteSlot(ctx context.Context, id int) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM schedule_slots WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
