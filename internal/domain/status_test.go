package domain

import "testing"

// TestStatusValuesAreDistinct guards against copy-paste errors where two
// status constants of the same type end up with the same wire value (or an
// empty one). The API, agents, and alerting compare these as raw strings,
// so a duplicate would silently merge two states.
func TestStatusValuesAreDistinct(t *testing.T) {
	sets := map[string][]string{
		"application": {
			string(AppStatusDeploying),
			string(AppStatusStarting),
			string(AppStatusStopping),
			string(AppStatusRestarting),
			string(AppStatusRunning),
			string(AppStatusStopped),
			string(AppStatusFailed),
			string(AppStatusUnknown),
		},
		"deployment": {
			string(DeploymentPending),
			string(DeploymentDeploying),
			string(DeploymentSuccess),
			string(DeploymentFailed),
		},
		"job": {
			string(JobQueued),
			string(JobRunning),
			string(JobSuccess),
			string(JobFailed),
			string(JobExpired),
		},
		"alert state": {
			string(AlertStateFiring),
			string(AlertStateResolved),
		},
	}

	for typeName, values := range sets {
		t.Run(typeName, func(t *testing.T) {
			seen := make(map[string]bool, len(values))
			for _, v := range values {
				if v == "" {
					t.Errorf("%s status has an empty value", typeName)
				}
				if seen[v] {
					t.Errorf("%s status value %q is duplicated", typeName, v)
				}
				seen[v] = true
			}
		})
	}
}

// TestValidMetricPath pins the evaluator's accepted metric_path vocabulary
// (see internal/alerting/evaluator.go). Accepting an unknown path lets a
// rule save that can never fire; rejecting a known path blocks a valid rule.
func TestValidMetricPath(t *testing.T) {
	t.Run("known paths are accepted", func(t *testing.T) {
		known := []string{
			MetricPathCPUUsagePercent,
			MetricPathCPUTemperature,
			MetricPathMemoryUsagePercent,
			MetricPathMemoryUsedGB,
			MetricPathDiskUsagePercent,
			MetricPathNetworkRXSpeedMBs,
			MetricPathNetworkTXSpeedMBs,
			MetricPathUptimeSeconds,
		}
		for _, p := range known {
			if !ValidMetricPath(p) {
				t.Errorf("ValidMetricPath(%q) = false, want true", p)
			}
		}
	})

	t.Run("unknown empty and case-mismatched paths are rejected", func(t *testing.T) {
		// Lookup is exact: the evaluator resolves these strings verbatim,
		// so near-misses must not validate.
		bad := []string{
			"",
			"cpu",
			"cpu.usage",
			"CPU.USAGE_PERCENT",
			"cpu.usage_percent ",
			" cpu.usage_percent",
			"memory.*",
		}
		for _, p := range bad {
			if ValidMetricPath(p) {
				t.Errorf("ValidMetricPath(%q) = true, want false", p)
			}
		}
	})
}
