<div align="center">

# `</>` SLAYER L7

### Multi-Protocol Layer 7 Stress Testing & Network Resilience Toolkit

<p>
  <strong>Controlled infrastructure testing · Resilience engineering · Security research</strong>
</p>

<br>

<p>
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.21+">
  </a>
  <a href="https://github.com/nguyenphanno/Slayer-L7">
    <img src="https://img.shields.io/github/stars/nguyenphanno/Slayer-L7?style=for-the-badge&logo=github&label=Stars" alt="GitHub Stars">
  </a>
  <a href="https://github.com/nguyenphanno/Slayer-L7/network/members">
    <img src="https://img.shields.io/github/forks/nguyenphanno/Slayer-L7?style=for-the-badge&logo=github&label=Forks" alt="GitHub Forks">
  </a>
  <a href="https://github.com/nguyenphanno/Slayer-L7/issues">
    <img src="https://img.shields.io/github/issues/nguyenphanno/Slayer-L7?style=for-the-badge&logo=github&label=Issues" alt="GitHub Issues">
  </a>
  <a href="https://github.com/nguyenphanno/Slayer-L7/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/nguyenphanno/Slayer-L7?style=for-the-badge&logo=opensourceinitiative&logoColor=white" alt="MIT License">
  </a>
</p>

<p>
  <a href="#overview">Overview</a> ·
  <a href="#highlights">Highlights</a> ·
  <a href="#capability-matrix">Capabilities</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#test-methods">Test Methods</a> ·
  <a href="#observability">Observability</a> ·
  <a href="#security--legal">Security</a>
</p>

</div>

---

> [!WARNING]
> ## Authorized Testing Only
>
> Slayer L7 is intended exclusively for infrastructure that you own or have explicit authorization to assess.
>
> High-volume, connection-exhaustion, parser-stress and protocol-edge-case scenarios can affect service availability. Always define a testing scope, establish a controlled testing window and monitor the target environment during execution.

---

## Table of Contents

- [Overview](#overview)
- [Highlights](#highlights)
- [Capability Matrix](#capability-matrix)
- [Architecture](#architecture)
- [Execution Model](#execution-model)
- [Requirements](#requirements)
- [Quick Start](#quick-start)
- [CLI Configuration](#cli-configuration)
- [Test Methods](#test-methods)
  - [HTTP & Application](#http--application)
  - [Advanced L7](#advanced-l7)
  - [Slow & Connection](#slow--connection)
  - [HTTP/2](#http2)
  - [Web Application](#web-application)
  - [Input Validation](#input-validation)
  - [Network & Infrastructure](#network--infrastructure)
  - [Minecraft / Netty](#minecraft--netty)
- [Proxy Support](#proxy-support)
- [Testing Workflow](#testing-workflow)
- [Observability](#observability)
- [Performance](#performance)
- [Repository Intelligence](#repository-intelligence)
- [Project Structure](#project-structure)
- [Developer Reference](#developer-reference)
- [Troubleshooting](#troubleshooting)
- [Roadmap](#roadmap)
- [Security & Legal](#security--legal)
- [Contributing](#contributing)
- [License](#license)

---

# Overview

**Slayer L7** is a Go-based multi-protocol stress-testing and network-resilience toolkit designed for **authorized infrastructure assessments**.

The project provides a compact executable capable of generating controlled workloads across application, transport and protocol layers. It can be used by system administrators, infrastructure engineers and security researchers to study system behavior under increased traffic, connection pressure and protocol-specific edge cases.

### What Slayer L7 Provides

- High-concurrency execution using Go goroutines
- HTTP/1.1 and HTTP/2 workload generation
- Configurable worker concurrency
- Configurable test duration
- Protocol-specific testing scenarios
- HTTP / SOCKS4 / SOCKS5 proxy support
- User-Agent rotation
- Runtime diagnostics
- Network workload testing
- Application-layer resilience scenarios
- Parser and protocol robustness testing
- Controlled performance testing

---

# Highlights

<table>
<tr>
<td width="50%" valign="top">

### ⚡ High Concurrency

- Goroutine-based execution
- Configurable worker pool
- Lightweight Go runtime
- Long-running test sessions
- Adjustable workload intensity
- Low-overhead execution

</td>

<td width="50%" valign="top">

### 🌐 Multi-Protocol

- HTTP/1.1
- HTTP/2
- TCP
- UDP
- ICMP
- DNS
- WebSocket
- GraphQL
- Minecraft / Netty

</td>
</tr>

<tr>
<td width="50%" valign="top">

### 🔬 Security Research

- WAF behavior analysis
- Rate-limit validation
- Reverse-proxy testing
- Parser robustness
- Input validation
- Connection-limit testing
- HTTP/2 behavior analysis

</td>

<td width="50%" valign="top">

### 📊 Resilience Engineering

- Controlled load progression
- Baseline measurements
- Runtime diagnostics
- Error monitoring
- Latency analysis
- Infrastructure bottleneck identification

</td>
</tr>
</table>

---

# Capability Matrix

| Layer | Protocol / Technology | Coverage |
|:---|:---|:---:|
| L7 | HTTP/1.1 | ✓ |
| L7 | HTTP/2 | ✓ |
| L7 | WebSocket | ✓ |
| L7 | GraphQL | ✓ |
| L7 | XML / SOAP | ✓ |
| L7 | Multipart / Forms | ✓ |
| L4 | TCP | ✓ |
| L4 | UDP | ✓ |
| L3 | ICMP | ✓ |
| Network | DNS | ✓ |
| Game | Minecraft / Netty | ✓ |
| Proxy | HTTP | ✓ |
| Proxy | SOCKS4 | ✓ |
| Proxy | SOCKS5 | ✓ |

---

# Architecture

Slayer L7 uses a compact Go architecture centered around a single executable.

```text
                         ┌───────────────────────┐
                         │       CLI Input       │
                         │ target / method /     │
                         │ workers / duration    │
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │    Configuration      │
                         │      Validation       │
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │   Scenario Registry   │
                         │    Method Selection   │
                         └───────────┬───────────┘
                                     │
                  ┌──────────────────┼──────────────────┐
                  │                  │                  │
                  ▼                  ▼                  ▼
           ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
           │ HTTP / L7   │    │ TCP / UDP   │    │ Minecraft   │
           │ Test Engine │    │ Test Engine │    │ Test Engine │
           └──────┬──────┘    └──────┬──────┘    └──────┬──────┘
                  │                  │                  │
                  └──────────────────┼──────────────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │    Worker Controller  │
                         │    Goroutine Pool     │
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │ Transport / Proxy     │
                         │ HTTP / SOCKS4 / SOCKS5│
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │   Authorized Target   │
                         └───────────────────────┘
````

---

# Execution Model

```text
Configuration
     │
     ▼
Validation
     │
     ▼
Method Selection
     │
     ▼
Worker Initialization
     │
     ├── Goroutine 01
     ├── Goroutine 02
     ├── Goroutine 03
     ├── Goroutine ...
     └── Goroutine N
             │
             ▼
       Test Workload
             │
             ▼
       Test Endpoint
             │
             ▼
      Runtime Metrics
             │
             ▼
       Error Analysis
```

The workload is controlled through the CLI, allowing concurrency and duration to be adjusted according to the capacity of the test environment.

---

# Design Principles

| Principle       | Description                                    |
| :-------------- | :--------------------------------------------- |
| **Controlled**  | Explicit target, duration and concurrency      |
| **Repeatable**  | Consistent scenario configuration              |
| **Observable**  | Runtime diagnostics and measurable behavior    |
| **Portable**    | Single Go executable                           |
| **Lightweight** | Goroutine-based concurrency                    |
| **Extensible**  | Protocol scenarios can evolve independently    |
| **Defensive**   | Intended for authorized resilience assessments |

---

# Requirements

| Component    | Requirement                         |
| :----------- | :---------------------------------- |
| Go           | `1.21+`                             |
| Linux        | Supported                           |
| macOS        | Supported                           |
| Windows      | WSL recommended                     |
| Architecture | amd64 / arm64 where supported by Go |

---

# Quick Start

## Clone

```bash
git clone https://github.com/nguyenphanno/Slayer-L7.git
cd Slayer-L7
```

## Dependencies

```bash
go mod tidy
```

## Build

```bash
go build -ldflags="-s -w" -o slayer main.go
```

Linux / macOS:

```bash
chmod +x slayer
```

## Verify

```bash
./slayer --help
```

---

# CLI Configuration

## Command Syntax

```text
./slayer -t <TARGET> -m <METHOD> -w <WORKERS> -d <DURATION> [OPTIONS]
```

## Options

| Flag | Type     |   Default | Description                      |
| :--- | :------- | --------: | :------------------------------- |
| `-t` | `string` |         — | Target URL or endpoint           |
| `-m` | `string` | `httpget` | Test method                      |
| `-w` | `int`    |    `2048` | Concurrent workers               |
| `-d` | `int`    |      `30` | Test duration in seconds         |
| `-p` | `string` |         — | Proxy list path                  |
| `-r` | `int`    |       `0` | Delay per worker in milliseconds |
| `-v` | `bool`   |   `false` | Enable verbose logging           |

---

## Example

Use only an endpoint that is explicitly within your authorized test scope:

```bash
./slayer \
  -t https://your-staging-environment.example \
  -m httpget \
  -w 100 \
  -d 30
```

Start with a conservative concurrency level and gradually increase the workload while monitoring the environment.

---

# Test Methods

Slayer L7 contains a broad collection of application, protocol and infrastructure test scenarios.

> [!NOTE]
> Scenario availability and exact behavior depend on the implementation in the current repository version. Always review the source before executing a method in a production-like environment.

---

## HTTP & Application

| Method           | Description                  |
| :--------------- | :--------------------------- |
| `httpget`        | HTTP GET request workload    |
| `httppost`       | HTTP POST request workload   |
| `httpoptions`    | OPTIONS / CORS handling      |
| `httpdelete`     | DELETE request handling      |
| `httpput`        | PUT request handling         |
| `httphead`       | HEAD request handling        |
| `http_json`      | JSON parser workload         |
| `http_multipart` | Multipart form handling      |
| `http_form_bomb` | Large parameter-set handling |
| `http_payload`   | Request-body handling        |
| `http_empty`     | Empty request handling       |
| `http_poll`      | Long-polling workload        |

---

## Advanced L7

| Method              | Description                              |
| :------------------ | :--------------------------------------- |
| `mixpost`           | Multi-format Content-Type handling       |
| `mixheavy`          | XML/SOAP parser robustness               |
| `mixbunchof`        | Multipart boundary handling              |
| `mixregex`          | Regular-expression processing resilience |
| `mixchunked`        | Chunked transfer handling                |
| `slowpost_bomb`     | Slow POST behavior                       |
| `graphql_recursion` | GraphQL parser recursion testing         |

---

## Slow & Connection

| Method           | Description                  |
| :--------------- | :--------------------------- |
| `rudy`           | Slow request-body behavior   |
| `slowloris`      | Partial HTTP header handling |
| `chunkpost`      | Chunked transfer behavior    |
| `http_slow_read` | Slow response consumption    |
| `http_dead_conn` | Idle connection handling     |
| `http_bad_start` | Incomplete request handling  |

---

## HTTP/2

| Method              | Description                         |
| :------------------ | :---------------------------------- |
| `rapidreset`        | HTTP/2 reset behavior               |
| `rapidreset_adv`    | Advanced HTTP/2 reset behavior      |
| `h2continuation`    | CONTINUATION frame handling         |
| `http_h2_flood`     | HTTP/2 stream-capacity testing      |
| `h2_window`         | HTTP/2 flow-control behavior        |
| `h2_cont_bomb`      | HTTP/2 CONTINUATION stress scenario |
| `smuggle_clte`      | CL/TE parser-alignment testing      |
| `smuggle_tete`      | TE/TE parser-alignment testing      |
| `http_conn_smuggle` | Persistent connection validation    |

---

## Web Application

| Method         | Description                  |
| :------------- | :--------------------------- |
| `cfbypass`     | CDN / WAF behavior research  |
| `cachebypass`  | Cache-key behavior testing   |
| `cache_poison` | Cache-key validation         |
| `wsflood`      | WebSocket connection testing |
| `headerflood`  | Large HTTP header handling   |
| `cookiebomb`   | Cookie-size handling         |
| `range`        | Range-request processing     |
| `malformed`    | URI parser robustness        |
| `http_auth`    | Authentication workload      |
| `http_ntlm`    | NTLM authentication behavior |

---

## Input Validation

| Method           | Description                 |
| :--------------- | :-------------------------- |
| `xss_probe`      | XSS input validation        |
| `sqli_probe`     | SQL injection validation    |
| `path_traversal` | Path normalization testing  |
| `redos`          | Regex processing resilience |
| `graphql_batch`  | GraphQL workload testing    |
| `zstd_bomb`      | Decompression-limit testing |
| `pingback`       | XML-RPC endpoint testing    |
| `http_ssrf`      | SSRF validation probe       |

---

## Network & Infrastructure

| Method               | Description                     |
| :------------------- | :------------------------------ |
| `tcp_connect`        | TCP connection-capacity testing |
| `tcp_slow`           | Slow TCP transfer testing       |
| `tcp_payload`        | TCP payload testing             |
| `tcp_socket_exhaust` | Socket exhaustion testing       |
| `tcp_urg`            | TCP urgent-pointer behavior     |
| `tcp_oob`            | TCP out-of-band behavior        |
| `tcp_fin`            | TCP FIN handling                |
| `tcp_half_open`      | Half-open connection behavior   |
| `tcp_fragmented`     | Fragmented TCP handling         |
| `tcp_large_connect`  | Large TCP payload testing       |
| `udp_flood`          | UDP throughput testing          |
| `udp_amp`            | UDP amplification behavior      |
| `udp_dns`            | UDP DNS resolver workload       |
| `udp_memcached`      | Memcached protocol behavior     |
| `dns_query`          | DNS resolver workload           |
| `dns_nx`             | NXDomain handling               |
| `icmp_flood`         | ICMP echo workload              |
| `icmp_large`         | Large ICMP packet handling      |
| `syn_flood`          | SYN handling validation         |
| `ack_flood`          | Stateful firewall validation    |

---

## Minecraft / Netty

| Method               | Description                  |
| :------------------- | :--------------------------- |
| `mc_ping`            | Server-list ping testing     |
| `mc_bot`             | Authentication-flow testing  |
| `mc_bigpacket`       | Frame-decoder robustness     |
| `mc_legacy`          | Legacy protocol handling     |
| `mc_hold`            | Connection-limit testing     |
| `mc_bungee`          | Proxy-forwarding validation  |
| `mc_data`            | Data packet handling         |
| `mc_ext_login`       | Extended login-flow testing  |
| `mc_varint`          | VarInt frame-decoder testing |
| `mc_ping_var`        | Ping variation testing       |
| `mc_data_spam`       | Data packet workload         |
| `mc_profile_flood`   | Profile lookup workload      |
| `mc_handshake_flood` | Handshake workload           |
| `mc_nullping`        | Null-ping handling           |
| `mc_account_fill`    | Account-flow testing         |
| `mc_spam_pkt`        | Packet workload              |
| `mc_bad_pkt`         | Invalid packet handling      |
| `mc_random_pkt`      | Random packet handling       |
| `mc_slow_read`       | Slow-read connection testing |

---

# Proxy Support

Slayer L7 supports proxy-based testing using controlled proxy infrastructure.

Supported formats include:

```text
host:port
```

and, where supported by the implementation:

```text
username:password@host:port
```

Repository datasets:

```text
proxy.txt
socks5.txt
```

User-Agent data:

```text
useragent.txt
```

### Recommended Practice

Use proxies only when:

* You own the proxy infrastructure;
* You have permission to use it;
* The traffic source is part of the authorized test scope;
* Proxy credentials are not committed to Git.

> [!CAUTION]
> Never commit private proxy lists, authentication credentials, API keys or other secrets to the repository.

---

# Testing Workflow

A controlled resilience assessment should follow a gradual progression.

```text
┌─────────────────────┐
│ 1. Define Scope     │
│ Target + Test Window│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 2. Establish        │
│ Baseline Metrics    │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 3. Start Low Load   │
│ Conservative Workers│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 4. Gradually Ramp   │
│ Concurrency / Rate  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 5. Monitor System   │
│ CPU / RAM / Network │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 6. Analyze Results  │
│ Bottlenecks / Errors│
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 7. Harden & Retest  │
└─────────────────────┘
```

---

# Observability

A useful resilience assessment should monitor both the workload generator and the target environment.

### Infrastructure Metrics

* CPU utilization
* Memory utilization
* Network bandwidth
* Active connections
* File descriptors
* Ephemeral port usage
* Kernel socket limits

### Application Metrics

* Request latency
* Error rate
* HTTP status codes
* Application exceptions
* Database utilization
* Upstream latency
* Queue depth

### Security / Edge Metrics

* WAF events
* Reverse-proxy logs
* Rate-limit events
* CDN behavior
* Connection limits
* Firewall counters

---

# Performance

Slayer L7 is built around Go's lightweight concurrency model.

## Goroutine-Based Execution

```text
                    Test Controller
                           │
                    ┌──────┴──────┐
                    │ Worker Pool │
                    └──────┬──────┘
                           │
             ┌─────────────┼─────────────┐
             ▼             ▼             ▼
         Worker 1      Worker 2      Worker N
             │             │             │
             └─────────────┼─────────────┘
                           ▼
                    Network Layer
                           │
                           ▼
                     Test Target
```

Actual throughput depends on both the testing machine and target environment.

Important limiting factors include:

* CPU
* RAM
* File descriptors
* Ephemeral ports
* Network bandwidth
* Kernel socket limits
* Proxy capacity
* Target-side rate limits
* Application architecture

> Higher worker counts do not necessarily produce better results. Controlled load progression generally produces more useful capacity measurements.

---

# Repository Intelligence

## Project Status

<p align="center">

<a href="https://github.com/nguyenphanno/Slayer-L7">
  <img src="https://img.shields.io/github/last-commit/nguyenphanno/Slayer-L7?style=flat-square&label=Last%20Commit">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7">
  <img src="https://img.shields.io/github/repo-size/nguyenphanno/Slayer-L7?style=flat-square&label=Repository%20Size">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7">
  <img src="https://img.shields.io/github/languages/top/nguyenphanno/Slayer-L7?style=flat-square&label=Top%20Language">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7">
  <img src="https://img.shields.io/github/commit-activity/y/nguyenphanno/Slayer-L7?style=flat-square&label=Commit%20Activity">
</a>

</p>

## Repository Metrics

| Metric             |                                                 Status                                                |
| :----------------- | :---------------------------------------------------------------------------------------------------: |
| ⭐ Stars            |         ![Stars](https://img.shields.io/github/stars/nguyenphanno/Slayer-L7?style=flat-square)        |
| 🍴 Forks           |         ![Forks](https://img.shields.io/github/forks/nguyenphanno/Slayer-L7?style=flat-square)        |
| 👁 Watchers        |      ![Watchers](https://img.shields.io/github/watchers/nguyenphanno/Slayer-L7?style=flat-square)     |
| 🐛 Open Issues     |        ![Issues](https://img.shields.io/github/issues/nguyenphanno/Slayer-L7?style=flat-square)       |
| 🔀 Open PRs        |        ![PRs](https://img.shields.io/github/issues-pr/nguyenphanno/Slayer-L7?style=flat-square)       |
| 💾 Repository Size |       ![Size](https://img.shields.io/github/repo-size/nguyenphanno/Slayer-L7?style=flat-square)       |
| 📅 Last Commit     |     ![Commit](https://img.shields.io/github/last-commit/nguyenphanno/Slayer-L7?style=flat-square)     |
| 📈 Commit Activity | ![Activity](https://img.shields.io/github/commit-activity/y/nguyenphanno/Slayer-L7?style=flat-square) |
| 📝 License         |       ![License](https://img.shields.io/github/license/nguyenphanno/Slayer-L7?style=flat-square)      |

---

# Build & Quality

<p align="center">

<img src="https://img.shields.io/badge/Language-Go-00ADD8?style=flat-square&logo=go&logoColor=white">

<img src="https://img.shields.io/badge/Architecture-Goroutines-2EA44F?style=flat-square&logo=go&logoColor=white">

<img src="https://img.shields.io/badge/Protocols-Multi--Protocol-6E56CF?style=flat-square">

<img src="https://img.shields.io/badge/License-MIT-0969DA?style=flat-square">

</p>

When CI workflows are configured, add their badges here:

```markdown
[![Build](https://img.shields.io/github/actions/workflow/status/nguyenphanno/Slayer-L7/go.yml?style=flat-square&label=Build)](https://github.com/nguyenphanno/Slayer-L7/actions)
```

> Update `go.yml` to match the actual workflow filename in `.github/workflows/`.

---

# Development Activity

```text
Repository
    │
    ├── Commits
    ├── Contributors
    ├── Pull Requests
    ├── Issues
    ├── Releases
    └── Activity
```

GitHub repository statistics can be integrated into a generated project dashboard using the GitHub REST API.

Useful endpoints include:

```text
GET /repos/{owner}/{repo}

GET /repos/{owner}/{repo}/contributors

GET /repos/{owner}/{repo}/stats/code_frequency

GET /repos/{owner}/{repo}/stats/commit_activity

GET /repos/{owner}/{repo}/stats/contributors

GET /repos/{owner}/{repo}/stats/participation

GET /repos/{owner}/{repo}/stats/punch_card

GET /repos/{owner}/{repo}/traffic/clones

GET /repos/{owner}/{repo}/traffic/views

GET /repos/{owner}/{repo}/traffic/popular/referrers

GET /repos/{owner}/{repo}/traffic/popular/paths
```

These endpoints can be consumed by CI automation to generate a periodically updated repository dashboard.

---

# Contributors

<a href="https://github.com/nguyenphanno/Slayer-L7/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=nguyenphanno/Slayer-L7" alt="Contributors">
</a>

---

# Latest Release

<p align="center">

<a href="https://github.com/nguyenphanno/Slayer-L7/releases">
  <img src="https://img.shields.io/github/v/release/nguyenphanno/Slayer-L7?style=for-the-badge&logo=github&label=Latest%20Release">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7/releases">
  <img src="https://img.shields.io/github/downloads/nguyenphanno/Slayer-L7/total?style=for-the-badge&label=Downloads">
</a>

</p>

---

# Project Structure

The current project intentionally keeps the implementation compact.

```text
Slayer-L7/
├── .github/
│   └── workflows/
│
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── main.go
├── proxy.txt
├── socks5.txt
└── useragent.txt
```

## File Reference

| File            | Purpose                          |
| :-------------- | :------------------------------- |
| `main.go`       | Main application and test engine |
| `go.mod`        | Go module definition             |
| `go.sum`        | Dependency checksums             |
| `proxy.txt`     | Proxy configuration dataset      |
| `socks5.txt`    | SOCKS5 proxy dataset             |
| `useragent.txt` | User-Agent dataset               |
| `README.md`     | Project documentation            |
| `LICENSE`       | Project license                  |

---

# Developer Reference

## GitHub Repository API

| Endpoint                                          | Purpose                    |
| :------------------------------------------------ | :------------------------- |
| `/repos/{owner}/{repo}`                           | Repository metadata        |
| `/repos/{owner}/{repo}/contributors`              | Contributor information    |
| `/repos/{owner}/{repo}/stats/code_frequency`      | Code additions / deletions |
| `/repos/{owner}/{repo}/stats/commit_activity`     | Weekly commit activity     |
| `/repos/{owner}/{repo}/stats/contributors`        | Contributor statistics     |
| `/repos/{owner}/{repo}/stats/participation`       | Repository participation   |
| `/repos/{owner}/{repo}/stats/punch_card`          | Commit time distribution   |
| `/repos/{owner}/{repo}/traffic/clones`            | Clone statistics           |
| `/repos/{owner}/{repo}/traffic/views`             | Page-view statistics       |
| `/repos/{owner}/{repo}/traffic/popular/referrers` | Referral sources           |
| `/repos/{owner}/{repo}/traffic/popular/paths`     | Popular repository paths   |

---

# Documentation Map

| Document                            | Description               |
| :---------------------------------- | :------------------------ |
| [Installation](#requirements)       | Requirements and setup    |
| [Quick Start](#quick-start)         | Build and first execution |
| [Configuration](#cli-configuration) | CLI configuration         |
| [Architecture](#architecture)       | Internal execution model  |
| [Capabilities](#capability-matrix)  | Protocol coverage         |
| [Test Methods](#test-methods)       | Scenario reference        |
| [Proxy Support](#proxy-support)     | Proxy configuration       |
| [Observability](#observability)     | Metrics and monitoring    |
| [Performance](#performance)         | Runtime considerations    |
| [Troubleshooting](#troubleshooting) | Common problems           |
| [Security](#security--legal)        | Authorized use            |
| [Contributing](#contributing)       | Development guidelines    |

---

# Troubleshooting

| Symptom                      | Likely Cause                | Action                                      |
| :--------------------------- | :-------------------------- | :------------------------------------------ |
| `Err: 377920`                | Invalid / unavailable proxy | Validate proxy configuration                |
| Very low throughput          | Local resource exhaustion   | Reduce worker count                         |
| Connections stop increasing  | Rate limiting               | Inspect WAF / proxy logs                    |
| Increasing latency           | Application saturation      | Inspect CPU, database and upstream services |
| Console artifacts            | ANSI compatibility          | Use a modern terminal                       |
| Frequent connection failures | Socket exhaustion           | Reduce concurrency and inspect OS limits    |

---

## Verbose Diagnostics

```bash
./slayer \
  -t https://your-staging-environment.example \
  -m httpget \
  -w 100 \
  -d 30 \
  -v
```

---

# Roadmap

## Core Engine

* [x] Multi-protocol execution engine
* [x] Goroutine-based workers
* [x] CLI configuration
* [x] Proxy abstraction
* [x] Runtime diagnostics

## Observability

* [ ] Structured JSON output
* [ ] Improved latency statistics
* [ ] Latency percentiles
* [ ] Prometheus metrics
* [ ] Interactive terminal dashboard
* [ ] Result export
* [ ] Test-session reporting

## Developer Experience

* [ ] Config-file support
* [ ] Scenario presets
* [ ] Better configuration validation
* [ ] Improved error classification
* [ ] Automated integration tests
* [ ] Improved protocol diagnostics

## CI / CD

* [ ] Automated releases
* [ ] Multi-platform binaries
* [ ] CodeQL
* [ ] Dependency scanning
* [ ] SBOM generation
* [ ] Reproducible builds

## Distributed Testing

* [ ] Controlled distributed testing
* [ ] Coordinator / worker architecture
* [ ] Centralized metrics
* [ ] Session orchestration
* [ ] Distributed result aggregation

---

# Security & Legal

> [!WARNING]
> **Authorized use only.**

Slayer L7 is designed for system administrators, infrastructure engineers and security researchers performing controlled testing on systems they own or have explicit authorization to assess.

High-volume traffic, connection exhaustion, parser stress and protocol edge cases can affect service availability.

Before running a test:

1. Define the authorized target scope.
2. Establish a testing window.
3. Record baseline metrics.
4. Start with conservative workload levels.
5. Monitor the target continuously.
6. Stop the test if unexpected instability occurs.
7. Preserve relevant logs and measurements.

The project authors are not responsible for damage, disruption or misuse resulting from unauthorized deployment.

---

# Responsible Testing

A safe assessment should prioritize measurement over maximum traffic volume.

```text
             ┌──────────────────────┐
             │     Authorization    │
             └──────────┬───────────┘
                        │
                        ▼
             ┌──────────────────────┐
             │       Baseline       │
             └──────────┬───────────┘
                        │
                        ▼
             ┌──────────────────────┐
             │    Controlled Load   │
             └──────────┬───────────┘
                        │
                        ▼
             ┌──────────────────────┐
             │      Monitoring      │
             └──────────┬───────────┘
                        │
                        ▼
             ┌──────────────────────┐
             │      Analysis        │
             └──────────┬───────────┘
                        │
                        ▼
             ┌──────────────────────┐
             │   Harden / Retest    │
             └──────────────────────┘
```

---

# Contributing

Contributions that improve reliability, documentation, observability and authorized testing capabilities are welcome.

Before submitting a pull request:

1. Keep changes focused.
2. Follow existing project conventions.
3. Document new configuration options.
4. Test changes in an isolated environment.
5. Update the README when behavior changes.
6. Add tests where practical.
7. Avoid committing credentials or private datasets.
8. Do not commit API keys, tokens or secrets.
9. Keep protocol implementations isolated where possible.

---

# Security Reporting

If you discover a security issue affecting the project itself, please avoid publishing sensitive exploit details in a public issue.

Use the repository's configured private security reporting mechanism when available.

---

# License

Distributed under the **MIT License**.

See [`LICENSE`](LICENSE) for details.

---

<div align="center">

<br>

# `</>` SLAYER L7

### Measure · Analyze · Harden

<p>
  Multi-protocol resilience testing for authorized environments.
</p>

<br>

<a href="https://github.com/nguyenphanno/Slayer-L7">
  <img src="https://img.shields.io/github/stars/nguyenphanno/Slayer-L7?style=flat-square&logo=github&label=Star%20Repository">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7/issues">
  <img src="https://img.shields.io/github/issues/nguyenphanno/Slayer-L7?style=flat-square&label=Issues">
</a>

<a href="https://github.com/nguyenphanno/Slayer-L7/blob/main/LICENSE">
  <img src="https://img.shields.io/github/license/nguyenphanno/Slayer-L7?style=flat-square&label=MIT">
</a>

<br><br>

<sub>Built with Go · Designed for controlled infrastructure testing</sub>

</div>
