package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBranch(t *testing.T) {
	// Valid branches
	assert.NoError(t, validateBranch("main"))
	assert.NoError(t, validateBranch("feature/ok"))
	assert.NoError(t, validateBranch("v1.0.0"))

	// Invalid branches (empty or beginning with '-')
	assert.Error(t, validateBranch(""))
	assert.Error(t, validateBranch("-foo"))
	assert.Error(t, validateBranch("--upload-pack=x"))
}

func TestBuildCloneArgs(t *testing.T) {
	branch := "main"
	remoteURL := "https://github.com/example/repo.git"
	workDir := "/tmp/workdir"

	args := buildCloneArgs(branch, remoteURL, workDir)
	expected := []string{"clone", "--branch", branch, "--depth", "1", "--", remoteURL, workDir}
	assert.Equal(t, expected, args)

	// Ensure malicious URL starting with flag is placed after `--`
	maliciousURL := "--upload-pack=/bin/echo"
	argsMalicious := buildCloneArgs(branch, maliciousURL, workDir)
	require.Len(t, argsMalicious, 8)
	assert.Equal(t, "--", argsMalicious[5])
	assert.Equal(t, maliciousURL, argsMalicious[6])
}
