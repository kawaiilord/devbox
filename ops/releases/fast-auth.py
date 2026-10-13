#!/usr/bin/env python3
"""Deploy lightweight account pages and their canonical nginx routes together."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

ROOT = Path('/opt/sameframe')


def run(*args):
    return subprocess.run(args, cwd=ROOT, check=True)


def save(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')


def reload_nginx():
    run('docker', 'compose', 'exec', '-T', 'web', 'nginx', '-t')
    run('docker', 'compose', 'exec', '-T', 'web', 'nginx', '-s', 'reload')


def deploy(args):
    if not args.revision or any(char not in '0123456789abcdef' for char in args.revision):
        raise ValueError('Supply the built hexadecimal revision')
    release = ROOT / 'releases' / ('fast-auth-' + args.revision)
    pending = ROOT / 'releases' / ('fast-auth-routing-' + args.revision + '.json')
    if release.exists() or pending.exists():
        raise RuntimeError('Release state already exists; inspect before retrying')
    old = (ROOT / 'nginx.conf').read_text()
    marker = '  location /internal/ { return 404; }'
    if old.count(marker) != 2 or 'location = /login {' in old:
        raise RuntimeError('Expected the existing IP and domain HTTPS virtual hosts')
    snippet = Path(args.routes).read_text()
    new = old.replace(marker, '\n'.join('  ' + line for line in snippet.splitlines()) + '\n' + marker)
    backup = ROOT / 'backups' / ('fast-auth-routing-' + time.strftime('%Y%m%d-%H%M%S', time.gmtime()))
    backup.mkdir(mode=0o700, parents=True)
    shutil.copy2(ROOT / 'nginx.conf', backup / 'nginx.conf')
    record = {'status':'backed_up', 'revision':args.revision, 'backup_directory':str(backup),
              'nginx_sha256':hashlib.sha256(old.encode()).hexdigest(),
              'rollback_command':'python3 ' + str(release / 'fast-auth-release.py') + ' --rollback'}
    save(pending, record)
    try:
        run('python3', str(Path(__file__).resolve().with_name('membership.py')),
            '--name', 'fast-auth', '--revision', args.revision,
            '--database-note', 'no schema change; registration email toggle in existing site policy',
            '--web-archive', args.web_archive, '--server-binary', args.server_binary)
        shutil.copyfile(__file__, release / 'fast-auth-release.py')
        save(release / 'routing.json', record)
        # nginx.conf is bind-mounted: retain its inode.
        (ROOT / 'nginx.conf').write_text(new)
        reload_nginx()
        record.update(status='deployed',deployed_at_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()))
        save(release / 'routing.json', record)
        save(pending, record)
        manifest=release / 'deployment.json'
        data=json.loads(manifest.read_text())
        data['application_only_rollback_command']=data['rollback_command']
        data['rollback_command']=record['rollback_command']
        data['routing_record']=str(release / 'routing.json')
        save(manifest,data)
    except Exception:
        (ROOT / 'nginx.conf').write_text(old)
        manifest=release / 'deployment.json'
        if manifest.exists() and json.loads(manifest.read_text()).get('status')=='deployed':
            run('python3',str(release / 'release.py'),'--rollback')
        reload_nginx()
        record['status']='failed_restored'
        save(pending,record)
        raise
    print(json.dumps(record))


def rollback():
    release=Path(__file__).resolve().parent
    path=release / 'routing.json';record=json.loads(path.read_text())
    old=(Path(record['backup_directory']) / 'nginx.conf').read_bytes()
    if hashlib.sha256(old).hexdigest()!=record['nginx_sha256']:
        raise RuntimeError('Original nginx configuration checksum mismatch')
    run('python3',str(release / 'release.py'),'--check-rollback')
    (ROOT / 'nginx.conf').write_bytes(old)
    run('python3',str(release / 'release.py'),'--rollback')
    reload_nginx()
    record['status']='rolled_back'
    save(path,record)
    print('Previous application, web files and nginx routes restored; database activity retained.')


if __name__=='__main__':
    os.umask(0o077)
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--revision');parser.add_argument('--web-archive');parser.add_argument('--server-binary')
    parser.add_argument('--routes');parser.add_argument('--rollback',action='store_true')
    args=parser.parse_args()
    if args.rollback:rollback()
    elif args.web_archive and args.server_binary and args.routes:deploy(args)
    else:parser.error('--web-archive, --server-binary and --routes are required')
