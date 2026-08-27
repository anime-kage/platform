package repo

// Custom chat emotes (migration 0040).

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

const emoteCols = `id, code, image_url, width, height, is_active, created_at`

// Emotes lists emotes. `activeOnly` is what the chat and picker ask for; the
// admin list wants everything so a disabled one can be turned back on.
func (r *Repo) Emotes(ctx context.Context, activeOnly bool) ([]model.Emote, error) {
	rows := []model.Emote{}
	where := ""
	if activeOnly {
		where = "WHERE is_active"
	}
	err := pgxscan.Select(ctx, r.pool, &rows,
		`SELECT `+emoteCols+` FROM emotes `+where+` ORDER BY lower(code)`)
	return rows, err
}

func (r *Repo) CreateEmote(ctx context.Context, code, url string, w, h, by int) (model.Emote, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO emotes (code, image_url, width, height, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, code, url, w, h, by).Scan(&id)
	if err != nil {
		if IsUniqueViolation(err) {
			return model.Emote{}, ErrExists
		}
		return model.Emote{}, err
	}
	return r.EmoteByID(ctx, id)
}

func (r *Repo) EmoteByID(ctx context.Context, id int) (model.Emote, error) {
	var e model.Emote
	err := pgxscan.Get(ctx, r.pool, &e, `SELECT `+emoteCols+` FROM emotes WHERE id = $1`, id)
	if pgxscan.NotFound(err) {
		return e, ErrNotFound
	}
	return e, err
}

func (r *Repo) SetEmoteActive(ctx context.Context, id int, active bool) (model.Emote, error) {
	res, err := r.pool.Exec(ctx, `UPDATE emotes SET is_active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return model.Emote{}, err
	}
	if res.RowsAffected() == 0 {
		return model.Emote{}, ErrNotFound
	}
	return r.EmoteByID(ctx, id)
}

func (r *Repo) DeleteEmote(ctx context.Context, id int) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM emotes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
