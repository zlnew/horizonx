// Package application
package application

import (
	"context"
	"encoding/json"
	"fmt"

	"horizonx/internal/domain"
	"horizonx/internal/event"

	"github.com/google/uuid"
)

// AgentCommandSender is the narrow slice of the agent WS router the logs
// methods need. Defined here (not in domain) so the application layer never
// imports the adapter — *agentws.Router satisfies it structurally.
type AgentCommandSender interface {
	SendCommand(ctx context.Context, serverID uuid.UUID, cmd domain.AgentCommand) error
}

type Service struct {
	repo          domain.ApplicationRepository
	serverSvc     domain.ServerService
	jobSvc        domain.JobService
	deploymentSvc domain.DeploymentService
	bus           *event.Bus

	agentCmd AgentCommandSender
}

func NewService(
	repo domain.ApplicationRepository,
	serverSvc domain.ServerService,
	jobSvc domain.JobService,
	deploymentSvc domain.DeploymentService,
	bus *event.Bus,
	agentCmd AgentCommandSender,
) *Service {
	return &Service{
		repo:          repo,
		serverSvc:     serverSvc,
		jobSvc:        jobSvc,
		deploymentSvc: deploymentSvc,
		bus:           bus,

		agentCmd: agentCmd,
	}
}

func (s *Service) List(ctx context.Context, opts domain.ApplicationListOptions) (*domain.ListResult[*domain.Application], error) {
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

	applications, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.Application]{
		Data: applications,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
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
		ServerID: req.ServerID,
		Name:     req.Name,
		RepoName: req.RepoName,
		RepoURL:  req.RepoURL,
		SiteURL:  req.SiteURL,
		Branch:   req.Branch,
		Status:   domain.AppStatusStopped,
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

	if s.bus != nil {
		s.bus.Publish("application_created", domain.EventApplicationCreated{
			ApplicationID: app.ID,
			ServerID:      app.ServerID,
		})
	}

	return created, nil
}

func (s *Service) Update(ctx context.Context, req domain.ApplicationUpdateRequest, appID int64) error {
	_, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	app := &domain.Application{
		Name:    req.Name,
		SiteURL: req.SiteURL,
		Branch:  req.Branch,
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

	return nil
}

func (s *Service) Delete(ctx context.Context, appID int64) error {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return err
	}

	targets := []string{
		string(domain.AppStatusDeploying),
		string(domain.AppStatusStarting),
		string(domain.AppStatusRestarting),
		string(domain.AppStatusStopping),
	}
	if domain.ContainsAny(string(app.Status), targets) {
		return fmt.Errorf(
			"application cannot be deleted while it is %s; please wait for the operation to finish.",
			app.Status,
		)
	}

	payload := domain.AppDestroyPayload{
		ApplicationID: app.ID,
		AppKey:        domain.GetAppKey(app),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		Type:          domain.JobTypeAppDestroy,
		Payload:       payloadBytes,
	}

	if _, err := s.jobSvc.Create(ctx, job); err != nil {
		return err
	}

	return s.repo.Delete(ctx, appID)
}

func (s *Service) UpdateStatus(ctx context.Context, appID int64, status domain.ApplicationStatus) error {
	err := s.repo.UpdateStatus(ctx, appID, status)
	if err != nil {
		return err
	}

	if s.bus != nil {
		s.bus.Publish("application_status_changed", domain.EventApplicationStatusChanged{
			ApplicationID: appID,
			Status:        status,
		})
	}

	return nil
}

func (s *Service) UpdateLastDeployment(ctx context.Context, appID int64) error {
	return s.repo.UpdateLastDeployment(ctx, appID)
}

func (s *Service) Deploy(ctx context.Context, appID int64, deployedBy int64) (*domain.Deployment, error) {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	envVars, err := s.repo.ListEnvVars(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch env vars: %w", err)
	}

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Key] = env.Value
	}

	deployment, err := s.deploymentSvc.Create(ctx, domain.DeploymentCreateRequest{
		ApplicationID: appID,
		Branch:        app.Branch,
		DeployedBy:    &deployedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	// P3-19: snapshot the env vars used for this deploy (for the diff view).
	if err := s.deploymentSvc.UpdateEnvSnapshot(ctx, deployment.ID, envMap); err != nil {
		return nil, fmt.Errorf("failed to snapshot env vars: %w", err)
	}

	payload := domain.AppDeployPayload{
		ApplicationID: appID,
		DeploymentID:  deployment.ID,
		AppKey:        domain.GetAppKey(app),
		RepoURL:       app.RepoURL,
		Branch:        app.Branch,
		EnvVars:       envMap,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		DeploymentID:  &deployment.ID,
		Type:          domain.JobTypeAppDeploy,
		Payload:       payloadBytes,
	}

	if _, err := s.jobSvc.Create(ctx, job); err != nil {
		s.repo.UpdateStatus(ctx, appID, domain.AppStatusFailed)
		return nil, fmt.Errorf("failed to create deployment job: %w", err)
	}

	return deployment, nil
}

// Rollback re-deploys the last successfully built image for an app (P0-4).
// The previous image tag is derived from the most recent successful
// deployment's commit hash (image tags are `<appKey>:<commitHash>`), so no
// extra schema is needed — the tag already exists in the docker daemon.
func (s *Service) Rollback(ctx context.Context, appID int64, deployedBy int64) (*domain.Deployment, error) {
	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	// Find the last successful deployment to learn the image tag to roll back to.
	success := "success"
	result, err := s.deploymentSvc.List(ctx, domain.DeploymentListOptions{
		ListOptions:   domain.ListOptions{Limit: 1},
		ApplicationID: &appID,
		Statuses:      []string{success},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch previous deployments: %w", err)
	}
	var prev []*domain.Deployment
	if result != nil {
		prev = result.Data
	}
	if len(prev) == 0 || prev[0].CommitHash == nil || *prev[0].CommitHash == "" {
		return nil, fmt.Errorf("no successful deployment to roll back to")
	}

	imageTag := fmt.Sprintf("%s:%s", domain.GetAppKey(app), *prev[0].CommitHash)

	envVars, err := s.repo.ListEnvVars(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch env vars: %w", err)
	}

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Key] = env.Value
	}

	deployment, err := s.deploymentSvc.Create(ctx, domain.DeploymentCreateRequest{
		ApplicationID: appID,
		Branch:        app.Branch,
		DeployedBy:    &deployedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	payload := domain.AppRollbackPayload{
		ApplicationID: appID,
		DeploymentID:  deployment.ID,
		AppKey:        domain.GetAppKey(app),
		ImageTag:      imageTag,
		EnvVars:       envMap,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		DeploymentID:  &deployment.ID,
		Type:          domain.JobTypeAppRollback,
		Payload:       payloadBytes,
	}

	if _, err := s.jobSvc.Create(ctx, job); err != nil {
		s.repo.UpdateStatus(ctx, appID, domain.AppStatusFailed)
		return nil, fmt.Errorf("failed to create rollback job: %w", err)
	}

	return deployment, nil
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

	payload := domain.AppStartPayload{
		ApplicationID: appID,
		AppKey:        domain.GetAppKey(app),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		Type:          domain.JobTypeAppStart,
		Payload:       payloadBytes,
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

	payload := domain.AppStopPayload{
		ApplicationID: appID,
		AppKey:        domain.GetAppKey(app),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		Type:          domain.JobTypeAppStop,
		Payload:       payloadBytes,
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

	payload := domain.AppRestartPayload{
		ApplicationID: appID,
		AppKey:        domain.GetAppKey(app),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	job := &domain.Job{
		TraceID:       uuid.New(),
		ServerID:      app.ServerID,
		ApplicationID: &appID,
		Type:          domain.JobTypeAppRestart,
		Payload:       payloadBytes,
	}

	_, err = s.jobSvc.Create(ctx, job)
	return err
}

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

func (s *Service) UpdateHealth(ctx context.Context, serverID uuid.UUID, reports []domain.ApplicationHealth) error {
	if err := s.repo.UpdateHealth(ctx, serverID, reports); err != nil {
		return err
	}

	// Alerting hook: the agent's local "app_healths" topic only exists inside
	// the agent process; the server re-publishes the health report (with the
	// reporting server id) so the alert evaluator can react to it.
	if s.bus != nil {
		s.bus.Publish("app_healths", domain.EventApplicationHealthReported{
			ServerID: serverID,
			Reports:  reports,
		})
	}

	return nil
}
