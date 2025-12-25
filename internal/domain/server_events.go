package domain

import "github.com/google/uuid"

type EventServerStatusChanged struct {
	ServerID uuid.UUID `json:"server_id"`
	IsOnline bool      `json:"is_online"`
}
