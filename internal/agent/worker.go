// Package agent
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"horizonx-server/internal/agent/executor"
	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

type JobWorker struct {
	cfg      *config.Config
	log      logger.Logger
	client   *Client
	executor *executor.Executor
}

func NewJobWorker(cfg *config.Config, log logger.Logger, metrics func() *domain.Metrics) *JobWorker {
	return &JobWorker{
		cfg:      cfg,
		log:      log,
		client:   NewClient(cfg),
		executor: executor.NewExecutor("/var/horizonx/apps", metrics, log),
	}
}

func (w *JobWorker) Initialize() error {
	return w.executor.Initialize()
}

func (w *JobWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	w.log.Info("job worker started, polling for jobs...")

	for {
		select {
		case <-ctx.Done():
			w.log.Info("job worker stopping...")
			return ctx.Err()

		case <-ticker.C:
			if err := w.pollAndExecuteJobs(ctx); err != nil {
				w.log.Error("failed to poll jobs", "error", err)
			}
		}
	}
}

func (w *JobWorker) pollAndExecuteJobs(ctx context.Context) error {
	jobs, err := w.client.GetPendingJobs(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil
	}

	w.log.Debug("received jobs", "count", len(jobs))

	for _, job := range jobs {
		if err := w.processJob(ctx, job); err != nil {
			w.log.Error("failed to process job", "job_id", job.ID, "error", err)
		}
	}

	return nil
}

func (w *JobWorker) processJob(ctx context.Context, job domain.Job) error {
	w.log.Debug("processing job", "job_id", job.ID)

	if err := w.client.StartJob(ctx, job.ID); err != nil {
		w.log.Error("failed to mark job as running", "job_id", job.ID, "error", err)
		return err
	}

	execErr := w.execute(job)

	status := domain.JobSuccess
	if execErr != nil {
		status = domain.JobFailed
		w.log.Error("job execution failed", "job_id", job.ID, "error", execErr)
	} else {
		w.log.Debug("job executed successfully", "job_id", job.ID)
	}

	if err := w.client.FinishJob(ctx, job.ID, status); err != nil {
		w.log.Error("failed to mark job as finished", "job_id", job.ID, "error", err)
		return err
	}

	return execErr
}

func (w *JobWorker) execute(job domain.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logCh := make(chan domain.EventLogEmitted, 200)
	commitCh := make(chan domain.EventCommitInfoEmitted, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for evt := range logCh {
			err := w.client.SendLog(ctx, &domain.LogEmitRequest{
				Timestamp:     evt.Timestamp,
				Level:         evt.Level,
				Source:        evt.Source,
				Action:        evt.Action,
				TraceID:       job.TraceID,
				JobID:         &job.ID,
				ServerID:      &job.ServerID,
				ApplicationID: job.ApplicationID,
				DeploymentID:  job.DeploymentID,
				Message:       evt.Message,
				Context:       evt.Context,
			})
			if err != nil {
				w.log.Error("failed to send log", "error", err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for evt := range commitCh {
			if err := w.client.SendCommitInfo(
				ctx,
				evt.DeploymentID,
				evt.Hash,
				evt.Message,
			); err != nil {
				w.log.Error("failed to send commit info", "error", err)
			}
		}
	}()

	bus := event.New()

	bus.Subscribe("metrics", func(event any) {
		metrics, ok := event.(*domain.Metrics)
		if !ok {
			return
		}

		if err := w.client.SendMetrics(ctx, metrics); err != nil {
			w.log.Error("failed to send metrics", "error", err)
		}
	})

	bus.Subscribe("log", func(event any) {
		evt, ok := event.(domain.EventLogEmitted)
		if !ok {
			return
		}

		logCh <- evt
	})

	bus.Subscribe("commit_info", func(event any) {
		evt, ok := event.(domain.EventCommitInfoEmitted)
		if !ok {
			return
		}

		commitCh <- evt
	})

	onEmit := func(event any) {
		switch event.(type) {
		case *domain.Metrics:
			bus.Publish("metrics", event)
		case domain.EventLogEmitted:
			bus.Publish("log", event)
		case domain.EventCommitInfoEmitted:
			bus.Publish("commit_info", event)
		}
	}

	err := w.executor.Execute(context.Background(), &job, onEmit)

	close(logCh)
	close(commitCh)

	wg.Wait()

	return err
}
