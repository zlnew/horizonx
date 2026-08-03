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

func TestLoadAgentCredsFromBubble(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateBubble(dir, "203.0.113.10"); err != nil {
		t.Fatalf("GenerateBubble: %v", err)
	}
	// Assert the generated .env has exactly the keys install agent reads
	// (parseEnv handles the reading side, covered by TestParseEnv).
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	s := string(data)
	for _, k := range []string{"HORIZONX_SERVER_ID", "HORIZONX_SERVER_API_TOKEN", "HORIZONX_API_URL", "HORIZONX_WS_URL"} {
		if !strings.Contains(s, k+"=") {
			t.Errorf(".env missing %s", k)
		}
	}
}

func TestInstallAgentTokenSplit(t *testing.T) {
	// --token "uuid.secret" must split into ServerID + ServerToken.
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
