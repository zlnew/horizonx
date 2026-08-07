package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"horizonx/internal/domain"

	"github.com/redis/go-redis/v9"
)

// SessionStore is the Redis-backed implementation of domain.SessionStore.
//
// Layout:
//
//	sess:{id}                  -> JSON session body, TTL = JWT expiry
//	user_sessions:{userID}     -> SET of session IDs (for list/revoke-all)
//
// TTL keeps expired sessions from piling up even if logout is never called.
type SessionStore struct {
	client *redis.Client
}

func NewSessionStore(client *redis.Client) *SessionStore {
	return &SessionStore{client: client}
}

func sessionKey(id string) string        { return "sess:" + id }
func userSessionsKey(userID int64) string { return fmt.Sprintf("user_sessions:%d", userID) }

func (s *SessionStore) Create(ctx context.Context, sess *domain.Session) error {
	raw, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	ttl := time.Until(sess.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}

	pipe := s.client.TxPipeline()
	pipe.Set(ctx, sessionKey(sess.ID), raw, ttl)
	pipe.SAdd(ctx, userSessionsKey(sess.UserID), sess.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *SessionStore) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	raw, err := s.client.Get(ctx, sessionKey(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sess domain.Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	sess, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, sessionKey(sessionID))
	if sess != nil {
		pipe.SRem(ctx, userSessionsKey(sess.UserID), sessionID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	ids, err := s.client.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	pipe := s.client.TxPipeline()
	for _, id := range ids {
		pipe.Del(ctx, sessionKey(id))
	}
	pipe.Del(ctx, userSessionsKey(userID))
	_, err = pipe.Exec(ctx)
	return err
}

func (s *SessionStore) ListForUser(ctx context.Context, userID int64) ([]*domain.Session, error) {
	ids, err := s.client.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return nil, err
	}

	var sessions []*domain.Session
	for _, id := range ids {
		sess, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if sess != nil {
			sessions = append(sessions, sess)
		}
	}
	return sessions, nil
}
