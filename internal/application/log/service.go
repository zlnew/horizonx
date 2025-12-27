// Package log
package log

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
)

type LogService struct {
	repo domain.LogRepository
	bus  *event.Bus
}

func NewService(repo domain.LogRepository, bus *event.Bus) domain.LogService {
	return &LogService{
		repo: repo,
		bus:  bus,
	}
}

func (s *LogService) List(ctx context.Context, opts domain.LogListOptions) (*domain.ListResult[*domain.Log], error) {
	if opts.IsPaginate {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		if opts.Limit <= 0 {
			opts.Limit = 10
		}
	} else {
		if opts.Limit <= 0 {
			opts.Limit = 1000
		}
	}

	logs, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.Log]{
		Data: logs,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
}

func (s *LogService) GetByID(ctx context.Context, logID int64) (*domain.Log, error) {
	return s.repo.GetByID(ctx, logID)
}

func (s *LogService) Emit(ctx context.Context, l *domain.Log) (*domain.Log, error) {
	log, err := s.repo.Emit(ctx, l)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("log_received", l)
	}

	return log, nil
}
