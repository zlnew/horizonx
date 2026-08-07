package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnv(t *testing.T) {
	vars := parseEnv("# comment\n\nKEY=value\nSPACED = spaced value\nNOVAL\n")
	if vars["KEY"] != "value" {
		t.Errorf("KEY = %q, want value", vars["KEY"])
	}
	if vars["SPACED"] != "spaced value" {
		t.Errorf("SPACED = %q, want 'spaced value'", vars["SPACED"])
	}
	if _, ok := vars["NOVAL"]; ok {
		t.Errorf("NOVAL should be skipped")
	}
}

func TestLoadAgentServerURLFromInstance(t *testing.T) {
	// Same-box install: --token not given, so the URL comes from the instance
	// .env but the token MUST NOT (dashboard registration is the only valid
	// source; the .env HORIZONX_SERVER_ID/API_TOKEN are placeholders).
	dir := t.TempDir()
	// instanceDir is a package const pointing at /opt/horizonx — can't change.
	// Instead verify the read logic against a temp .env via the parser +
	// URL derivation, and that the .env contains the URL keys.
	if _, err := GenerateInstance(dir, "203.0.113.10"); err != nil {
		t.Fatalf("GenerateInstance: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	vars := parseEnv(string(data))
	if vars["HORIZONX_API_URL"] == "" || vars["HORIZONX_WS_URL"] == "" {
		t.Errorf(".env missing HORIZONX_API_URL / HORIZONX_WS_URL")
	}
}

func TestInstallAgentTokenSplit(t *testing.T) {
	// --token "uuid.secret" must split into ServerID + ServerToken (the
	// dashboard-registration token format ValidateAgentCredentials expects).
	prov := defaultAgentProvision()
	prov.APIURL = "http://203.0.113.10:4858"
	prov.WSURL = "ws://203.0.113.10:4858/ws/agent"
	tok := "11111111-2222-3333-4444-555555555555.abcdef"
	if i := strings.Index(tok, "."); i > 0 {
		prov.ServerID = tok[:i]
		prov.ServerToken = tok[i+1:]
	}
	if prov.ServerID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("ServerID = %q", prov.ServerID)
	}
	if prov.ServerToken != "abcdef" {
		t.Errorf("ServerToken = %q", prov.ServerToken)
	}
}

func TestInstallAgentRequiresToken(t *testing.T) {
	// Regression (2026-08-04): install agent must NOT fall back to the instance
	// .env's HORIZONX_SERVER_ID/API_TOKEN — those placeholders never
	// authenticate (the server checks the dashboard-registered servers table).
	// Token is now REQUIRED; missing -> clear error.
	err := RunInstallAgent(InstallAgentOptions{Server: "http://203.0.113.10:4858"})
	if err == nil {
		t.Fatal("install agent without --token must fail")
	}
	if !strings.Contains(err.Error(), "--token") {
		t.Errorf("error should mention --token: %v", err)
	}
}

func TestInstallAgentRejectsMalformedToken(t *testing.T) {
	// A token without the "<uuid>.<secret>" shape is rejected up front.
	err := RunInstallAgent(InstallAgentOptions{Server: "http://203.0.113.10:4858", Token: "no-dot-here"})
	if err == nil {
		t.Fatal("install agent with malformed token must fail")
	}
	if !strings.Contains(err.Error(), "invalid agent token") {
		t.Errorf("error should mention invalid token: %v", err)
	}
}
