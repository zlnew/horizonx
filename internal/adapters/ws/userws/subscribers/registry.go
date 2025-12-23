package subscribers

import (
	"horizonx-server/internal/adapters/ws/userws"
)

func Register(bus EventBus, hub *userws.Hub) {
	// Server Events
	serverStatusChanged := NewServerStatusChanged(hub)
	serverMetricsReceived := NewServerMetricsReceived(hub)

	bus.Subscribe("server_status_changed", serverStatusChanged.Handle)
	bus.Subscribe("server_metrics_received", serverMetricsReceived.Handle)

	// Application Events
	applicationStatusChanged := NewApplicationStatusChanged(hub)

	bus.Subscribe("application_status_changed", applicationStatusChanged.Handle)

	// Deployment Events
	deploymentStatusChanged := NewDeploymentStatusChanged(hub)
	deploymentLogsUpdated := NewDeploymentLogsUpdated(hub)

	bus.Subscribe("deployment_status_changed", deploymentStatusChanged.Handle)
	bus.Subscribe("deployment_logs_updated", deploymentLogsUpdated.Handle)
}
