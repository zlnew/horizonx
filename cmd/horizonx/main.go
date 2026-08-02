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
  horizonx version    Print the build version
  horizonx upgrade    Self-update to the latest release
  horizonx help       Show this help

Install (one-liner):
  curl -fsSL https://raw.githubusercontent.com/zlnew/horizonx/main/install.sh | bash

Setup:
  horizonx setup                  # defaults into ./horizonx-setup
  horizonx setup --host 203.0.113.10 --dir /opt/horizonx`)
}
