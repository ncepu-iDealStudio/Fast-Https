<h1 align="center">Fast-Https</h1>

<p align="center">A multi-task, high-concurrency web server and reverse proxy written in Go.<br />
One JSON file describes listen ports and locations; a single binary runs it.</p>

<p align="center">
  <img src="https://img.shields.io/badge/version-1.4.0-0E7490?style=flat-square" alt="version" />
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/HTTP%2F2-x%2Fnet%2Fhttp2-0E7490?style=flat-square" alt="HTTP/2" />
  <img src="https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-334155?style=flat-square" alt="platform" />
  <img src="https://img.shields.io/badge/license-GPL--2.0-334155?style=flat-square" alt="license" />
  <img src="https://img.shields.io/badge/Gitee-ncepu--bj%2Ffast--https-C71D23?style=flat-square&logo=gitee&logoColor=white" alt="gitee" />
</p>

<p align="center">
  <img src="https://skillicons.dev/icons?i=go,docker,linux,windows" height="34" alt="tech stack" />
</p>

<p align="center"><a href="README.md">简体中文</a> | English</p>

## Introduction

Fast-Https accepts connections, parses HTTP/1.1, and writes responses itself — no nginx or Caddy underneath. HTTP/2 framing and HPACK are delegated to `golang.org/x/net/http2`; TLS comes from the standard library. Configuration is plain JSON, and `location` blocks read like their nginx counterparts.

How a request travels:

```text
connection → rate limit / IP blacklist → location routing → application firewall → static file / reverse proxy / TCP / WebSocket → response
```

- **Static sites**: `Last-Modified` conditional requests return 304, a single `Range` returns 206/416, and gzip / brotli are negotiated from `Accept-Encoding`. A directory requested without a trailing slash is redirected with 301.
- **Reverse proxy**: pooled upstream connections for HTTP/HTTPS, response cache (map index + LRU), WebSocket upgrade, and bidirectional TCP forwarding.
- **Security**: per-IP token-bucket rate limiting, IP blacklist (IPv6 ranges included), Basic auth, and the GuardianWAF application firewall (sqli/xss/lfi/cmdi/xxe/ssrf, optionally log-only).
- **Operations**: `start` / `stop` / `reload` / `status` / `check` / `dev`, config reload without dropping connections, a Windows tray program, and install as a system service.
- **Packaging**: release archives for Linux / macOS / Windows, an RPM spec, and three Docker base images.

Core dependencies (versions live in `go.mod`; everything else is the standard library):

| Dependency | Role |
| --- | --- |
| `spf13/cobra` | CLI entry point and subcommands |
| `rs/zerolog` | Console output plus system, access, error, and security JSON logs |
| `golang.org/x/net` | HTTP/2 stack |
| `guardianwaf/guardianwaf` | Application firewall engine |
| `klauspost/compress`, `andybalholm/brotli` | gzip and brotli compression |
| `kardianos/service`, `fyne.io/systray` | System service install, Windows tray |

## Quick Start

```bash
# Option 1: release package
# Windows: run fast-https-<version>-windows-<arch>-setup.exe.
# Linux / macOS: download the archive from https://gitee.com/ncepu-bj/fast-https/releases and unpack it.
./fast-https start

# Option 2: build from source (Go 1.27)
go build -o fast-https . && ./fast-https start
```

The shipped `config/fast-https.json` already serves `./httpdoc/root` on port 80 (the same role as nginx's default site). Change `listen` to `8080` or another high port when running as a non-root user.

```json
{
  "http": {
    "server": [
      {
        "listen": 80,
        "server_name": "localhost",
        "location": [
          { "url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html", "index.htm"] }
        ]
      }
    ],
    "include": ["./config/conf.d"]
  }
}
```

```bash
curl -I http://127.0.0.1/    # expect 200 and Server: fast-https
```

The config tolerates `//` and `/* */` comments as well as trailing commas; the shipped file comments every block.

| Command | What it does |
| --- | --- |
| `./fast-https start` | Start the server (`start` is the default). Goes to the background on Linux / macOS; stays in the foreground under containers and systemd |
| `./fast-https dev` | Foreground with log level 6 and pprof on `0.0.0.0:10000` |
| `./fast-https stop` | Stop. Sends `SIGTERM` on Unix for a graceful shutdown; running it again is a no-op |
| `./fast-https reload` | Validate the new config, then reload; a failed validation keeps the old config |
| `./fast-https status` | Read the pid file and report whether the server is running |
| `./fast-https check` | Validate the config only — no listening, no pid file |
| `./fast-https install` | Install as a system service |

## Documentation

| Document | Contents |
| --- | --- |
| [Fast-Https User Guide (English)](https://idealstudio-ncepu.yuque.com/kgagg7/rstqtx) | Deployment and operations: config keys, sites, certificates, proxy, cache, WAF |
| [Fast-Https 产品说明（中文）](https://idealstudio-ncepu.yuque.com/kgagg7/lbeklg) | The same guide in Chinese |
| [docs/usage.md](docs/usage.md) | In-repo source of the usage guide; `docs/usage.html` is generated from it and ships with releases |
| [docs/feature/](docs/feature/README.md) | One page per feature, with config samples and implementation notes |
| [docs/develop/](docs/develop/README.md) | Build environment, tests, code standards, adding config keys, debugging, release |
| [docs/architecture-map.md](docs/architecture-map.md) | Package inventory and dependency direction |
| [docs/nginx-core-gap.md](docs/nginx-core-gap.md) | Feature-by-feature comparison with nginx |

The full index is [docs/README.md](docs/README.md).

## Project Structure

```text
fast-https/
├── fast-https.go        Entry point, calls cmd.RootCmd()
├── cmd/                 CLI: start / dev / stop / reload / status / check / install
│                        daemon_unix.go daemonizes by re-exec, daemon_windows.go is a no-op
├── config/              Config parsing, validation, and reload; fast-https.json is the single source
├── init/                Startup initialization: config, logging, certificates, cache
├── core/                Contracts: Event, handler registry, lifecycle hooks
├── runtime/             Data plane: listeners, connections, request/response, filters, reload
├── modules/             Features: static / proxy / proxy_tcp / websocket / cache /
│                        compress / auth / safe / appfirewall / rewrite
├── infra/               Infrastructure: logging / color / timeutil / banner
├── monitor/             Windows tray program (separate main)
├── httpdoc/root/        Default site and the self-signed root certificate directory
├── docs/                Feature, development, and test documentation
├── tests/               Unit tests and end-to-end tests
└── build/               Build and packaging: goreleaser, Dockerfile, RPM spec, NSIS, systemd unit
```

Layering rule: `core` holds contracts only, `runtime` never imports `modules`, and feature packages are wired at startup by `cmd.RegisterHandlers` / `cmd.RegisterServices`. Details in [docs/develop/2-code-standards.md](docs/develop/2-code-standards.md).

## More

- Packaging and release (binary, RPM, Docker, GoReleaser): [docs/develop/5-release.md](docs/develop/5-release.md)
- Test layout and how to run them: [tests/readme.md](tests/readme.md); coverage and stress tests: [docs/test/](docs/test/README.md)
- Startup flow and signal handling: [docs/about.md](docs/about.md)
- Contributing: fork the repo, create a `Feat_xxx` branch, commit, and open a pull request
- License [GPL-2.0](LICENSE); the project is part of the openEuler community (<https://gitee.com/src-openeuler/fast-https>)
