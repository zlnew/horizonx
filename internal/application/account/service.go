package account

import (
	"context"
	"sort"

	"horizonx/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo     domain.UserRepository
	sessions domain.SessionStore
}

func NewService(repo domain.UserRepository, sessions domain.SessionStore) domain.AccountService {
	return &Service{repo: repo, sessions: sessions}
}

func (s *Service) UpdateProfile(ctx context.Context, req domain.AccountProfileRequest) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}

	user, err := s.repo.GetByID(ctx, userCtx.ID)
	if err != nil {
		return err
	}

	user.Name = req.Name

	return s.repo.Update(ctx, user, user.ID)
}

func (s *Service) ChangePassword(ctx context.Context, req domain.AccountPasswordRequest) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}

	user, err := s.repo.GetByID(ctx, userCtx.ID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		return domain.ErrInvalidCurrentPassword
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(newHashedPassword)

	// Password changed — revoke every session so the old token is dead
	// everywhere immediately (the user re-logs in with the new password).
	if err := s.repo.Update(ctx, user, user.ID); err != nil {
		return err
	}
	if s.sessions != nil {
		return s.sessions.DeleteAllForUser(ctx, userCtx.ID)
	}
	return nil
}

// ListSessions returns the caller's sessions, current device first, then
// newest-first by CreatedAt. Sessions whose sess: key has TTL'd are pruned
// from the user_sessions set by the store during the read.
func (s *Service) ListSessions(ctx context.Context) ([]*domain.Session, error) {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	if s.sessions == nil {
		return []*domain.Session{}, nil
	}

	sessions, err := s.sessions.ListForUser(ctx, userCtx.ID)
	if err != nil {
		return nil, err
	}

	// Current device first, then newest-first.
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].ID == userCtx.SessionID {
			return true
		}
		if sessions[j].ID == userCtx.SessionID {
			return false
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	return sessions, nil
}

// RevokeSession terminates one of the caller's sessions. The ownership
// check is the security-critical line: an authenticated user must not be
// able to terminate another user's session by guessing an ID.
func (s *Service) RevokeSession(ctx context.Context, sessionID string) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}
	if s.sessions == nil {
		return domain.ErrSessionNotFound
	}

	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess == nil || sess.UserID != userCtx.ID {
		return domain.ErrSessionNotFound
	}

	return s.sessions.Delete(ctx, sessionID)
}

// RevokeOtherSessions terminates every session except the current one.
func (s *Service) RevokeOtherSessions(ctx context.Context) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}
	if s.sessions == nil {
		return nil
	}

	sessions, err := s.sessions.ListForUser(ctx, userCtx.ID)
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess.ID != userCtx.SessionID {
			if err := s.sessions.Delete(ctx, sess.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
