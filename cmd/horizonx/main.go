// Command horizonx is the unified HorizonX CLI.
//
// One binary, subcommands — the opencode-style install target:
//
//	horizonx install server   # install OR upgrade the docker bubble (server+dashboard+postgres+redis)
//	horizonx install agent    # install OR upgrade a host agent (systemd + user + ssh key + udev)
//	horizonx server           # run the control-plane server (foreground)
//	horizonx agent            # run an app-host agent (foreground)
//	horizonx upgrade          # self-update to the latest release
//	horizonx version          # print the build version
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
	case "install":
		err = runInstall(os.Args[2:])
	case "server":
		err = app.RunServer()
	case "agent":
		err = app.RunAgent()
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

// runInstall dispatches `horizonx install <component>`.
func runInstall(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "install requires a component: server | agent")
		fmt.Fprintln(os.Stderr, "  horizonx install server   # install/upgrade the docker bubble")
		fmt.Fprintln(os.Stderr, "  horizonx install agent    # install/upgrade a host agent")
		os.Exit(1)
	}
	switch args[0] {
	case "server":
		opts, err := app.InstallServerFlags(args[1:])
		if err != nil {
			return err
		}
		return app.RunInstallServer(opts)
	case "agent":
		opts, err := app.InstallAgentFlags(args[1:])
		if err != nil {
			return err
		}
		return app.RunInstallAgent(opts)
	default:
		return fmt.Errorf("unknown install target: %s (expected server | agent)", args[0])
	}
}

func usage() {
	fmt.Println(`horizonx — HorizonX control plane

Usage:
  horizonx install server     Install or upgrade the control plane (docker bubble
                              with server + dashboard + postgres + redis at /opt/horizonx)
  horizonx install agent      Install or upgrade a host agent (systemd unit, horizonx
                              user, SSH key, udev rules — never docker)
  horizonx server             Run the control-plane server (API + /metrics + WebSocket)
  horizonx agent              Run an app-host agent (deploys docker-compose apps)
  horizonx migrate            Apply/rollback database migrations (-op=up|down|version|force)
  horizonx version            Print the build version
  horizonx upgrade            Self-update to the latest release
  horizonx help               Show this help

Install the binary (one-liner, auto-sudo):
  curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash

Install flags:
  horizonx install server --yes --host 203.0.113.10 --admin you@x.com
  horizonx install server --generate-only   # write files only, apply manually
  horizonx install agent --server http://host:4858 --token <token>
  horizonx install agent                    # same box: reads creds from /opt/horizonx/.env`)
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
