package app

import (
	"embed"
)

//go:embed templates/*.service
var systemdFS embed.FS

// systemdUnit returns the named unit template (e.g. "horizonx-server.service").
func systemdUnit(name string) ([]byte, error) {
	return systemdFS.ReadFile("templates/" + name)
}
