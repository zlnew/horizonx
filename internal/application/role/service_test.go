package role_test

import (
	"context"
	"testing"

	"horizonx/internal/application/role"
	"horizonx/internal/domain"

	"github.com/stretchr/testify/assert"
)

func ctxWithRole(r domain.RoleConst) context.Context {
	return domain.SetUserContext(context.Background(), domain.UserContext{ID: 1, Role: r})
}

func TestHasPermission_ViewerJobAndLogAccess(t *testing.T) {
	svc := role.NewService(nil)
	ctx := ctxWithRole(domain.RoleViewer)

	assert.NoError(t, svc.HasPermission(ctx, domain.PermJobRead))
	assert.NoError(t, svc.HasPermission(ctx, domain.PermLogRead))
	assert.ErrorIs(t, svc.HasPermission(ctx, domain.PermJobWrite), domain.ErrYouDontHavePermission)
}

func TestHasPermission_AdminJobAndLogAccess(t *testing.T) {
	svc := role.NewService(nil)
	ctx := ctxWithRole(domain.RoleAdmin)

	assert.NoError(t, svc.HasPermission(ctx, domain.PermJobRead))
	assert.NoError(t, svc.HasPermission(ctx, domain.PermLogRead))
	assert.NoError(t, svc.HasPermission(ctx, domain.PermJobWrite))
}
