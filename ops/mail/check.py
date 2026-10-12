#!/usr/bin/env python3
"""Read-only checks for self-hosted direct SMTP delivery prerequisites."""
import concurrent.futures
import json
from pathlib import Path
import shlex
import socket
import subprocess

STATE = Path('/opt/sameframe/mail')
DOMAIN = 'sameframe.top'
HOST = 'mail.sameframe.top'
IP = '38.76.190.179'


def dns(name, kind):
    result = subprocess.run(['dig', '@1.1.1.1', '+short', '+time=3', '+tries=1', name, kind],
                            capture_output=True, text=True, timeout=6, check=True)
    lines = result.stdout.strip().splitlines()
    return [''.join(shlex.split(line)) for line in lines] if kind == 'TXT' else lines


def probe(domain):
    hosts = sorted(dns(domain, 'MX'), key=lambda row: int(row.split()[0]))
    if not hosts:
        return {'domain': domain, 'ok': False, 'error': 'No MX'}
    host = hosts[0].split()[1].rstrip('.')
    address = socket.getaddrinfo(host, 25, socket.AF_INET, socket.SOCK_STREAM)[0][4]
    try:
        with socket.create_connection(address, timeout=8) as connection:
            connection.settimeout(8)
            banner = connection.recv(1024).decode(errors='replace').strip()
            connection.sendall(b'QUIT\r\n')
        return {'domain': domain, 'host': host, 'address': address[0], 'ok': banner.startswith('220'), 'banner': banner[:160]}
    except OSError as error:
        return {'domain': domain, 'host': host, 'address': address[0], 'ok': False, 'error': str(error)}


def check():
    checks = []
    for record in json.loads((STATE / 'dns-records.json').read_text()):
        name = DOMAIN if record['name'] == '@' else record['name'] + '.' + DOMAIN
        actual = dns(name, record['type'])
        expected = record['content']
        if record['type'] == 'MX':
            expected = str(record['priority']) + ' ' + expected + '.'
        checks.append({'name': name, 'type': record['type'], 'ok': expected in actual, 'actual': actual})
    reverse = dns('179.190.76.38.in-addr.arpa', 'PTR')
    checks.append({'name': IP, 'type': 'PTR', 'ok': HOST + '.' in reverse, 'actual': reverse})
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        reachability = list(pool.map(probe, ['126.com', 'qq.com', 'gmail.com']))
    return {'ready': all(item['ok'] for item in checks + reachability), 'dns': checks, 'outbound_25': reachability}


if __name__ == '__main__':
    result = check()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    raise SystemExit(0 if result['ready'] else 2)
