// Package version carries the build version shared by server and agent.
// P2-18: agent version pinning — the server warns when an agent's version
// does not match its own, so stale agents are visible before they drift.
package version

import "runtime/debug"

// Version is the semantic version of this build. Overridable at build time:
//
//	go build -ldflags "-X horizonx/internal/version.Version=1.2.3"
var Version = "dev"

// AgentVersionHeader is the HTTP header agents send so the server can
// compare versions.
const AgentVersionHeader = "X-Agent-Version"

// BuildInfo returns a short description of the build (module version if
// available). Used in startup logs.
func BuildInfo() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
