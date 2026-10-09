# Fast-Https Web Server

## Introduction

Fast-Https is a multi-task, high-concurrency web server written in Go. The current tree implements HTTP/1.1, HTTP/2, HTTPS, static files, HTTP and HTTPS reverse proxy, TCP proxy, and WebSocket upgrade. On startup it can create a self-signed certificate for any server whose `listen` value contains `ssl`, which is meant for test access.

Installation packages are published for Windows and Linux. Packages for other platforms are added over time.

The project is part of the openEuler community (<https://gitee.com/src-openeuler/fast-https>).

## Stack, Versions, and Dependencies

The product version is `V1.3.2` (`CURRENT_VERSION` in `config/consts.go`). The module declares **Go 1.27.0** and toolchain **go1.27.1** in `go.mod`. That is the current supported release as of October 2026; Go 1.26 is the other supported line. The root `Dockerfile` builds with `golang:1.27-alpine`. The RPM `spec` downloads Go 1.27.1.

The server parses HTTP/1.1 and HTTP/2 itself. The standard library supplies TCP, `crypto/tls`, and gzip. Development mode serves pprof from the default `net/http` mux on `0.0.0.0:10000`.

Direct runtime dependencies, versions from `go.mod`:

| Dependency | Version | Role |
| --- | --- | --- |
| `github.com/spf13/cobra` | v1.8.1 | CLI: `start`, `dev`, `stop`, `reload`, `install`, `uninstall`, `status` |
| `github.com/spf13/viper` | v1.19.0 | Loads `config/fast-https.json` and `http.include` |
| `github.com/kardianos/service` | v1.2.2 | Install and remove the OS service |
| `github.com/sirupsen/logrus` | v1.9.3 | System, access, error, and safe logs under `logs/` |
| `github.com/fufuok/chanx` | v1.2.2 | Log message queue |
| `golang.org/x/time` | v0.5.0 | Connection rate limit |
| `github.com/microcosm-cc/bluemonday` | v1.0.26 | `xss` HTML cleanup for JSON bodies |
| `github.com/andybalholm/brotli` | v1.1.0 | Brotli compression |
| `github.com/Jxck/hpack` | 2015-07-13 snapshot | HTTP/2 HPACK integer coding and Huffman |
| `github.com/emirpasic/gods` | v1.18.1 | Red-black tree used by the cache |
| `github.com/panjf2000/ants/v2` | v2.9.1 | Goroutine pool |
| `github.com/chenhg5/collection` | 2020-09-25 snapshot | Listener port de-duplication |
| `github.com/getlantern/systray` | v1.2.2 | Windows tray program in `monitor/` |
| `golang.org/x/sys` | v0.21.0 | Windows console Ctrl+Break / Ctrl+C |
| `github.com/fatih/color` | v1.17.0 | Colored console output |

`github.com/stretchr/testify` v1.9.0 is test-only. Release archives are built with GoReleaser (`.goreleaser.yaml`). The snapshot version is `1.3.2-next`.

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

A request also passes connection filters (blacklist and rate limit in `modules/safe`), optional Basic auth (`modules/auth`), and access counting. Static and proxy paths run the application firewall: `xss` returns 403 after a `<script` tag is removed from a JSON object or array; `sql` is registered and does not intercept.

When `listen` contains `h2`, an unknown frame type is discarded and the next frame is read. An illegal HPACK index closes that stream. If the peer flow-control window cannot hold a DATA payload, the write loop stops and the connection closes.

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
│   ├── init.go                   # Config, logs, certificates, cache
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
├── .goreleaser.yaml              # Linux, Darwin, and Windows release
└── .workflow/go-test.yml         # Gitee Go: go test ./... on push and merge requests
```

### Service Startup

1. `fast-https.go` sets the log level to 4 and runs `cmd.RootCmd()`. With no subcommand, the process starts as `start`.
2. `start` and `dev` pre-check `config/fast-https.json` and its includes, scan ports, and refuse to start when a live pid is already recorded.
3. After the logo, `init.InitSystem()` loads config and `mime.json`, starts the message logger, creates or loads certificates, and restores the disk cache and its expiry loop.
4. Outside a container, Linux amd64 `start` forks without changing the working directory and writes the child pid to `fast-https.pid`. Windows, `FASTHTTPS_FOREGROUND=1`, and container marker files keep the process in the foreground and write the same pid file. `dev` does not daemonize, raises the log level to 6, and serves pprof on `0.0.0.0:10000`.
5. `server.ServerInit()` installs the signal loop, opens listeners from config, initializes the safe module, and registers dynamic logs.
6. Each port accepts in its own goroutine. A `listen` value that contains `h2` uses HTTP/2; every other listener uses the HTTP/1.1 event loop.

Signals:

- `SIGTERM` and `SIGQUIT` stop the process
- `SIGINT`, including Ctrl+C in the foreground, reloads configuration on every platform and does not exit
- `reload` reads the pid file and sends `SIGINT`. On Windows it tries Ctrl+Break, then Ctrl+C
- Stop the process with `stop`, which sends `Kill` to the pid

Reload rereads configuration, hot-updates listeners that stay up, closes removed ports, and starts goroutines for new ports. On a removed port, the accept loop and in-flight HTTP/1.1 connections exit.

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
go run fast-https.go status
go run fast-https.go install
go run fast-https.go uninstall
```

`status` reads `fast-https.pid`. It prints the pid and exits 0 when that process is running. It exits non-zero when the pid file is missing or the process is gone.

A successful `reload` reopens the four log files under the current `log_root`. If the new directory cannot be created, logging stays on the previous files.

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

## Packaging

The Go version is declared in `go.mod` (language 1.27.0, toolchain go1.27.1). The product version is **V1.3.2**. The RPM version is `1.3.2`. GoReleaser snapshots use `1.3.2-next`. Run the commands from the repository root.

`LoadFile` turns site roots, certificates, the log directory, and include paths into absolute paths. Images set `FASTHTTPS_FOREGROUND=1`, so the process stays in the foreground and does not fork or change directory.

1. Server binary

   ```bash
   go build -o fast-https .
   ```

   Run it with `start` or `dev`, for example `./fast-https start`.

2. Release archives

   `.goreleaser.yaml` builds Linux and Darwin (`amd64`, `arm64`, `tar.gz`) and Windows (`amd64`, `zip`). The Windows binary uses `-H=windowsgui`. A before hook compiles `monitor/monitor.exe` and the zip contains it. Output goes to `dist/`.

   ```bash
   goreleaser release -f .goreleaser.yaml --snapshot --clean
   ```

   See [monitor/README.md](monitor/README.md) for the tray program.

3. RPM

   `spec` is version `1.3.2` and builds with `-tags=rpm`, which sets the base directory to `/usr/share/fast-https/`.

4. Container images

   All three Dockerfiles compile the current source with `golang:1.27-alpine`. They copy config JSON and the default site, create empty cert, include, and log directories, and expose 8080 and 443.

   ```bash
   docker build -t fast-https .
   docker build -f docker/Dockerfile_ubuntu -t fast-https:ubuntu .
   docker build -f docker/Dockerfile_centos7 -t fast-https:centos7 .
   ```

   Runtime bases are Alpine, Ubuntu 24.04, and CentOS 7. The Ubuntu and CentOS images install into `/usr/local/fast-https` and set `Fast-Https_VERSION=1.3.2`.

## Contributing

1. Fork this repository
2. Create a Feat_xxx branch
3. Commit your changes
4. Open a pull request
