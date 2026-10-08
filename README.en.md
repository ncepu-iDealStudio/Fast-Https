# Fast-Https Web Server

## Introduction

Fast-Https is a multi-task, high-concurrency web server written in Go. The current tree implements HTTP/1.1, HTTP/2, HTTPS, static files, HTTP and HTTPS reverse proxy, TCP proxy, and WebSocket upgrade. On startup it can create a self-signed certificate for any server whose `listen` value contains `ssl`, which is meant for test access.

Installation packages are published for Windows and Linux. Packages for other platforms are added over time.

The project is part of the openEuler community (<https://gitee.com/src-openeuler/fast-https>).

## Software Architecture

The server accepts connections, parses requests, and writes responses itself. `fast-https.go` is the entry point, `cmd` dispatches subcommands, `config` loads JSON, `init` prepares certificates and caches, and `modules/core` owns listeners and protocol handling. Each configured port runs its own accept loop, then hands the connection to the HTTP/1.1 event loop or the HTTP/2 path.

`location.type` selects the handler. `modules/core/server` blank-imports the static, proxy, rewrite, and dev modules so their handlers are registered before the accept loops start.

| `location.type` / `listen` | Package | Behavior |
| --- | --- | --- |
| `local` | `modules/static` | Serve files from `root` / `index` |
| `proxy` with an `http` address | `modules/proxy` | HTTP reverse proxy |
| `proxy` with an `https` address | `modules/proxy` | HTTPS reverse proxy |
| `rewrite` | `modules/rewrite` | 301 with the configured `Location` |
| `devmod` | `modules/dev_mod` | Fixed development response |
| `listen` contains `tcp` | `modules/proxy_tcp` | Bidirectional TCP proxy, no HTTP parse |
| `listen` contains `ssl` | `modules/core/listener` | TLS |
| `listen` contains `h2` | `modules/core/h2` | HTTP/2 |

A request also passes connection filters (blacklist and rate limit in `modules/safe`), optional Basic auth (`modules/auth`), and, on the static and proxy paths, `modules/appfirewall`.

### Project Structure

```text
fast-https/
├── fast-https.go                 # Entry point, calls cmd.RootCmd()
├── cmd/                          # start / dev / stop / reload / install / uninstall / status
│   ├── commands.go
│   ├── unix_linux_amd64.go       # Linux amd64 daemon
│   ├── unix_other.go             # Daemon no-op on other platforms
│   ├── reload_windows.go         # Windows reload signal
│   └── reload_other.go           # Non-Windows reload signal
├── config/                       # Runtime configuration
│   ├── fast-https.json           # Source of truth
│   ├── conf.d/                   # Extra JSON loaded from http.include
│   ├── mime.json                 # Extension to Content-Type
│   ├── cert/                     # Site certificates; created by init when missing
│   ├── dev.yaml                  # Example only; not loaded at runtime
│   ├── config.go                 # Parse, validate, reload
│   ├── consts.go                 # Paths, version, defaults
│   ├── base_dir.go               # Default base directory ./
│   └── base_dir_rpm.go           # /usr/share/fast-https/ when built with -tags=rpm
├── init/
│   ├── init.go                   # Config, logs, certificates, cache, safe module
│   └── autocert.go               # Self-signed root and site certificates
├── modules/
│   ├── module.go                 # Module registry
│   ├── core/                     # Listeners, events, HTTP/1.1, HTTP/2, request and response
│   │   ├── server/               # Start, signals, hot reload
│   │   ├── listener/             # Listeners from the listen field
│   │   ├── events/               # Connection and request dispatch
│   │   ├── request/  response/
│   │   ├── filters/
│   │   ├── h2/                   # HTTP/2 frames, streams, HPACK
│   │   ├── dynlog/
│   │   └── engine/               # Master/slave registry (not called on the current start path)
│   ├── static/  proxy/  proxy_tcp/  rewrite/  websocket/
│   ├── auth/  safe/  cache/  compress/  appfirewall/  logging/
│   ├── dev_mod/
│   └── workchain/example/        # Registration samples; not loaded at startup
├── output/                       # Logo and startup banners
├── utils/                        # Logger, messages, files, color, RSA
├── monitor/                      # Windows tray program (separate main)
├── httpdoc/root/                 # Default static site and self-signed root certificate
├── test/                         # Unit tests, client tests, dev scripts
├── docker/                       # CentOS 7 and Ubuntu Dockerfiles
├── Dockerfile                    # Multi-stage image, exposes 8080 and 443
├── spec                          # RPM spec; build uses -tags=rpm
├── shell/.acme.sh/               # Bundled acme.sh; the Go process does not call it
├── engine.sh                     # Builds master/slave binaries after rewriting the engine id
├── .goreleaser.yaml              # Linux and Darwin release
└── .goreleaser.windows.yaml      # Windows release
```

### Service Startup

1. `fast-https.go` sets the log level to 4 and runs `cmd.RootCmd()`. With no subcommand, the process starts as `start`.
2. `start` and `dev` pre-check `config/fast-https.json` and its includes, scan ports, and refuse to start when a live pid is already recorded.
3. After the logo, `init.InitSystem()` loads config and `mime.json`, starts the message logger, creates or loads certificates, restores the disk cache and its expiry loop, and initializes the safe module.
4. On Linux amd64, `start` forks a daemon and writes `fast-https.pid`. On Windows it stays in the foreground and writes the same pid file. `dev` does not daemonize, raises the log level to 6, and serves pprof on `0.0.0.0:10000`.
5. `server.ServerInit()` installs the signal loop, opens listeners from config, and registers dynamic logs.
6. Each port accepts in its own goroutine. A `listen` value that contains `h2` uses HTTP/2; every other listener uses the HTTP/1.1 event loop.

Signals:

- `SIGTERM` and `SIGQUIT` stop the process
- `SIGINT` stops the process on Windows and reloads configuration on other platforms
- `reload` reads the pid file, then sends a console Ctrl+C on Windows or `SIGINT` elsewhere

Reload rereads configuration, hot-updates listeners that stay up, closes removed ports, and starts goroutines for new ports.

## Installation

1. Download a release from <https://gitee.com/ncepu-bj/fast-https/releases/>
2. Extract it on the server
3. Edit `config/fast-https.json`. Extra site files belong in the directories listed by `http.include` (default `config/conf.d`)

## Quick Start

### Runtime Config Source

- The runtime source of truth is `config/fast-https.json`
- Startup loads that file, then every JSON path in `http.include` (default `config/conf.d`)
- `config/mime.json` maps file extensions to Content-Type
- `config/dev.yaml` is an example and is not loaded

### Common Commands

```bash
# Foreground or daemon start (no args is the same as start)
go run fast-https.go start

# Development mode: log level 6 and pprof on port 10000
go run fast-https.go dev

go run fast-https.go stop
go run fast-https.go reload
go run fast-https.go install
go run fast-https.go uninstall
```

`status` is a registered subcommand. Its handler currently returns without printing status.

### Minimal Config

`config/fast-https.json`:

```json
{
   "http": {
      "server": [
         {
            "listen": 8080,
            "server_name": "localhost",
            "location": [
               {
                  "url": "/",
                  "type": "local",
                  "root": "./httpdoc/root",
                  "index": ["index.html", "index.htm"]
               }
            ]
         }
      ],
      "include": ["./config/conf.d"]
   }
}
```

The repository default also serves `443 ssl` for `localhost`, using `config/cert/localhost.pem` and `config/cert/localhost-key.pem`.

## Usage

Product manual: <https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?#>

Test layout: [test/readme.md](test/readme.md).

## Building from Source

The Go version is declared in `go.mod` (currently 1.21.5).

1. Server binary

   ```bash
   go build -o fast-https .
   ```

   The RPM spec builds with `-tags=rpm`, which sets the base directory to `/usr/share/fast-https/`.

2. Windows tray program, from `monitor/`

   ```bash
   windres -o monitor.syso monitor.rc
   go build -ldflags "-s -w -H=windowsgui" -o monitor.exe .
   ```

   See [monitor/README.md](monitor/README.md).

3. Linux and Darwin archives

   ```bash
   goreleaser release -f .goreleaser.yaml --snapshot --clean
   ```

4. Windows archive

   ```bash
   goreleaser release -f .goreleaser.windows.yaml --snapshot --clean
   ```

5. Container image

   ```bash
   docker build -t fast-https .
   ```

   The root `Dockerfile` is Alpine-based, copies `config/`, `httpdoc/`, and `logs/`, and exposes 8080 and 443. `docker/Dockerfile_centos7` and `docker/Dockerfile_ubuntu` are separate build files.

## Contributing

1. Fork this repository
2. Create a Feat_xxx branch
3. Commit your changes
4. Open a pull request
