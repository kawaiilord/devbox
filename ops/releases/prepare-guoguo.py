#!/usr/bin/env python3
"""Install the private Guoguo provider and back up its Compose configuration.

Run before the ordinary web/backend release. --rollback restores only service
configuration; it never restores the database or removes provider data.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time

import yaml


def run(args, root, **kwargs):
    return subprocess.run(args, cwd=root, check=True, **kwargs)


def save(path, data):
    pending = path.with_suffix('.next')
    pending.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    pending.replace(path)


def restore(root, record):
    override = root / 'compose.override.yml'
    backup = Path(record['backup']) / 'compose.override.yml'
    shutil.copy2(backup, override)
    run(['docker', 'compose', 'up', '-d', '--no-deps', '--no-build', '--force-recreate', 'app'], root)
    # The exact provider was created by this release and is not used by the
    # baseline app. Retain its container and volume for recovery/inspection.
    run(['docker', 'stop', record['container']], root)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', default='/opt/sameframe')
    parser.add_argument('--revision')
    parser.add_argument('--provider-binary')
    parser.add_argument('--rollback', action='store_true')
    args = parser.parse_args()
    root = Path(args.root).resolve()
    if args.rollback:
        manifest = Path(__file__).resolve().parent / 'provider-deployment.json'
        record = json.loads(manifest.read_text())
        restore(root, record)
        record['status'] = 'rolled_back'
        save(manifest, record)
        print(json.dumps({'status': record['status'], 'data_preserved': True}))
        return
    if not args.revision or any(c not in '0123456789abcdef' for c in args.revision):
        parser.error('revision must be a commit hash')
    binary = Path(args.provider_binary).resolve()
    release = root / 'releases' / ('guoguo-provider-' + args.revision)
    release.mkdir(mode=0o700, exist_ok=False)
    backup = root / 'backups' / ('guoguo-provider-' + time.strftime('%Y%m%d-%H%M%S', time.gmtime()))
    backup.mkdir(mode=0o700)
    override = root / 'compose.override.yml'
    config = yaml.safe_load(override.read_text())
    if 'guoguo' in config.get('services', {}):
        raise RuntimeError('A provider already exists; this installer expects the pre-Guoguo baseline')
    shutil.copy2(override, backup / override.name)
    shutil.copy2(root / 'compose.yml', backup / 'compose.yml')
    shutil.copy2(__file__, release / 'prepare-guoguo.py')
    provider = root / 'guoguo'
    provider.mkdir(mode=0o755, exist_ok=True)
    env = provider / 'bridge.env'
    if env.exists():
        raise RuntimeError('Provider secret already exists; inspect the earlier installation')
    env.write_text('SAMEFRAME_GUOGUO_TOKEN=' + secrets.token_urlsafe(40) + '\n')
    for name in ['data', 'downloads']:
        directory = provider / name
        directory.mkdir(mode=0o700, exist_ok=True)
        os.chown(directory, 65532, 65532)
    image = 'sameframe-guoguo:' + args.revision
    shutil.copyfile(binary, release / 'guoguo')
    (release / 'guoguo').chmod(0o755)
    # The inspected runtime already contains FFmpeg, CA roots and an unprivileged
    # UID. Replace only the binary with the exact CI artifact being released.
    base = run(['docker', 'image', 'inspect', 'sameframe-guoguo:review', '--format', '{{.Id}}'],
               root, capture_output=True, text=True).stdout.strip()
    base_tag = 'sameframe-guoguo:runtime-' + base.removeprefix('sha256:')[:12]
    run(['docker', 'tag', base, base_tag], root)
    (release / 'Dockerfile').write_text('FROM ' + base_tag + '\nCOPY --chown=65532:65532 guoguo /usr/local/bin/guoguo\n')
    run(['docker', 'build', '-t', image, '.'], release)
    service = {
        'image': image, 'container_name': 'sameframe-guoguo',
        'restart': 'unless-stopped', 'env_file': ['./guoguo/bridge.env'],
        'environment': {'SAMEFRAME_BRIDGE': '1'},
        'volumes': ['./guoguo/data:/data', './guoguo/downloads:/downloads'],
        'mem_limit': '1200m', 'cpus': 2.5,
        'security_opt': ['no-new-privileges:true'],
        'healthcheck': {
            'test': ['CMD-SHELL', 'curl --noproxy "*" -fsS -H "Authorization: Bearer $$SAMEFRAME_GUOGUO_TOKEN" http://127.0.0.1:8999/bridge/health >/dev/null'],
            'interval': '20s', 'timeout': '5s', 'retries': 3, 'start_period': '10s',
        },
        'logging': {'driver': 'json-file', 'options': {'max-size': '10m', 'max-file': '3'}},
    }
    config.setdefault('services', {})['guoguo'] = service
    app = config['services'].setdefault('app', {})
    app.setdefault('env_file', []).append('./guoguo/bridge.env')
    app.setdefault('environment', {})['SAMEFRAME_GUOGUO_URL'] = 'http://guoguo:8999'
    pending = override.with_suffix('.next.yml')
    pending.write_text(yaml.safe_dump(config, allow_unicode=True, sort_keys=False))
    run(['docker', 'compose', '-f', 'compose.yml', '-f', str(pending), 'config', '--quiet'], root)
    manifest = release / 'provider-deployment.json'
    record = {'status': 'prepared', 'revision': args.revision, 'backup': str(backup),
              'image': image, 'container': 'sameframe-guoguo', 'ports_published': [],
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'rollback': 'python3 ' + str(release / 'prepare-guoguo.py') + ' --rollback',
              'data_policy': 'Preserve provider data and all database activity on rollback'}
    save(manifest, record)
    pending.replace(override)
    try:
        run(['docker', 'compose', 'up', '-d', '--no-build', '--no-deps', 'guoguo'], root)
        ready = False
        for _ in range(35):
            state = run(['docker', 'inspect', 'sameframe-guoguo', '--format', '{{.State.Health.Status}}'],
                        root, capture_output=True, text=True).stdout.strip()
            if state == 'healthy':
                ready = True
                break
            time.sleep(1)
        if not ready:
            raise RuntimeError('Private provider health check failed')
    except Exception:
        shutil.copy2(backup / override.name, override)
        record['status'] = 'failed_configuration_restored'
        save(manifest, record)
        raise
    record['status'] = 'provider_ready_app_pending'
    save(manifest, record)
    print(json.dumps(record, ensure_ascii=False))


if __name__ == '__main__':
    main()
