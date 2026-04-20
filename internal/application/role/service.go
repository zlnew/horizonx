package role

import (
	"context"

	"horizonx/internal/domain"
)

type Service struct {
	repo domain.RoleRepository
}

func NewService(repo domain.RoleRepository) domain.RoleService {
	return &Service{repo: repo}
}

var roleHasPermissions = map[domain.RoleConst]map[domain.PermissionConst]bool{
	domain.RoleAdmin: {
		domain.PermMetricsRead: true,
		domain.PermServerRead:  true,
		domain.PermServerWrite: true,
		domain.PermMemberRead:  true,
		domain.PermMemberWrite: true,
		domain.PermAppRead:     true,
		domain.PermAppWrite:    true,
	},
	domain.RoleViewer: {
		domain.PermMetricsRead: true,
		domain.PermServerRead:  true,
		domain.PermMemberRead:  true,
		domain.PermAppRead:     true,
	},
}

func (s *Service) HasPermission(ctx context.Context, perm domain.PermissionConst) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}

	perms, ok := roleHasPermissions[userCtx.Role]
	if !ok {
		return domain.ErrYouDontHavePermission
	}

	if !perms[perm] {
		return domain.ErrYouDontHavePermission
	}

	return nil
}

func (s *Service) SyncPermissions(ctx context.Context) error {
	return s.repo.SyncPermissions(ctx, roleHasPermissions)
}
