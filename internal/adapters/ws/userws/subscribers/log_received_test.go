package subscribers

import (
	"reflect"
	"testing"

	"horizonx/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func TestLogChannelsScopedByJobAndDeployment(t *testing.T) {
	jobID := int64(42)
	deploymentID := int64(7)

	got := logChannels(&domain.Log{JobID: &jobID, DeploymentID: &deploymentID})
	want := []string{"job:42", "deployment:7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestLogChannelsJobOnly(t *testing.T) {
	jobID := int64(42)

	got := logChannels(&domain.Log{JobID: &jobID})
	want := []string{"job:42"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestLogChannelsDeploymentOnly(t *testing.T) {
	deploymentID := int64(7)

	got := logChannels(&domain.Log{DeploymentID: &deploymentID})
	want := []string{"deployment:7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestLogChannelsFallbackToGlobal(t *testing.T) {
	got := logChannels(&domain.Log{})
	want := []string{"logs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
