<div align="center">

# </> SLAYER L7

### Multi-Protocol Layer 7 Stress Testing & Network Resilience Toolkit

<p>
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.21+">
  </a>
  <img src="https://img.shields.io/badge/Methods-75%2B-6E56CF?style=for-the-badge" alt="75+ Methods">
  <img src="https://img.shields.io/badge/Architecture-Goroutines-2EA44F?style=for-the-badge&logo=go&logoColor=white" alt="Goroutines">
  <img src="https://img.shields.io/badge/Proxy-SOCKS4%20%7C%20SOCKS5%20%7C%20HTTP-24292F?style=for-the-badge" alt="Proxy">
  <img src="https://img.shields.io/badge/License-MIT-0969DA?style=for-the-badge&logo=opensourceinitiative&logoColor=white" alt="MIT License">
</p>

<p>
  <strong>Controlled infrastructure testing · Resilience engineering · Security research</strong>
</p>

<p>
  <a href="#overview">Overview</a> ·
  <a href="#features">Features</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#installation">Installation</a> ·
  <a href="#usage">Usage</a> ·
  <a href="#test-methods">Test Methods</a> ·
  <a href="#project-structure">Structure</a> ·
  <a href="#troubleshooting">Troubleshooting</a> ·
  <a href="#security--legal">Security</a>
</p>

</div>

---

## Overview

**Slayer L7** is a Go-based multi-protocol stress-testing and network-resilience toolkit built for **authorized infrastructure assessments**.

The project provides a single lightweight executable capable of generating controlled workloads across multiple application and network protocols, allowing administrators and security researchers to evaluate system behavior under increased traffic, connection pressure and protocol-specific workloads.

### What Slayer L7 Provides

* High-concurrency Go execution using Goroutines
* HTTP/1.1 and HTTP/2 workload generation
* Configurable request and connection behavior
* SOCKS4 / SOCKS5 / HTTP proxy support
* User-Agent rotation
* Protocol-specific testing scenarios
* Minecraft / Netty protocol testing
* TCP, UDP and DNS workload testing
* Configurable execution duration
* Runtime diagnostics and error reporting

---

## Features

<table>
<tr>
<td width="50%" valign="top">

### HTTP & Application

* HTTP GET / POST / PUT / DELETE
* HEAD / OPTIONS requests
* JSON workloads
* Multipart requests
* Form-data testing
* Header-size testing
* Cookie handling
* Cache behavior testing
* WebSocket testing
* GraphQL workloads

</td>
<td width="50%" valign="top">

### Network & Protocol

* HTTP/1.1
* HTTP/2
* TCP
* UDP
* DNS
* Minecraft / Netty
* BungeeCord-compatible flows
* Persistent connections
* Proxy-based testing

</td>
</tr>

<tr>
<td valign="top">

### Performance

* Goroutine-based concurrency
* Configurable worker count
* Request pacing
* Connection management
* User-Agent rotation
* Low-overhead execution
* Long-running test sessions

</td>
<td valign="top">

### Security Research

* WAF validation
* Rate-limit testing
* Reverse-proxy analysis
* Parser robustness testing
* Connection-limit testing
* HTTP/2 behavior analysis
* Input validation testing
* Infrastructure resilience testing

</td>
</tr>
</table>

---

## Architecture

Slayer L7 uses a compact Go architecture centered around a single executable.

```text
                         ┌───────────────────────┐
                         │        CLI Input      │
                         │ target / method /     │
                         │ workers / duration    │
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │     Test Controller   │
                         │   scenario selection  │
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
                         │    Proxy / Network    │
                         │ HTTP / SOCKS4 / SOCKS5│
                         └───────────┬───────────┘
                                     │
                                     ▼
                         ┌───────────────────────┐
                         │    Authorized Target  │
                         └───────────────────────┘
```

### Execution Model

```text
Configuration
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
       Request / Packet
             │
             ▼
        Test Endpoint
             │
             ▼
      Runtime Statistics
```

The workload is controlled through the CLI, allowing concurrency and duration to be adjusted according to the capacity of the test environment.

---

## Installation

### Requirements

| Component | Requirement     |
| --------- | --------------- |
| Go        | `1.21+`         |
| Linux     | Supported       |
| macOS     | Supported       |
| Windows   | WSL recommended |

### Clone

```bash
git clone https://github.com/nguyenphanno/Slayer-L7.git
cd Slayer-L7
```

### Dependencies

```bash
go mod tidy
```

### Build

```bash
go build -ldflags="-s -w" -o slayer main.go
```

Linux / macOS:

```bash
chmod +x slayer
```

### Verify

```bash
./slayer --help
```

---

## Usage

### Command Syntax

```text
./slayer -t <TARGET> -m <METHOD> -w <WORKERS> -d <DURATION> [OPTIONS]
```

### CLI Configuration

| Flag | Type     |   Default | Description                      |
| ---- | -------- | --------: | -------------------------------- |
| `-t` | `string` |         — | Target URL or endpoint           |
| `-m` | `string` | `httpget` | Test method                      |
| `-w` | `int`    |    `2048` | Concurrent workers               |
| `-d` | `int`    |      `30` | Test duration in seconds         |
| `-p` | `string` |         — | Proxy list path                  |
| `-r` | `int`    |       `0` | Delay per worker in milliseconds |
| `-v` | `bool`   |   `false` | Enable verbose logging           |

### Example

```bash
./slayer \
  -t https://your-staging-environment.example \
  -m httpget \
  -w 100 \
  -d 30
```

Start with a low concurrency level and gradually increase it while observing the target environment.

---

## Test Methods

Slayer L7 contains a collection of protocol and application-level test scenarios.

### HTTP Methods

| Method           | Description                  |
| ---------------- | ---------------------------- |
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

### Slow / Connection Tests

| Method           | Description                  |
| ---------------- | ---------------------------- |
| `rudy`           | Slow request-body behavior   |
| `slowloris`      | Partial HTTP header handling |
| `chunkpost`      | Chunked transfer behavior    |
| `http_slow_read` | Slow response consumption    |
| `http_dead_conn` | Idle connection handling     |
| `http_bad_start` | Incomplete request handling  |

### HTTP/2 & Proxy Tests

| Method              | Description                        |
| ------------------- | ---------------------------------- |
| `rapidreset`        | HTTP/2 reset behavior              |
| `h2continuation`    | HTTP/2 continuation-frame handling |
| `http_h2_flood`     | HTTP/2 stream-capacity testing     |
| `smuggle_clte`      | CL/TE parser-alignment testing     |
| `smuggle_tete`      | TE parser-alignment testing        |
| `http_conn_smuggle` | Persistent connection validation   |

### Web Application Tests

| Method         | Description                  |
| -------------- | ---------------------------- |
| `mixpost`      | Content-Type handling        |
| `cfbypass`     | CDN / WAF behavior research  |
| `cache_poison` | Cache-key validation         |
| `wsflood`      | WebSocket connection testing |
| `headerflood`  | Large HTTP header testing    |
| `cookiebomb`   | Cookie-size handling         |
| `range`        | Range-request processing     |
| `malformed`    | URI parser robustness        |

### Input Validation

| Method           | Description                        |
| ---------------- | ---------------------------------- |
| `xss_probe`      | XSS input validation               |
| `sqli_probe`     | SQL injection detection validation |
| `path_traversal` | Path normalization testing         |
| `redos`          | Regex processing resilience        |
| `graphql_batch`  | GraphQL workload testing           |
| `zstd_bomb`      | Decompression-limit testing        |
| `pingback`       | XML-RPC endpoint testing           |

### Minecraft / Netty

| Method         | Description                 |
| -------------- | --------------------------- |
| `mc_ping`      | Server-list ping testing    |
| `mc_bot`       | Authentication-flow testing |
| `mc_bigpacket` | Frame-decoder robustness    |
| `mc_legacy`    | Legacy protocol handling    |
| `mc_hold`      | Connection-limit testing    |
| `mc_bungee`    | Proxy-forwarding validation |

### Network

| Method        | Description                     |
| ------------- | ------------------------------- |
| `tcp_connect` | TCP connection-capacity testing |
| `tcp_slow`    | Slow TCP transfer testing       |
| `udp_flood`   | UDP throughput testing          |
| `dns_query`   | DNS resolver workload testing   |
| `syn_flood`   | SYN handling validation         |
| `ack_flood`   | Stateful firewall validation    |

---

## Proxy Support

Slayer L7 supports proxy-based testing using controlled proxy infrastructure.

Supported proxy formats include:

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
* The traffic source is part of the authorized test scope.

---

## Project Structure

The repository intentionally uses a compact structure rather than splitting the implementation into multiple internal packages.

```text
Slayer-L7/
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── main.go
├── proxy.txt
├── socks5.txt
└── useragent.txt
```

### File Reference

| File            | Purpose                          |
| --------------- | -------------------------------- |
| `main.go`       | Main application and test engine |
| `go.mod`        | Go module definition             |
| `go.sum`        | Dependency checksums             |
| `proxy.txt`     | Proxy dataset                    |
| `socks5.txt`    | SOCKS5 proxy dataset             |
| `useragent.txt` | User-Agent dataset               |
| `README.md`     | Project documentation            |
| `LICENSE`       | License information              |

---

## Testing Workflow

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
└─────────────────────┘
```

### Metrics Worth Monitoring

* CPU utilization
* Memory utilization
* Network bandwidth
* Active connections
* Request latency
* HTTP status codes
* Application error rate
* Database utilization
* Reverse-proxy metrics
* WAF events
* System file descriptors

---

## Performance

Slayer L7 is built around Go's lightweight concurrency model.

### Goroutine-Based Execution

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

> Higher worker counts do not necessarily produce better test results. Controlled load progression generally produces more useful capacity measurements.

---

## Troubleshooting

| Symptom                      | Likely Cause                | Action                                      |
| ---------------------------- | --------------------------- | ------------------------------------------- |
| `Err: 377920`                | Invalid / unavailable proxy | Validate the proxy list                     |
| Very low RPS                 | Local resource exhaustion   | Reduce `-w` and inspect system limits       |
| Connections stop increasing  | Rate limiting               | Inspect WAF / reverse-proxy logs            |
| Increasing latency           | Application saturation      | Inspect CPU, database and upstream services |
| Console artifacts            | ANSI compatibility          | Use a modern terminal                       |
| Frequent connection failures | Socket exhaustion           | Reduce concurrency and inspect OS limits    |

### Verbose Diagnostics

```bash
./slayer \
  -t https://your-staging-environment.example \
  -m httpget \
  -w 100 \
  -d 30 \
  -v
```

---

## Roadmap

* [ ] Structured JSON output
* [ ] Config-file support
* [ ] Improved latency statistics
* [ ] Scenario presets
* [ ] Result export
* [ ] Prometheus metrics
* [ ] Interactive terminal dashboard
* [ ] Better proxy health monitoring
* [ ] Improved HTTP/2 diagnostics
* [ ] Test-session reporting
* [ ] Controlled distributed testing

---

## Security & Legal

> [!WARNING]
> **Authorized use only.**
>
> Slayer L7 is designed for system administrators, infrastructure engineers and security researchers performing controlled testing on systems they own or have explicit authorization to assess.
>
> High-volume traffic and resource-exhaustion scenarios can affect service availability. Define the target scope, obtain authorization and establish a controlled testing window before execution.
>
> The project authors are not responsible for damage, disruption or misuse resulting from unauthorized deployment.

---

## Contributing

Contributions that improve the project's reliability, documentation, observability and authorized testing capabilities are welcome.

Before submitting a pull request:

1. Keep changes focused.
2. Document new configuration options.
3. Test changes in an isolated environment.
4. Update the README when behavior changes.
5. Avoid committing credentials, private proxy lists or other sensitive data.

---

## License

Distributed under the **MIT License**.

See [`LICENSE`](LICENSE) for details.

---

<div align="center">

## SLAYER L7

**Measure · Analyze · Harden**

Multi-protocol resilience testing for authorized environments.

</div>
