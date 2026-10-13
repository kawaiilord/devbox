#!/usr/bin/env python3
"""Provision authenticated TURN, then publish voice UI with a combined rollback."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time

ROOT = Path('/opt/sameframe')
STATE = ROOT / 'releases/room-voice-infrastructure.json'
IMAGE = 'coturn/coturn:4.18.0-r0-alpine@sha256:c29135c08565810d37053f384584392b7c24b69570fc81a721b878d6fe63e04e'


def run(*args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def prepare(args):
    if STATE.exists() or (ROOT/'compose.override.yml').exists() or (ROOT/'turn').exists():
        raise RuntimeError('Existing TURN/override state must be inspected before provision')
    ipaddress.IPv4Address(args.public_ip)
    if not args.domain or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789.-' for c in args.domain):
        raise ValueError('Invalid TURN certificate hostname')
    certificate = ROOT/'letsencrypt/live'/args.domain/'fullchain.pem'
    if not certificate.is_file(): raise RuntimeError('Existing TLS certificate is required')
    route = json.loads(run('ip','-j','route','get','1.1.1.1',capture_output=True,text=True).stdout)[0]
    private = str(ipaddress.IPv4Address(route['prefsrc']))
    renewal = ROOT/'renew-tls.sh'
    original = renewal.read_text()
    marker = '  docker compose restart mediamtx\n'
    if original.count(marker) != 1: raise RuntimeError('Unexpected certificate renewal script')
    backup = ROOT/'backups'/('room-voice-infra-'+time.strftime('%Y%m%d-%H%M%S',time.gmtime()))
    backup.mkdir(mode=0o700)
    shutil.copy2(renewal,backup/'renew-tls.sh')
    record = {'status':'backed_up','backup_directory':str(backup),'image':IMAGE,
              'override_previously_absent':True,'turn_directory_previously_absent':True,
              'renewal_sha256':hashlib.sha256(original.encode()).hexdigest(),
              'public_ip':args.public_ip,'private_ip':private,'domain':args.domain,
              'ports':['3478/udp','3478/tcp','5349/tcp','49160-49359/udp']}
    save(STATE,record)
    try:
        directory = ROOT/'turn';directory.mkdir(mode=0o700)
        secret = secrets.token_hex(32)
        urls = ','.join([f'turn:{args.public_ip}:3478?transport=udp',
                         f'turn:{args.public_ip}:3478?transport=tcp',
                         f'turns:{args.domain}:5349?transport=tcp'])
        (directory/'app.env').write_text('SAMEFRAME_TURN_URLS='+urls+'\nSAMEFRAME_TURN_SECRET='+secret+'\n')
        config = f'''listening-port=3478
tls-listening-port=5349
listening-ip=0.0.0.0
relay-ip={private}
external-ip={args.public_ip}/{private}
min-port=49160
max-port=49359
realm={args.domain}
server-name={args.domain}
fingerprint
use-auth-secret
static-auth-secret={secret}
cert=/etc/letsencrypt/live/{args.domain}/fullchain.pem
pkey=/etc/letsencrypt/live/{args.domain}/privkey.pem
no-tlsv1
no-tlsv1_1
no-dtls
no-cli
no-tcp-relay
no-multicast-peers
user-quota=32
total-quota=512
max-bps=128000
bps-capacity=16000000
stale-nonce=600
pidfile=/tmp/turnserver.pid
userdb=/tmp/turn.sqlite
log-file=stdout
simple-log
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
denied-peer-ip=192.0.0.0-192.0.0.255
denied-peer-ip=192.0.2.0-192.0.2.255
denied-peer-ip=198.18.0.0-198.19.255.255
denied-peer-ip=198.51.100.0-198.51.100.255
denied-peer-ip=203.0.113.0-203.0.113.255
denied-peer-ip=224.0.0.0-255.255.255.255
denied-peer-ip=::
denied-peer-ip=::1
denied-peer-ip=fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff
'''
        (directory/'turnserver.conf').write_text(config)
        (ROOT/'compose.override.yml').write_text(f'''services:
  app:
    env_file: [./turn/app.env]
  turn:
    image: {IMAGE}
    network_mode: host
    user: "0:0"
    restart: unless-stopped
    read_only: true
    cap_drop: [ALL]
    cap_add: [NET_BIND_SERVICE]
    security_opt: [no-new-privileges:true]
    command: ["-c", "/etc/coturn/turnserver.conf"]
    volumes:
      - ./turn/turnserver.conf:/etc/coturn/turnserver.conf:ro
      - ./letsencrypt:/etc/letsencrypt:ro
    tmpfs: ["/tmp:rw,noexec,nosuid,size=16m", "/var/lib/coturn:rw,noexec,nosuid,size=16m"]
    healthcheck:
      test: ["CMD", "turnutils_stunclient", "-p", "3478", "127.0.0.1"]
      interval: 30s
      timeout: 5s
      retries: 3
''')
        renewal.write_text(original.replace(marker,marker+'  docker compose restart turn\n'))
        run('docker','compose','config','--quiet')
        run('docker','compose','up','-d','--no-deps','turn')
        time.sleep(2)
        run('docker','compose','exec','-T','turn','turnutils_stunclient','-p','3478','127.0.0.1')
        record.update(status='prepared',turn_urls=urls,prepared_at_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()))
        save(STATE,record)
    except Exception:
        restore_infrastructure(record)
        raise
    print(json.dumps(record))


def restore_infrastructure(record):
    backup = Path(record['backup_directory'])/'renew-tls.sh'
    if hashlib.sha256(backup.read_bytes()).hexdigest()!=record['renewal_sha256']:
        raise RuntimeError('Renewal backup checksum mismatch')
    if (ROOT/'compose.override.yml').exists():
        run('docker','compose','stop','turn')
        run('docker','compose','rm','-f','-s','-v','turn')
        (ROOT/'compose.override.yml').unlink()
    shutil.copy2(backup,ROOT/'renew-tls.sh')
    if (ROOT/'turn').exists(): shutil.rmtree(ROOT/'turn')
    record['status']='restored'
    save(STATE,record)


def publish(args):
    record=json.loads(STATE.read_text())
    if record['status']!='prepared': raise RuntimeError('TURN must be prepared and externally verified first')
    if not args.revision or any(c not in '0123456789abcdef' for c in args.revision): raise ValueError('Invalid built revision')
    release=ROOT/'releases'/('room-voice-'+args.revision)
    if release.exists(): raise RuntimeError('Release already exists')
    try:
        run('python3',str(ROOT/'membership.py'),'--name','room-voice','--revision',args.revision,
            '--database-note','no schema change; authenticated TURN and room audio playback',
            '--web-archive',args.web_archive,'--server-binary',args.server_binary)
        shutil.copyfile(__file__,release/'voice-release.py')
        save(release/'turn.json',record)
        manifest=release/'deployment.json';data=json.loads(manifest.read_text())
        data['application_only_rollback_command']=data['rollback_command']
        data['rollback_command']='python3 '+str(release/'voice-release.py')+' --rollback'
        data['turn_record']=str(release/'turn.json');save(manifest,data)
        record.update(status='deployed',revision=args.revision,rollback_command=data['rollback_command'])
        save(STATE,record);save(release/'turn.json',record)
    except Exception:
        restore_infrastructure(record)
        if (release/'deployment.json').exists(): run('python3',str(release/'release.py'),'--rollback')
        raise
    print(json.dumps(record))


def rollback():
    release=Path(__file__).resolve().parent
    record=json.loads((release/'turn.json').read_text())
    run('python3',str(release/'release.py'),'--check-rollback')
    restore_infrastructure(record)
    run('python3',str(release/'release.py'),'--rollback')
    save(release/'turn.json',record)
    print('Previous app, web and service configuration restored; database activity retained.')


if __name__=='__main__':
    os.umask(0o077)
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepare',action='store_true');parser.add_argument('--public-ip');parser.add_argument('--domain')
    parser.add_argument('--revision');parser.add_argument('--web-archive');parser.add_argument('--server-binary')
    parser.add_argument('--rollback',action='store_true')
    args=parser.parse_args()
    if args.prepare: prepare(args)
    elif args.rollback: rollback()
    elif args.web_archive and args.server_binary: publish(args)
    else: parser.error('Choose --prepare, --rollback or supply build inputs')
