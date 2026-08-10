<div align="center">

# </> SLAYER L7

**Advanced Multi-Protocol Layer 7 Stress Testing & Network Resilience Appliance**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Methods](https://img.shields.io/badge/Methods-75+-FF4136?style=for-the-badge&logo=codeforces&logoColor=white)](#-attack-vectors--methods)
[![Architecture](https://img.shields.io/badge/Architecture-Goroutines-2EA44F?style=for-the-badge&logo=go&logoColor=white)](#-architecture--overview)
[![Proxy](https://img.shields.io/badge/Proxy-SOCKS5%20%7C%20HTTP-1A1A1A?style=for-the-badge&logo=proxyman&logoColor=white)](#-cli-flags--configuration)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge&logo=readthedocs&logoColor=white)](LICENSE)

[Overview](#-architecture--overview) • [Installation](#-installation) • [Usage](#-cli-flags--configuration) • [Attack Vectors](#-attack-vectors--methods) • [Troubleshooting](#-troubleshooting) • [Disclaimer](#-security--legal-disclaimer)

</div>

---

## 📖 Architecture & Overview

**Slayer L7** is a modern, ultra-high-throughput network stress-testing engine engineered in Go. Designed to evaluate web server resilience, load balancers, and WAF rulesets, it leverages asynchronous Goroutines, custom TLS/HTTP handshakes, and low-allocation memory pools to deliver massive Request-Per-Second (RPS) workloads.

> **Key Capabilities:**
> * **Zero-Allocation HTTP Forging**: Dynamic header generation, user-agent rotation, and payload polymorphic randomization.
> * **Protocol Exploitation Vectors**: Implements HTTP/2 Rapid Reset (CVE-2023-44487), Continuation Floods, and HTTP Request Smuggling.
> * **Built-in Proxy Engine**: Native SOCKS4/SOCKS5/HTTP proxy parsing with dynamic round-robin connection pooling.
> * **Game Protocol Testing**: Specialized engines for Minecraft BungeeCord/Netty protocol validation and handshake stress testing.

---

## 🚀 Installation

### Prerequisites

* [Go (Golang)](https://go.dev/dl/) `v1.21` or higher.
* POSIX-compliant environment (Linux / macOS) or Windows Subsystem for Linux (WSL).

### Building from Source

```bash
# Clone the repository
git clone [https://github.com/yinlewoaisuru/Slayer-L7.git](https://github.com/yinlewoaisuru/Slayer-L7.git)
cd Slayer-L7

# Initialize & fetch dependencies
go mod init slayer
go mod tidy

# Build native binary with optimizations
go build -ldflags="-s -w" -o slayer main.go

# Grant execution rights (Linux / macOS)
chmod +x slayer

```

---

## ⚙️ CLI Flags & Configuration

```bash
./slayer -t <TARGET_URL> -m <METHOD> -w <WORKERS> -d <DURATION> [-p <PROXY_FILE>]

```

| Flag | Type | Description | Required | Default |
| --- | --- | --- | --- | --- |
| `-t` | `string` | Target URL/Endpoint (e.g., `https://example.com` or `http://mc.net:25565`) | ✅ | — |
| `-m` | `string` | Attack method / test vector | ❌ | `httpget` |
| `-w` | `int` | Concurrent worker threads (Goroutines) | ❌ | `2048` |
| `-d` | `int` | Execution duration in seconds | ❌ | `30` |
| `-p` | `string` | Path to proxy list file | ❌ | — |
| `-r` | `int` | Per-worker request delay in milliseconds | ❌ | `0` |
| `-v` | `bool` | Verbose error logging | ❌ | `false` |

---

## 🛡️ Attack Vectors & Methods

Slayer L7 includes over **75+ built-in test vectors** categorized across specific network layers and application behaviors:

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `httpget` | Cache-busted HTTP GET requests | Stress static application pages |
| `httppost` | High-density JSON/Base64 payload POST | Exhaust backend application parsers |
| `apiflood` | Deeply nested JSON tree queries | Strain REST API microservices |
| `httpoptions` | CORS Preflight OPTIONS queries | Bypass serverless caching layers |
| `httpdelete` | Resource deletion requests | Force intensive database authorization lookups |
| `httpput` | Multipart data upload stream | Stress storage write queues |
| `httphead` | Cache-bypass HEAD requests | Exhaust web server socket handlers |
| `http_json` | High-depth structured JSON parsing | Exhaust Node.js / Python server loops |
| `http_multipart` | Form-data file upload emulation | Fill temporary disk storage buffers |
| `http_form_bomb` | Massive key-value pair injection | Saturate HTTP request parameter parsers |
| `http_payload` | Raw random byte payload delivery | Test web server buffer bounds |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `rudy` | Slow POST body transmission (1 byte/sec) | Exhaust Apache/IIS thread pools |
| `slowloris` | Partial HTTP header transmission | Occupy connection slots indefinitely |
| `chunkpost` | Drip-fed chunked transfer encoding | Bypass proxy timeout controls |
| `http_slow_read` | Micro-window TCP response consumption | Starve worker threads on web servers |
| `http_dead_conn` | Long-lived idle TCP socket holding | Fill active socket descriptor tables |
| `http_bad_start` | Incomplete HTTP header initializers | Stall connection state machines |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `rapidreset` | HTTP/2 Rapid Reset (CVE-2023-44487) | Overwhelm HTTP/2 framing layer |
| `h2continuation` | HTTP/2 Continuation Flood (CVE-2024-2730) | Force OOM condition on load balancers |
| `http_h2_flood` | Concurrent HTTP/2 stream generation | Exhaust HTTP/2 multiplexing limits |
| `smuggle_clte` | CL.TE HTTP Request Smuggling | Test frontend/backend proxy alignment |
| `smuggle_tete` | TE.TE HTTP Request Smuggling | Bypass WAF request inspection |
| `http_conn_smuggle` | Keep-Alive connection smuggling | Test rate-limiting proxy enforcement |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `mixpost` | Dynamic Content-Type polymorphism | Evade static signature-based WAFs |
| `cfbypass` | Browser-accurate TLS fingerprinting | Evaluate CDN threat scoring |
| `cache_poison` | Unkeyed header query injection | Test reverse proxy cache pollution |
| `wsflood` | Persistent WebSocket connection holding | Exhaust full-duplex socket resources |
| `headerflood` | Oversized HTTP headers (>100KB) | Trigger request header buffer overflows |
| `cookiebomb` | Massive cookie string injection | Saturate session memory limits |
| `range` | Overlapping Byte-Range request headers | Exhaust CPU during content reassembly |
| `malformed` | Extremely long URI strings (>8KB) | Force crash in logging parsers |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `xss_probe` | XSS vector query injection | Overwhelm security event logs |
| `sqli_probe` | SQL syntax query injection | Stress WAF regex inspection engines |
| `path_traversal` | Directory traversal string testing | Test path normalization rules |
| `redos` | ReDoS triggers via complex strings | Cause high CPU utilization in regex evaluation |
| `graphql_batch` | Multi-nested GraphQL queries | Force query engine execution loops |
| `zstd_bomb` | High-compression ratio payloads | Cause memory spikes during body decompression |
| `pingback` | XML-RPC Pingback reflection requests | Test legacy CMS endpoint vulnerabilities |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `mc_ping` | Server List Ping spam | Evaluate Netty thread pool performance |
| `mc_bot` | Simulated player login sequences | Stress authentication & proxy pipeline |
| `mc_bigpacket` | Malformed VarInt frame sizing | Test protocol decoder stability |
| `mc_legacy` | Legacy SLP handshake packets | Stress backward compatibility handlers |
| `mc_hold` | Connection holding after ping | Occupy player slot limits |
| `mc_bungee` | BungeeCord forwarding spoofing | Validate proxy IP forwarding rules |

| Vector | Target Mechanism | Primary Objective |
| --- | --- | --- |
| `tcp_connect` | Full TCP connection handshake holding | Exhaust system connection limits |
| `tcp_slow` | Drip-fed TCP data transmission | Bypass stateless SYN flood protections |
| `udp_flood` | High-volume UDP datagram delivery | Test network interface card throughput |
| `dns_query` | High-frequency DNS query requests | Stress recursive DNS resolver infrastructure |
| `syn_flood` | Raw TCP SYN packet generation | Evaluate firewall conntrack table capacity |
| `ack_flood` | High-rate TCP ACK packet stream | Overwhelm stateful firewall tracking |

---

## 🔧 Troubleshooting

| Error / Symptom | Root Cause | Solution |
| --- | --- | --- |
| `Err: 377920` | Dead or unresponsive proxies | Update `proxy.txt` or remove `-p` to run in direct mode. |
| `RPS: 1` | Local socket exhaustion (`TIME_WAIT`) | Lower worker count `-w` to `500-1000` or adjust OS socket reuse flags. |
| Target blocks after ~100 req | Rate-limiting / IP firewall active | Utilize residential SOCKS5 proxies (`-p proxy.txt`). |
| Visual artifacts in console | Terminal lacks ANSI color code support | Use Windows Terminal, Alacritty, or standard Linux/macOS bash terminals. |

---

## ⚠️ Security & Legal Disclaimer

> [!CAUTION]
> This tool is strictly intended for **authorized system administration, infrastructure stress testing, and academic research**. Executing uncoordinated stress tests against third-party systems without prior written consent is illegal and violates international cybercrime laws. The developers assume no responsibility for any misuse or damage caused by this software.

---

Developed for System Administrators & Security Researchers.
