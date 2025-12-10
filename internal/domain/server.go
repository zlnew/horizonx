package domain

import (
	"context"
	"errors"
	"time"
)

var ErrServerNotFound = errors.New("server not found")

type Server struct {
	ID        int64     `json:"id"`
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
	Create(ctx context.Context, s *Server) error
	Update(ctx context.Context, s *Server, serverID int64) error
	Delete(ctx context.Context, serverID int64) error
	GetByID(ctx context.Context, serverID int64) (*Server, error)
	GetByToken(ctx context.Context, token string) (*Server, error)
	UpdateStatus(ctx context.Context, serverID int64, isOnline bool) error
}

type ServerService interface {
	Get(ctx context.Context) ([]Server, error)
	Register(ctx context.Context, req ServerSaveRequest) (*Server, string, error)
	Update(ctx context.Context, req ServerSaveRequest, serverID int64) error
	Delete(ctx context.Context, serverID int64) error
}
