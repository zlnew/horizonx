// Single source of truth for HorizonX's signature ports.
//
// HorizonX chose ports that are unique and recognizable rather than common
// dev-tool-range ports: 4858 = 0x4858 = ASCII "HX" (the mnemonic; verified
// free in the IANA service-names registry). Every URL the CLI emits for the
// server (API + WS) is derived from ServerPort via the helpers below so the
// agent's HORIZONX_API_URL / HORIZONX_WS_URL can never drift from the port
// the install actually configures.
package app

const (
	// ServerPort is the host port the HorizonX control plane listens on.
	ServerPort = "4858"
	// DashboardPort is the host port the HorizonX dashboard is served on.
	DashboardPort = "4859"
)

// ServerAPIURL returns the base HTTP URL for the server at host.
func ServerAPIURL(host string) string {
	return "http://" + host + ":" + ServerPort
}

// ServerWSURL returns the agent WebSocket endpoint URL for the server at host.
func ServerWSURL(host string) string {
	return "ws://" + host + ":" + ServerPort + "/ws/agent"
}
