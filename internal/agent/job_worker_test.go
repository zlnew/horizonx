package agent

import (
	"testing"
	"time"

	"horizonx/internal/domain"
)

// P0-2: deploy/rollback timeouts must be long enough for real builds.
func TestDeployTimeoutIsLongEnoughForRealBuilds(t *testing.T) {
	if d := jobTimeout(domain.JobTypeAppDeploy); d < 10*time.Minute {
		t.Fatalf("deploy timeout %v is too short for real builds (needs >= 10m)", d)
	}
	if d := jobTimeout(domain.JobTypeAppRollback); d < 5*time.Minute {
		t.Fatalf("rollback timeout %v too short", d)
	}
}

// P0-2: the old 30s deploy budget must never come back.
func TestDeployTimeoutNotThirtySeconds(t *testing.T) {
	if d := jobTimeout(domain.JobTypeAppDeploy); d == 30*time.Second {
		t.Fatal("deploy timeout must not be 30s")
	}
}

func TestDefaultTimeoutAppliesToUnknownTypes(t *testing.T) {
	if d := jobTimeout("no_such_job"); d != defaultJobTimeout {
		t.Fatalf("expected default %v, got %v", defaultJobTimeout, d)
	}
}
