package account_test

import (
	"context"

	"horizonx/internal/domain"
)

// fakeSessionStore is an in-memory SessionStore for tests.
type fakeSessionStore struct {
	sessions      map[string]*domain.Session
	deletedAllFor map[int64]bool
	deleted       []string
}

func newFakeSessionStore(sessions ...*domain.Session) *fakeSessionStore {
	f := &fakeSessionStore{
		sessions:      map[string]*domain.Session{},
		deletedAllFor: map[int64]bool{},
	}
	for _, s := range sessions {
		f.sessions[s.ID] = s
	}
	return f
}

func (f *fakeSessionStore) Create(ctx context.Context, s *domain.Session) error {
	if f.sessions == nil {
		f.sessions = map[string]*domain.Session{}
	}
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeSessionStore) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	return f.sessions[sessionID], nil
}

func (f *fakeSessionStore) Delete(ctx context.Context, sessionID string) error {
	delete(f.sessions, sessionID)
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
	var out []*domain.Session
	for _, s := range f.sessions {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}
