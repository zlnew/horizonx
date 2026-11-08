package config

import (
	"testing"
)

// S3: TRUST_PROXY is on by default, which is correct behind tailscale serve but
// spoofable when the API is reachable from a non-loopback address. The guard
// must speak up for every exposed bind and stay quiet for every loopback one.
func TestTrustProxyExposureWarning(t *testing.T) {
	tests := []struct {
		name       string
		address    string
		trustProxy bool
		wantWarn   bool
	}{
		{"all interfaces, trusting proxy", ":3000", true, true},
		{"explicit 0.0.0.0", "0.0.0.0:4858", true, true},
		{"lan address", "192.168.1.5:4858", true, true},
		{"tailscale address", "100.110.130.1:4858", true, true},
		{"loopback v4", "127.0.0.1:4858", true, false},
		{"loopback v4 elsewhere in 127/8", "127.9.9.9:4858", true, false},
		{"loopback v6", "[::1]:4858", true, false},
		{"localhost by name", "localhost:4858", true, false},
		{"all interfaces but not trusting proxy", ":3000", false, false},
		{"lan address but not trusting proxy", "192.168.1.5:4858", false, false},
		{"unparseable address", "not-an-address", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Address: tt.address, TrustProxy: tt.trustProxy}

			warn := cfg.TrustProxyExposureWarning()

			if tt.wantWarn && warn == "" {
				t.Fatalf("address %q with TrustProxy=%v must produce a warning, got none",
					tt.address, tt.trustProxy)
			}
			if !tt.wantWarn && warn != "" {
				t.Fatalf("address %q with TrustProxy=%v must not warn, got %q",
					tt.address, tt.trustProxy, warn)
			}
		})
	}
}

// A nil config must not panic, and a loopback bind must be silent — otherwise
// the warning fires on every healthy boot and stops being read.
func TestTrustProxyExposureWarningNilConfig(t *testing.T) {
	var cfg *Config

	if warn := cfg.TrustProxyExposureWarning(); warn != "" {
		t.Fatalf("nil config must not warn, got %q", warn)
	}
}
