package server_test

import (
	"context"
	"testing"

	"horizonx/internal/application/server"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestService_RotateSecret_ReturnsNewToken(t *testing.T) {
	mockRepo := mocks.NewMockServerRepository(t)
	serverID := uuid.New()

	mockRepo.EXPECT().
		GetByID(mock.Anything, serverID).
		Return(&domain.Server{ID: serverID, Name: "prod-1"}, nil)
	mockRepo.EXPECT().
		UpdateSecret(mock.Anything, serverID, mock.Anything).
		Return(nil)

	svc := server.NewService(mockRepo, nil)
	token, err := svc.RotateSecret(context.Background(), serverID)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Contains(t, token, "hzx_")
}

func TestService_RotateSecret_NotFound(t *testing.T) {
	mockRepo := mocks.NewMockServerRepository(t)
	serverID := uuid.New()

	mockRepo.EXPECT().
		GetByID(mock.Anything, serverID).
		Return(nil, domain.ErrServerNotFound)

	svc := server.NewService(mockRepo, nil)
	token, err := svc.RotateSecret(context.Background(), serverID)

	assert.ErrorIs(t, err, domain.ErrServerNotFound)
	assert.Empty(t, token)
}
