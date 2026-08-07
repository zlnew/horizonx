package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"horizonx/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingsRepository struct {
	db *pgxpool.Pool
}

func NewSettingsRepository(db *pgxpool.Pool) domain.SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context, key string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := r.db.QueryRow(ctx,
		`SELECT value FROM settings WHERE key = $1`, key,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrSettingNotFound
	}
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (r *SettingsRepository) Set(ctx context.Context, key string, value json.RawMessage) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value,
	)
	return err
}

func (r *SettingsRepository) Delete(ctx context.Context, key string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM settings WHERE key = $1`, key)
	return err
}
