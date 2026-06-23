# Fast-Https Web Server

## Introduction

Fast-Https is a multi-task, high-concurrency web server product developed in Go language. It supports mainstream protocols and standards such as HTTP1.1/HTTP2.0, HTTPS, and RPC. It can implement publishing and configuring regular web applications as well as configuring reverse proxy functionality. Notably, the web server has built-in self-signed SSL certificate generation functionality, which greatly facilitates HTTPS protocol access in testing environments.

Currently, we provide installation packages for Windows and Linux platforms, with packages for other platforms coming soon.

The Fast-Https project has joined the Huawei openeuler community (<https://gitee.com/src-openeuler/fast-https>). We welcome more developers to participate and help improve the Fast-Https product, allowing more users to benefit from it.

## Software Architecture

Fast-Https is developed in Go language, implemented based on Golang's net/http package, supporting mainstream protocols and standards such as HTTP1.1/HTTP2.0, HTTPS, and RPC.

Fast-Https adopts a modular design approach, with core server modules supporting horizontal expansion through plugins to enhance server functionality.

### Project Structure

```text
fast-https/
├── cmd/                           # Command-line related code
│   ├── commands.go               # Main command implementation
│   ├── unix_linux_amd64.go      # Linux AMD64 platform-specific code
│   ├── reload_windows.go        # Windows platform reload functionality
│   ├── reload_other.go         # Other platform reload functionality
│   └── unix_other.go           # Other Unix platform code
│
├── config/                      # Configuration related
│   └── config.go               # Configuration check and management
│
├── init/                       # Initialization related
│   └── initialization.go      # Initialization implementation
│
├── modules/                    # Core function modules
│   ├── core/                  # Core functionality
│   │   └── server/           # Server implementation
│   └── httptohttps/          # HTTP to HTTPS conversion module
│
├── output/                    # Output related
│   └── output.go             # Output implementation (including logo)
│
├── utils/                     # Utilities
│   ├── logger/               # Logging module
│   │   └── logger.go        # Logger implementation
│   └── ip/                   # IP related tools
│       └── ip.go            # IP tool implementation
│
├── go.mod                     # Go module definition
├── go.sum                     # Go dependency version lock
├── fast-https.go             # Main program entry
├── monitor.go                # Monitor program (Windows status bar)
├── .goreleaser.yaml          # Linux release configuration
├── .goreleaser.windows.yaml  # Windows release configuration
└── README.md                 # Project documentation
```

### Service Startup Process

The service startup process in Fast-Https follows a well-structured flow:

1. Entry Point (`fast-https.go`)
   - Initializes logging system
   - Executes root command through `cmd.RootCmd()`

2. Command Processing (`cmd/commands.go`)
   - Parses command-line arguments
   - Handles different commands: start, stop, reload, etc.
   - For 'start' command:
     - Performs pre-checks (port availability)
     - Initializes server components

3. Server Initialization (`modules/core/server/server.go`)
   - Creates new server instance
   - Sets up signal handlers (SIGTERM, SIGINT, SIGQUIT)
   - Initializes listeners based on configuration
   - Registers core modules

4. Listener Setup
   - Configures ports for HTTP/HTTPS
   - Initializes connection handlers
   - Supports both HTTP/1.1 and HTTP/2

5. Module Initialization
   - Initializes security modules
   - Sets up dynamic logging
   - Prepares other core functionalities

6. Service Running
   - Starts goroutines for each listener
   - Handles incoming connections
   - Manages graceful shutdown and reload

Signal Handling:

- SIGTERM: Graceful shutdown
- SIGINT (Ctrl+C):
  - Foreground mode: Stop service
  - Daemon mode: Reload configuration
- SIGQUIT: Stop service

## Installation Guide

1. Get the appropriate version and installation package from <https://gitee.com/ncepu-bj/fast-https/releases/>
2. Extract the installation package to the target directory on your server
3. Modify the configuration file

## Quick Start

### Runtime Config Source

- The runtime source of truth is `config/fast-https.json`
- At startup, the program loads `config/fast-https.json`, then loads extra json files from paths listed in `http.include` (for example `config/conf.d`)
- `config/dev.yaml` is currently a development example file and is not auto-loaded at runtime

### Common Commands

```bash
# Start in foreground mode
go run fast-https.go start

# Start in development mode (debug-level logs + pprof port)
go run fast-https.go dev

# Stop service
go run fast-https.go stop

# Reload config
go run fast-https.go reload
```

## Usage Instructions

See documentation: <https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?#> 'Fast-Https Product Manual'

## Building from Source

1. Build Windows status bar control program
    go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go

2. Build Linux platform release package
   goreleaser release -f .goreleaser.yaml --snapshot --clean
   If needed, modify the corresponding build configuration file "goreleaser.yaml"

3. Build Windows platform release package
   goreleaser release -f .goreleaser.windows.yaml --snapshot --clean

## Contributing

1. Fork this repository
2. Create a new Feat_xxx branch
3. Submit your code
4. Create a new Pull Request
