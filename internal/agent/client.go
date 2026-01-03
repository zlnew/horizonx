package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
)

type Client struct {
	cfg  *config.Config
	http *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Client) UpdateServerOSInfo(ctx context.Context, req domain.OSInfo) error {
	url := fmt.Sprintf("%s/agent/server/os-info", c.cfg.AgentTargetAPIURL)

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to update server os info, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetPendingJobs(ctx context.Context) ([]domain.Job, error) {
	url := c.cfg.AgentTargetAPIURL + "/agent/jobs"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var response struct {
		Data []domain.Job `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) StartJob(ctx context.Context, jobID int64) error {
	url := fmt.Sprintf("%s/agent/jobs/%d/start", c.cfg.AgentTargetAPIURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to start job, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) FinishJob(ctx context.Context, jobID int64, status domain.JobStatus) error {
	url := fmt.Sprintf("%s/agent/jobs/%d/finish", c.cfg.AgentTargetAPIURL, jobID)

	payload := &domain.JobFinishRequest{
		Status: status,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to mark job finished, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendAppHealthReports(ctx context.Context, req []domain.ApplicationHealth) error {
	url := fmt.Sprintf("%s/agent/applications/health", c.cfg.AgentTargetAPIURL)

	body, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send application health reports, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendMetrics(ctx context.Context, req *domain.Metrics) error {
	url := fmt.Sprintf("%s/agent/metrics", c.cfg.AgentTargetAPIURL)

	body, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to send metrics, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendLog(ctx context.Context, req *domain.LogEmitRequest) error {
	url := fmt.Sprintf("%s/agent/logs", c.cfg.AgentTargetAPIURL)

	body, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to send log, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) error {
	url := fmt.Sprintf("%s/agent/deployments/%d/commit-info", c.cfg.AgentTargetAPIURL, deploymentID)

	payload := &domain.DeploymentCommitInfoRequest{
		CommitHash:    commitHash,
		CommitMessage: commitMessage,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.AgentServerID.String()+"."+c.cfg.AgentServerAPIToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send deployment commit info, status: %d", resp.StatusCode)
	}

	return nil
}
