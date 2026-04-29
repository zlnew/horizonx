// Package agent
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"horizonx/internal/agent/executor"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"
)

type JobWorker struct {
	cfg *config.Config
	log logger.Logger

	httpClient *HttpClient
	executor   *executor.Executor
}

func NewJobWorker(cfg *config.Config, log logger.Logger, httpClient HttpClient, executor executor.Executor) *JobWorker {
	return &JobWorker{
		cfg: cfg,
		log: log,

		httpClient: &httpClient,
		executor:   &executor,
	}
}

func (w *JobWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	w.log.Info("job worker started, polling for jobs...")

	for {
		select {
		case <-ctx.Done():
			w.log.Info("job worker stopped")
			return nil
		case <-ticker.C:
			if err := w.pollAndExecuteJobs(ctx); err != nil {
				w.log.Warn("failed to poll and execute jobs", "error", err)
			}
		}
	}
}

func (w *JobWorker) pollAndExecuteJobs(ctx context.Context) error {
	jobs, err := w.httpClient.GetPendingJobs(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}
	if len(jobs) == 0 {
		return nil
	}

	w.log.Debug("received jobs", "count", len(jobs))

	jobCh := make(chan domain.Job, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	errCh := make(chan error, len(jobs))

	var wg sync.WaitGroup

	for range w.cfg.AgentJobWorkerCount {
		wg.Go(func() {
			for job := range jobCh {
				select {
				case <-ctx.Done():
					return
				case errCh <- w.processJob(ctx, job):
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for jobErr := range errCh {
		if jobErr != nil {
			w.log.Error("failed to process job", "error", jobErr)
		}
	}

	return nil
}

func (w *JobWorker) processJob(ctx context.Context, job domain.Job) error {
	w.log.Debug("processing job", "job_id", job.ID)

	if err := w.httpClient.StartJob(ctx, job.ID); err != nil {
		w.log.Error("failed to mark job as running", "job_id", job.ID, "error", err)
		return err
	}

	execErr := w.execute(ctx, job)

	status := domain.JobSuccess
	if execErr != nil {
		status = domain.JobFailed
		w.log.Error("job execution failed", "job_id", job.ID, "error", execErr)
	} else {
		w.log.Debug("job executed successfully", "job_id", job.ID)
	}

	if err := w.httpClient.FinishJob(ctx, job.ID, status); err != nil {
		w.log.Error("failed to mark job as finished", "job_id", job.ID, "error", err)
		return err
	}

	return execErr
}

var jobTimeouts = map[domain.JobType]time.Duration{
	domain.JobTypeAppDeploy:      30 * time.Second,
	domain.JobTypeAppStart:       45 * time.Second,
	domain.JobTypeAppStop:        2 * time.Minute,
	domain.JobTypeAppRestart:     10 * time.Minute,
	domain.JobTypeAppHealthCheck: 30 * time.Second,
	domain.JobTypeAppDestroy:     5 * time.Minute,
	domain.JobTypeMetricsCollect: 30 * time.Second,
}

const defaultJobTimeout = 5 * time.Minute

func jobTimeout(t domain.JobType) time.Duration {
	if d, ok := jobTimeouts[t]; ok {
		return d
	}
	return defaultJobTimeout
}

func (w *JobWorker) execute(ctx context.Context, job domain.Job) error {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout(job.Type))
	defer cancel()

	logCh := make(chan domain.EventLogEmitted, 500)
	commitCh := make(chan domain.EventCommitInfoEmitted, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	sendCtx := context.WithoutCancel(ctx)
	go func() {
		defer wg.Done()
		for evt := range logCh {
			err := w.httpClient.SendLog(sendCtx, &domain.LogEmitRequest{
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
			if err := w.httpClient.SendCommitInfo(
				sendCtx,
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

		if err := w.httpClient.SendMetrics(ctx, metrics); err != nil {
			w.log.Error("failed to send metrics", "error", err)
		}
	})

	bus.Subscribe("app_healths", func(event any) {
		reports, ok := event.([]domain.ApplicationHealth)
		if !ok {
			return
		}

		if err := w.httpClient.SendAppHealthReports(ctx, reports); err != nil {
			w.log.Error("failed to send application health reports", "error", err)
		}
	})

	bus.Subscribe("log", func(event any) {
		evt, ok := event.(domain.EventLogEmitted)
		if !ok {
			return
		}

		select {
		case logCh <- evt:
		default:
			w.log.Warn("log channel full, dropping event")
		}
	})

	bus.Subscribe("commit_info", func(event any) {
		evt, ok := event.(domain.EventCommitInfoEmitted)
		if !ok {
			return
		}

		select {
		case commitCh <- evt:
		default:
			w.log.Warn("commit channel full, dropping event")
		}
	})

	onEmit := func(event any) {
		switch event.(type) {
		case *domain.Metrics:
			bus.Publish("metrics", event)
		case []domain.ApplicationHealth:
			bus.Publish("app_healths", event)
		case domain.EventLogEmitted:
			bus.Publish("log", event)
		case domain.EventCommitInfoEmitted:
			bus.Publish("commit_info", event)
		}
	}

	err := w.executor.Execute(ctx, &job, onEmit)

	close(logCh)
	close(commitCh)

	wg.Wait()

	return err
}
