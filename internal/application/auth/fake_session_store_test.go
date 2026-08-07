package auth_test

import (
	"context"

	"horizonx/internal/domain"
)

// fakeSessionStore is a minimal in-memory SessionStore for auth tests.
type fakeSessionStore struct {
	created       []*domain.Session
	deleted       []string
	deletedAllFor map[int64]bool
}

func (f *fakeSessionStore) Create(ctx context.Context, s *domain.Session) error {
	f.created = append(f.created, s)
	return nil
}

func (f *fakeSessionStore) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	return nil, nil
}

func (f *fakeSessionStore) Delete(ctx context.Context, sessionID string) error {
	f.deleted = append(f.deleted, sessionID)
	return nil
}

func (f *fakeSessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	if f.deletedAllFor == nil {
		f.deletedAllFor = map[int64]bool{}
	}
	f.deletedAllFor[userID] = true
	return nil
}

func (f *fakeSessionStore) ListForUser(ctx context.Context, userID int64) ([]*domain.Session, error) {
	return nil, nil
}
