package app

import "testing"

func TestDefaultPorts(t *testing.T) {
	if ServerPort != "4858" {
		t.Errorf("ServerPort = %q, want 4858", ServerPort)
	}
	if DashboardPort != "4859" {
		t.Errorf("DashboardPort = %q, want 4859", DashboardPort)
	}
	if u := ServerAPIURL("myserver"); u != "http://myserver:4858" {
		t.Errorf("ServerAPIURL = %s, want http://myserver:4858", u)
	}
	if w := ServerWSURL("myserver"); w != "ws://myserver:4858/ws/agent" {
		t.Errorf("ServerWSURL = %s, want ws://myserver:4858/ws/agent", w)
	}
}
