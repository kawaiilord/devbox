#!/usr/bin/env python3
"""Attach a certificated domain to the existing SameFrame IP deployment."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time
import urllib.request


def run(root, *args):
    subprocess.run(args, cwd=root, check=True)


def checksum(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, record):
    path.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')


def ready():
    for _ in range(45):
        try:
            with urllib.request.urlopen('http://127.0.0.1:18082/readyz', timeout=3) as response:
                if json.load(response).get('data', {}).get('status') == 'ready':
                    return
        except (OSError, ValueError):
            pass
        time.sleep(1)
    raise RuntimeError('Application did not become ready')


def apply(root):
    run(root, 'docker', 'compose', 'config', '--quiet')
    run(root, 'docker', 'compose', 'exec', '-T', 'web', 'nginx', '-t')
    run(root, 'docker', 'compose', 'up', '-d', '--no-build', '--no-deps', '--force-recreate', 'app')
    ready()
    run(root, 'docker', 'compose', 'exec', '-T', 'web', 'nginx', '-s', 'reload')


def check(record):
    backup = Path(record['backup_directory'])
    for name, digest in record['backup_sha256'].items():
        if checksum(backup / name) != digest:
            raise RuntimeError('Backup checksum mismatch: ' + name)


def rollback(record, manifest):
    check(record)
    root, backup = Path(record['root']), Path(record['backup_directory'])
    for name in record['backup_sha256']:
        # Preserve the inode of the nginx bind-mounted file.
        (root / name).write_bytes((backup / name).read_bytes())
    apply(root)
    record['status'] = 'rolled_back'
    save(manifest, record)


def configure(args):
    if not args.domain or not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}', args.domain):
        raise ValueError('Supply a lowercase DNS hostname')
    root = Path(args.root).resolve()
    domain = args.domain
    for name in ['fullchain.pem', 'privkey.pem']:
        if not (root / 'letsencrypt/live' / domain / name).is_file():
            raise RuntimeError('Issue the domain webroot certificate first')
    nginx = (root / 'nginx.conf').read_text()
    if nginx.count('server {') != 2 or nginx.count('  listen 443 ssl;') != 1:
        raise RuntimeError('Expected the original HTTP and IP HTTPS virtual hosts')
    tls = nginx[nginx.index('server {\n  listen 443 ssl;'):]
    tls = tls.replace('server_name _;', 'server_name ' + domain + ';')
    tls = re.sub(r'/etc/letsencrypt/live/[^/]+/', '/etc/letsencrypt/live/' + domain + '/', tls)
    nginx += '\n# Official domain; the original IP remains available.\n' + tls
    env = (root / '.env').read_text()
    match = re.search(r'^PUBLIC_URL=(.+)$', env, re.M)
    if not match:
        raise RuntimeError('PUBLIC_URL is missing')
    old_origin = match.group(1).strip().strip('\"\'')
    existing = re.search(r'^ALLOWED_ORIGINS=(.+)$', env, re.M)
    origins = list(dict.fromkeys(['https://' + domain, old_origin] +
        (existing.group(1).strip().strip('\"\'').split(',') if existing else [])))
    env = re.sub(r'^PUBLIC_URL=.+$', 'PUBLIC_URL=https://' + domain, env, flags=re.M)
    env = re.sub(r'^ALLOWED_ORIGINS=.*\n?', '', env, flags=re.M).rstrip() + '\nALLOWED_ORIGINS=' + ','.join(origins) + '\n'
    compose = (root / 'compose.yml').read_text()
    old = 'SAMEFRAME_ALLOWED_ORIGINS: ${PUBLIC_URL:?required}'
    new = 'SAMEFRAME_ALLOWED_ORIGINS: ${ALLOWED_ORIGINS:-${PUBLIC_URL:?required}}'
    if old not in compose and new not in compose:
        raise RuntimeError('Unexpected allowed-origin configuration')
    compose = compose.replace(old, new)
    release = root / 'releases' / ('domain-' + time.strftime('%Y%m%d-%H%M%S', time.gmtime()))
    release.mkdir(parents=True, mode=0o700)
    backup = release / 'backup'
    backup.mkdir(mode=0o700)
    names = ['nginx.conf', 'compose.yml', '.env']
    for name in names:
        shutil.copy2(root / name, backup / name)
    shutil.copyfile(__file__, release / 'configure-domain.py')
    manifest = release / 'deployment.json'
    record = {
        'root': str(root), 'domain': domain, 'previous_public_origin': old_origin,
        'allowed_origins': origins, 'backup_directory': str(backup),
        'backup_sha256': {name: checksum(backup / name) for name in names},
        'status': 'backed_up', 'database_change': 'none',
        'rollback_command': 'python3 ' + str(release / 'configure-domain.py') + ' --rollback',
        'rollback_check_command': 'python3 ' + str(release / 'configure-domain.py') + ' --check-rollback',
    }
    save(manifest, record)
    try:
        for name, value in [('nginx.conf', nginx), ('compose.yml', compose), ('.env', env)]:
            (root / name).write_text(value)
        apply(root)
        record['status'] = 'configured'
        record['configured_at_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
        save(manifest, record)
    except Exception:
        rollback(record, manifest)
        raise
    print(json.dumps(record, ensure_ascii=False))


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--domain')
    parser.add_argument('--root', default='/opt/sameframe')
    parser.add_argument('--rollback', action='store_true')
    parser.add_argument('--check-rollback', action='store_true')
    args = parser.parse_args()
    if args.rollback or args.check_rollback:
        manifest = Path(__file__).resolve().parent / 'deployment.json'
        record = json.loads(manifest.read_text())
        if args.rollback:
            rollback(record, manifest)
        else:
            check(record)
        print(json.dumps({'status': record['status'], 'rollback_verified': True}))
    else:
        configure(args)


if __name__ == '__main__':
    main()
