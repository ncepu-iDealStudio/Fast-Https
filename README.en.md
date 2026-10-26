# Fast-Https Web Server

## Introduction

Fast-Https is a multi-task, high-concurrency web server written in Go. The current tree implements HTTP/1.1, HTTP/2, HTTPS, static files (conditional 304, Range, gzip/brotli negotiation), HTTP and HTTPS reverse proxy (upstream connection pool, response cache), TCP proxy, WebSocket upgrade, IP blacklist with tiered rate limiting, an application firewall (GuardianWAF engine), and zero-downtime config reload. On startup it can create a self-signed certificate for any server whose `listen` value contains `ssl`, which is meant for test access.

Installation packages are published for Windows and Linux. Packages for other platforms are added over time.

The project is part of the openEuler community (<https://gitee.com/src-openeuler/fast-https>).

## Stack, Versions, and Dependencies

The product version is `V1.4.0` (`CURRENT_VERSION` in `config/consts.go`). The module declares **Go 1.27.0** and toolchain **go1.27.1** in `go.mod`. That is the current supported release as of October 2026; Go 1.26 is the other supported line. The root `Dockerfile` builds with `golang:1.27-alpine`. The RPM `spec` downloads Go 1.27.1.

The server parses HTTP/1.1 itself; the HTTP/2 stack (framing, HPACK, flow control) is delegated to `golang.org/x/net/http2`, with only the connection intake and `core.Event` adapter kept in-tree. The standard library supplies TCP, `crypto/tls`, and config parsing (the config file is plain JSON). Development mode serves pprof from the default `net/http` mux on `0.0.0.0:10000`.

Direct runtime dependencies, versions from `go.mod`:

| Dependency | Version | Role |
| --- | --- | --- |
| `github.com/guardianwaf/guardianwaf` | v1.0.0 | Application firewall engine (tokenizer scoring: sqli/xss/lfi/cmdi/xxe/ssrf) |
| `github.com/spf13/cobra` | v1.10.2 | CLI: `start`, `dev`, `stop`, `reload`, `install`, `uninstall`, `status` |
| `github.com/rs/zerolog` | v1.35.1 | Console plus system, access, error, and safe JSON logs under `logs/` |
| `golang.org/x/net` | v0.61.0 | HTTP/2 stack (`http2.Server` / `Transport` / HPACK) |
| `github.com/klauspost/compress` | v1.17.2 | gzip compression (drop-in stdlib replacement, faster) |
| `github.com/andybalholm/brotli` | v1.2.7 | brotli compression (`zip: ["br"]`, `Content-Encoding: br`) |
| `golang.org/x/time` | v0.16.0 | Connection rate limit |
| `github.com/kardianos/service` | v1.3.0 | Install and remove the OS service |
| `fyne.io/systray` | v1.12.2 | Windows tray program in `monitor/` (maintained fork of getlantern/systray) |
| `golang.org/x/sys` | v0.49.0 | Windows console Ctrl+Break / Ctrl+C |
| `github.com/fatih/color` | v1.19.0 | Colored console output |

Everything else comes from the standard library: TCP listening, `crypto/tls`, config parsing (`encoding/json` — the config file is plain JSON), a plain map for the cache index, `slices` for listener port de-duplication, and the in-house HTTP/1.1 parser. `github.com/stretchr/testify` v1.12.1 is test-only. Release archives are built with GoReleaser (`.goreleaser.yaml`). The snapshot version is `1.4.0-next`.

## Software Architecture

The server accepts connections, parses requests, and writes responses itself. `fast-https.go` is the entry point, `cmd` dispatches subcommands, `config` loads JSON, `init` prepares certificates and caches, and `core` owns listeners and protocol handling. Each configured port runs its own accept loop, then hands the connection to the HTTP/1.1 event loop or the HTTP/2 path.

`location.type` selects the handler. `start` and `dev` call `cmd.RegisterHandlers` before listening, which registers the static, proxy, rewrite, and dev modules, and `cmd.RegisterServices`, which installs the blacklist, rate limit, authentication, TCP proxy, and WebSocket hooks.

| `location.type` / `listen` | Package | Behavior |
| --- | --- | --- |
| `local` | `modules/static` | Serve files from `root` / `index` |
| `proxy` with an `http` address | `modules/proxy` | HTTP reverse proxy |
| `proxy` with an `https` address | `modules/proxy` | HTTPS reverse proxy |
| `rewrite` | `modules/rewrite` | 301 with the configured `Location` |
| `devmod` | `modules/dev_mod` | Fixed development response |
| `listen` contains `tcp` | `modules/proxy_tcp` | Bidirectional TCP proxy, no HTTP parse |
| `listen` contains `ssl` | `runtime/listener` | TLS |
| `listen` contains `h2` | `runtime/events` | Delegated to `golang.org/x/net/http2` |

A request passes connection filters (per-IP token-bucket rate limit and IP blacklist in `modules/safe`), per-location access counting, optional Basic auth (`modules/auth`), and then the application firewall (`modules/appfirewall`, GuardianWAF scoring engine — active for locations with `appfirewall` or a `waf` block; hits return 403; `waf.mode: monitor` only logs; without configuration the engine is never built and the check is free). The firewall check runs once per request from the shared event handler, so every location type is covered.

`HEAD` keeps `Content-Length` and `Content-Type` and does not write a body. A body with `Content-Length` is read to that length. `Transfer-Encoding: chunked` is read chunk by chunk; a bad chunk or a body over `http.servers_limit.max_body_size` closes the connection. Static responses carry `Last-Modified`: a `If-Modified-Since` not older than the file mtime gets a 304; a single-segment `Range` gets 206 + `Content-Range` (malformed or out-of-range gets 416), and the body is written via `io.Copy` (sendfile zero-copy on Linux). The location `zip` setting drives compression negotiation: `["gzip"]` serves gzip only, `["br"]` serves brotli only, `["gzip","br"]` prefers brotli when the client accepts it and falls back to gzip; compressed responses carry `Vary: Accept-Encoding`. Optional `error_page.not_found` and `error_page.proxy` replace the 404 body and the upstream dial-failure body when those files can be read. A missing file keeps the default response.

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
├── core/                         # Contracts: Event, handler registry, lifecycle hooks
│   ├── event.go                  # Event / RRcircle / RRcircleHandler
│   ├── registry.go               # Map registry: RegisterHandler / Handler
│   ├── services.go               # Connect, listen, auth and shutdown hooks
│   ├── handlers.go               # Missing-handler check and type names
│   └── commands.go               # Default command parser
├── runtime/                      # Data-plane runtime; must not import modules
│   ├── server/                   # Start, signals, hot reload
│   ├── listener/                 # Listeners from the listen field
│   ├── events/                   # Connection and request dispatch
│   ├── request/  response/
│   ├── filters/
│   └── dynlog/                   # Access-log field assembly
├── modules/                      # Pluggable features
│   ├── static/  proxy/  proxy_tcp/  rewrite/  websocket/
│   ├── auth/  safe/  cache/  compress/  appfirewall/
│   └── dev_mod/
├── infra/                        # Infrastructure; must not depend on config
│   ├── logging/                  # Single log exit: console plus four JSON files
│   └── files/  color/  timeutil/
├── output/                       # Logo and startup banners
├── monitor/                      # Windows tray program (separate main)
├── httpdoc/root/                 # Default static site and self-signed root certificate
├── tests/                        # Automated tests; scripts/ and single/ are manual helpers, not go test
├── docker/                       # CentOS 7 and Ubuntu Dockerfiles
├── Dockerfile                    # Multi-stage image, exposes 8080 and 443
├── spec                          # RPM spec; build uses -tags=rpm
├── shell/.acme.sh/               # Bundled acme.sh; the Go process does not call it
├── engine.sh                     # Unused path: rewrites the engine id and builds master/slave; not go build .
├── .goreleaser.yaml              # Linux, Darwin, and Windows release
└── .workflow/go-test.yml         # Gitee Go: go test ./... on push and merge requests
```

### Service Startup

1. `fast-https.go` runs `cmd.RootCmd()`. With no subcommand, the process starts as `start`. Console log level defaults to `info` in `infra/logging`.
2. `start` and `dev` pre-check `config/fast-https.json` and its includes, scan ports, and refuse to start when a live pid is already recorded.
3. After the logo, `init.InitSystem()` loads config and `mime.json`, starts the message logger, creates or loads certificates, and restores the disk cache and its expiry loop.
4. Outside a container, Linux amd64 `start` forks without changing the working directory and writes the child pid to `fast-https.pid`. Windows, `FASTHTTPS_FOREGROUND=1`, and container marker files keep the process in the foreground and write the same pid file. `dev` does not daemonize, raises the console log level to `trace`, and serves pprof on `0.0.0.0:10000`.
5. `server.ServerInit()` installs the signal loop, opens listeners from config, initializes the safe module, and registers dynamic logs.
6. Each port accepts in its own goroutine. A `listen` value that contains `h2` uses HTTP/2; every other listener uses the HTTP/1.1 event loop.

Signals:

- `SIGTERM` and `SIGQUIT` stop accepting, wait up to 5 seconds for accepted requests to finish, then exit
- `SIGINT`: in daemon mode it reloads configuration without exiting; in `dev` foreground mode it shuts down gracefully (Ctrl+C exits, 2026-10-25)
- `reload` reads the pid file and sends `SIGINT`. On Windows it tries Ctrl+Break, then Ctrl+C
- Stop the process with `stop`, which sends `Kill` to the pid and does not wait for in-flight requests

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

# Development mode: console log level trace and pprof on port 10000
go run fast-https.go dev

go run fast-https.go stop
go run fast-https.go reload
go run fast-https.go status
go run fast-https.go check
go run fast-https.go install
go run fast-https.go uninstall
```

`status` reads `fast-https.pid`. It prints the pid and exits 0 when that process is running. It exits non-zero when the pid file is missing or the process is gone.

`check` validates `config/fast-https.json` and `http.include`. It does not listen and does not write a pid file. Success prints `config ok` and exits 0. Failure prints the field or include path and exits non-zero. `start` still runs the same check before listening.

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

In-repo docs ([docs/README.md](docs/README.md) is the index):

- [docs/feature/](docs/feature/README.md): 23 per-feature pages (0-19 implemented, 20-22 planned), each with config examples and implementation notes
- [docs/develop/](docs/develop/README.md): developer guides (environment, testing, standards, config development, debugging, release) plus historical plans under `develop/plans/`
- [docs/architecture-map.md](docs/architecture-map.md): current package inventory and dependency directions

Test layout: [tests/readme.md](tests/readme.md).

## Packaging

The Go version is declared in `go.mod` (language 1.27.0, toolchain go1.27.1). The product version is **V1.4.0**. The RPM version is `1.4.0`. GoReleaser snapshots use `1.4.0-next`. Run the commands from the repository root.

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

   `spec` is version `1.4.0` and builds with `-tags=rpm`, which sets the base directory to `/usr/share/fast-https/`.

4. Container images

   All three Dockerfiles compile the current source with `golang:1.27-alpine`. They copy config JSON and the default site, create empty cert, include, and log directories, and expose 8080 and 443.

   ```bash
   docker build -t fast-https .
   docker build -f docker/Dockerfile_ubuntu -t fast-https:ubuntu .
   docker build -f docker/Dockerfile_centos7 -t fast-https:centos7 .
   ```

   Runtime bases are Alpine, Ubuntu 24.04, and CentOS 7. The Ubuntu and CentOS images install into `/usr/local/fast-https` and set `Fast-Https_VERSION=1.4.0`.

## Contributing

1. Fork this repository
2. Create a Feat_xxx branch
3. Commit your changes
4. Open a pull request
