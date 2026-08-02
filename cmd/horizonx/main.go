// Command horizonx is the unified HorizonX CLI.
//
// One binary, subcommands — the opencode-style install target:
//
//	horizonx server    # run the control-plane server
//	horizonx agent     # run an app-host agent
//	horizonx setup     # bootstrap a control plane (secrets, .env, compose, systemd)
//	horizonx version   # print the build version
//	horizonx upgrade   # self-update to the latest release
package main

import (
	"flag"
	"fmt"
	"os"

	"horizonx/internal/app"
	"horizonx/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "server":
		err = app.RunServer()
	case "agent":
		err = app.RunAgent()
	case "setup":
		err = app.RunSetup()
	case "version", "--version", "-v":
		fmt.Println("horizonx " + version.Version)
		return
	case "upgrade":
		err = app.RunUpgrade()
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`horizonx — HorizonX control plane

Usage:
  horizonx server     Run the control-plane server (API + /metrics + WebSocket)
  horizonx agent      Run an app-host agent (deploys docker-compose apps)
  horizonx setup      Bootstrap a control plane: secrets, .env, compose, systemd
  horizonx migrate    Apply/rollback database migrations (-op=up|down|version|force)
  horizonx version    Print the build version
  horizonx upgrade    Self-update to the latest release
  horizonx help       Show this help

Install (one-liner, auto-sudo):
  curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash

Setup (interactive wizard):
  horizonx setup                       # walks through mode, preflight, method, env
  horizonx setup --mode agent --yes    # non-interactive agent install
  horizonx setup --generate-only       # write files only, install manually
  horizonx setup --host 203.0.113.10   # legacy: generate ./horizonx-setup only`)
}
// runMigrate parses the migrate subcommand flags and delegates to the shared
// app.RunMigrate implementation (same engine as cmd/migrate).
func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	envFile := fs.String("env-file", "", "path to .env file to load (optional)")
	cmd := fs.String("op", "", "operation: up, down, version, force")
	steps := fs.Int("steps", 0, "number of steps for up/down (0 = all); version number for force")
	dsn := fs.String("dsn", "", "database url (postgres://user:***@host:port/db); overrides DATABASE_URL env")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return app.RunMigrate(*envFile, *dsn, *cmd, *steps)
}
