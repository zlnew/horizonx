package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrServerNotFound = errors.New("server not found")

type Server struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	IPAddress string    `json:"ip_address"`
	APIToken  string    `json:"-"`
	IsOnline  bool      `json:"is_online"`
	OSInfo    *OSInfo   `json:"os_info,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ServerSaveRequest struct {
	Name      string `json:"name" validate:"required"`
	IPAddress string `json:"ip_address" validate:"required"`
}

type ServerRepository interface {
	List(ctx context.Context) ([]Server, error)
	Create(ctx context.Context, s *Server) (*Server, error)
	Update(ctx context.Context, s *Server, serverID uuid.UUID) error
	Delete(ctx context.Context, serverID uuid.UUID) error
	GetByID(ctx context.Context, serverID uuid.UUID) (*Server, error)
	GetByToken(ctx context.Context, token string) (*Server, error)
	UpdateStatus(ctx context.Context, serverID uuid.UUID, isOnline bool) error
}

type ServerService interface {
	Get(ctx context.Context) ([]Server, error)
	Register(ctx context.Context, req ServerSaveRequest) (*Server, string, error)
	Update(ctx context.Context, req ServerSaveRequest, serverID uuid.UUID) error
	Delete(ctx context.Context, serverID uuid.UUID) error
	AuthorizeAgent(ctx context.Context, serverID uuid.UUID, secret string) (*Server, error)
	UpdateStatus(ctx context.Context, serverID uuid.UUID, status bool) error
}
