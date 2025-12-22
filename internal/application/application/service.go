// Package application
package application

import (
	"context"
	"fmt"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"

	"github.com/google/uuid"
)

type Service struct {
	repo          domain.ApplicationRepository
	serverSvc     domain.ServerService
	jobSvc        domain.JobService
	deploymentSvc domain.DeploymentService
	bus           *event.Bus
}

func NewService(
	repo domain.ApplicationRepository,
	serverSvc domain.ServerService,
	jobSvc domain.JobService,
	deploymentSvc domain.DeploymentService,
	bus *event.Bus,
) domain.ApplicationService {
	return &Service{
		repo:          repo,
		serverSvc:     serverSvc,
		jobSvc:        jobSvc,
		deploymentSvc: deploymentSvc,
		bus:           bus,
	}
}

// ============================================================================
// APPLICATIONS
// ============================================================================

func (s *Service) List(ctx context.Context, serverID uuid.UUID) ([]domain.Application, error) {
	return s.repo.List(ctx, serverID)
}

func (s *Service) GetByID(ctx context.Context, appID int64) (*domain.Application, error) {
	return s.repo.GetByID(ctx, appID)
}

func (s *Service) Create(ctx context.Context, req domain.ApplicationCreateRequest) (*domain.Application, error) {
	_, err := s.serverSvc.GetByID(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}

	app := &domain.Application{
		ServerID:         req.ServerID,
		Name:             req.Name,
		RepoURL:          req.RepoURL,
		Branch:           req.Branch,
		DockerComposeRaw: req.DockerComposeRaw,
		Status:           domain.AppStatusStopped,
	}
	created, err := s.repo.Create(ctx, app)
	if err != nil {
		return nil, err
	}

	var envVars []domain.EnvironmentVariable
	for _, env := range req.EnvVars {
		envVars = append(envVars, domain.EnvironmentVariable{
			Key:       env.Key,
			Value:     env.Value,
			IsPreview: env.IsPreview,
		})
	}
	if err := s.repo.SyncEnvVars(ctx, created.ID, envVars); err != nil {
		return nil, err
	}

	var volumes []domain.Volume
	for _, vol := range req.Volumes {
		volumes = append(volumes, domain.Volume{
			HostPath:      vol.HostPath,
			ContainerPath: vol.ContainerPath,
			Mode:          vol.Mode,
		})
	}
	if err := s.repo.SyncVolumes(ctx, created.ID, volumes); err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("application_created", created)
	}

	return created, nil
}

func (s *Service) Update(ctx context.Context, req domain.ApplicationUpdateRequest, appID int64) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	app := &domain.Application{
		Name:             req.Name,
		RepoURL:          req.RepoURL,
		Branch:           req.Branch,
		DockerComposeRaw: req.DockerComposeRaw,
	}
	if err := s.repo.Update(ctx, app, appID); err != nil {
		return err
	}

	var envVars []domain.EnvironmentVariable
	for _, env := range req.EnvVars {
		envVars = append(envVars, domain.EnvironmentVariable{
			Key:       env.Key,
			Value:     env.Value,
			IsPreview: env.IsPreview,
		})
	}
	if err := s.repo.SyncEnvVars(ctx, appID, envVars); err != nil {
		return err
	}

	var volumes []domain.Volume
	for _, vol := range req.Volumes {
		volumes = append(volumes, domain.Volume{
			HostPath:      vol.HostPath,
			ContainerPath: vol.ContainerPath,
			Mode:          vol.Mode,
		})
	}
	if err := s.repo.SyncVolumes(ctx, appID, volumes); err != nil {
		return err
	}

	return nil
}

func (s *Service) Delete(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	if app.Status != domain.AppStatusStopped {
		return fmt.Errorf("cannot delete running application, stop it first")
	}

	return s.repo.Delete(ctx, appID)
}

// ============================================================================
// ACTIONS (Job-based)
// ============================================================================

func (s *Service) Deploy(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	envVars, err := s.repo.ListEnvVars(ctx, appID)
	if err != nil {
		return fmt.Errorf("failed to fetch env vars: %w", err)
	}

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Key] = env.Value
	}

	volumes, err := s.repo.ListVolumes(ctx, appID)
	if err != nil {
		return fmt.Errorf("failed to fetch volumes: %w", err)
	}

	volumeMounts := make([]domain.VolumeMount, len(volumes))
	for i, vol := range volumes {
		volumeMounts[i] = domain.VolumeMount{
			HostPath:      vol.HostPath,
			ContainerPath: vol.ContainerPath,
			Mode:          vol.Mode,
		}
	}

	deployment, err := s.deploymentSvc.Create(ctx, domain.DeploymentCreateRequest{
		ApplicationID: appID,
		Branch:        app.Branch,
		Environment:   "production", // TODO: Make this configurable
	})
	if err != nil {
		return fmt.Errorf("failed to create deployment record: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, appID, domain.AppStatusStarting); err != nil {
		return err
	}

	job := &domain.Job{
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		JobType:       domain.JobTypeDeployApp,
		CommandPayload: map[string]any{
			"application_id":     appID,
			"deployment_id":      deployment.ID,
			"repo_url":           app.RepoURL,
			"branch":             app.Branch,
			"docker_compose_raw": app.DockerComposeRaw,
			"env_vars":           envMap,
			"volumes":            volumeMounts,
		},
	}

	createdJob, err := s.jobSvc.Create(ctx, job)
	if err != nil {
		s.repo.UpdateStatus(ctx, appID, domain.AppStatusFailed)
		return fmt.Errorf("failed to create deployment job: %w", err)
	}

	s.bus.Publish("deployment_started", map[string]any{
		"deployment_id":  deployment.ID,
		"application_id": appID,
		"job_id":         createdJob.ID,
	})

	return nil
}

func (s *Service) Start(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	if app.Status == domain.AppStatusRunning {
		return fmt.Errorf("application is already running")
	}

	if err := s.repo.UpdateStatus(ctx, appID, domain.AppStatusStarting); err != nil {
		return err
	}

	job := &domain.Job{
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		JobType:       domain.JobTypeStartApp,
		CommandPayload: map[string]any{
			"application_id": appID,
		},
	}

	_, err = s.jobSvc.Create(ctx, job)
	return err
}

func (s *Service) Stop(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	if app.Status == domain.AppStatusStopped {
		return fmt.Errorf("application is already stopped")
	}

	job := &domain.Job{
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		JobType:       domain.JobTypeStopApp,
		CommandPayload: map[string]any{
			"application_id": appID,
		},
	}

	_, err = s.jobSvc.Create(ctx, job)
	return err
}

func (s *Service) Restart(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	if err := s.repo.UpdateStatus(ctx, appID, domain.AppStatusRestarting); err != nil {
		return err
	}

	job := &domain.Job{
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		JobType:       domain.JobTypeRestartApp,
		CommandPayload: map[string]any{
			"application_id": appID,
		},
	}

	_, err = s.jobSvc.Create(ctx, job)
	return err
}

// ============================================================================
// ENVIRONMENT VARIABLES
// ============================================================================

func (s *Service) ListEnvVars(ctx context.Context, appID int64) ([]domain.EnvironmentVariable, error) {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListEnvVars(ctx, appID)
}

func (s *Service) AddEnvVar(ctx context.Context, appID int64, req domain.EnvironmentVariableRequest) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	env := &domain.EnvironmentVariable{
		ApplicationID: appID,
		Key:           req.Key,
		Value:         req.Value,
		IsPreview:     req.IsPreview,
	}

	return s.repo.CreateEnvVar(ctx, env)
}

func (s *Service) UpdateEnvVar(ctx context.Context, appID int64, key string, req domain.EnvironmentVariableRequest) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	env := &domain.EnvironmentVariable{
		ApplicationID: appID,
		Key:           key,
		Value:         req.Value,
		IsPreview:     req.IsPreview,
	}

	return s.repo.UpdateEnvVar(ctx, env)
}

func (s *Service) DeleteEnvVar(ctx context.Context, appID int64, key string) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	return s.repo.DeleteEnvVar(ctx, appID, key)
}

// ============================================================================
// VOLUMES
// ============================================================================

func (s *Service) ListVolumes(ctx context.Context, appID int64) ([]domain.Volume, error) {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	return s.repo.ListVolumes(ctx, appID)
}

func (s *Service) AddVolume(ctx context.Context, appID int64, req domain.VolumeRequest) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	vol := &domain.Volume{
		ApplicationID: appID,
		HostPath:      req.HostPath,
		ContainerPath: req.ContainerPath,
		Mode:          req.Mode,
	}

	return s.repo.CreateVolume(ctx, vol)
}

func (s *Service) DeleteVolume(ctx context.Context, volumeID int64) error {
	return s.repo.DeleteVolume(ctx, volumeID)
}
