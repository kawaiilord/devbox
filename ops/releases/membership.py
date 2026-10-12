#!/usr/bin/env python3
"""Deploy the membership release, preserving an independently runnable rollback.

Only the app service and web files are replaced. Ordinary rollback preserves
all database activity, including points earned/spent after deployment.
"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import tarfile
import time
import urllib.error
import urllib.request

ENTRYPOINTS = ['main.dart.js', 'flutter_bootstrap.js', 'flutter_service_worker.js', 'version.json', 'app.html', 'login.html', 'register.html', 'index.html']


def command(args, root, **kwargs):
    return subprocess.run(args, cwd=root, check=True, **kwargs)


def output(args, root):
    return command(args, root, capture_output=True, text=True).stdout.strip()


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def save(path, value):
    pending = path.with_suffix('.tmp')
    pending.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    os.replace(pending, path)


def install_binary(source, root):
    target = root / 'bin/sameframe'
    pending = target.with_suffix('.next')
    shutil.copyfile(source, pending)
    pending.chmod(0o755)
    os.replace(pending, target)


def install_web(source, root):
    for required in ['main.dart.js', 'index.html', 'flutter_bootstrap.js']:
        if not (source / required).is_file():
            raise RuntimeError('Incomplete web release: ' + required)
    files = [p for p in source.rglob('*') if p.is_file()]
    files.sort(key=lambda p: (p.relative_to(source).as_posix() in ENTRYPOINTS,
        ENTRYPOINTS.index(p.relative_to(source).as_posix()) if p.relative_to(source).as_posix() in ENTRYPOINTS else 0))
    for item in files:
        target = root / 'web' / item.relative_to(source)
        target.parent.mkdir(parents=True, exist_ok=True)
        for parent in [target.parent, *target.parents]:
            if parent == root:
                break
            parent.chmod(0o755)
        pending = target.with_name(target.name + '.next')
        shutil.copyfile(item, pending)
        pending.chmod(0o644)
        os.replace(pending, target)


def ready():
    last = None
    for _ in range(45):
        try:
            with urllib.request.urlopen('http://127.0.0.1:18082/readyz', timeout=3) as response:
                data = json.load(response)
                if data.get('data', {}).get('status') == 'ready':
                    return
        except (OSError, ValueError) as error:
            last = error
        time.sleep(1)
    raise RuntimeError('App readiness check failed: ' + str(last))


def check_rollback(record):
    backup = Path(record['backup_directory'])
    for name, expected in record['backup_sha256'].items():
        if digest(backup / name) != expected:
            raise RuntimeError('Backup checksum mismatch: ' + name)
    output(['docker', 'image', 'inspect', record['rollback_image'], '--format', '{{.Id}}'], Path(record['root']))


def rollback(record, manifest):
    check_rollback(record)
    root, backup = Path(record['root']), Path(record['backup_directory'])
    stage = manifest.parent / 'rollback-web'
    stage.mkdir(exist_ok=True)
    with tarfile.open(backup / 'web.tgz', 'r:gz') as archive:
        archive.extractall(stage, filter='data')
    install_web(stage / 'web', root)
    # Remove only files introduced by this release, preserving unrelated files.
    web = (root / 'web').resolve()
    for relative in record.get('added_web_files', []):
        target = (web / relative).resolve()
        if not target.is_relative_to(web):
            raise RuntimeError('Invalid rollback file path')
        target.unlink(missing_ok=True)
    install_binary(backup / 'sameframe', root)
    command(['docker', 'tag', record['rollback_image'], record['runtime_image_name']], root)
    command(['docker', 'compose', 'up', '-d', '--no-build', '--no-deps', '--force-recreate', 'app'], root)
    ready()
    record['status'] = 'rolled_back'
    record['rollback_time_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    save(manifest, record)


def deploy(args):
    root = Path(args.root).resolve()
    if not args.revision or any(c not in '0123456789abcdef' for c in args.revision):
        raise ValueError('A hexadecimal revision is required')
    if not re.fullmatch(r'[a-z0-9-]+', args.name):
        raise ValueError('A lowercase release name is required')
    release = root / 'releases' / (args.name + '-' + args.revision)
    release.mkdir(parents=True, exist_ok=False)
    manifest = release / 'deployment.json'
    stage = release / 'web'
    stage.mkdir()
    with tarfile.open(args.web_archive, 'r:gz') as archive:
        archive.extractall(stage, filter='data')
    for name in ['main.dart.js', 'index.html', 'flutter_bootstrap.js']:
        if not (stage / name).is_file():
            raise RuntimeError('Incomplete build: ' + name)
    build = release / 'runtime'
    build.mkdir()
    shutil.copyfile(args.server_binary, build / 'sameframe')
    (build / 'sameframe').chmod(0o755)
    shutil.copyfile(__file__, release / 'release.py')
    backup = root / 'backups' / (args.name + '-' + time.strftime('%Y%m%d-%H%M%S', time.gmtime()))
    backup.mkdir(parents=True, mode=0o700)
    shutil.copy2(root / 'bin/sameframe', backup / 'sameframe')
    with tarfile.open(backup / 'web.tgz', 'w:gz') as archive:
        archive.add(root / 'web', arcname='web')
    with (backup / 'database.dump').open('wb') as stream:
        command(['docker', 'compose', 'exec', '-T', 'postgres', 'pg_dump', '-U', 'sameframe', '-d', 'sameframe', '-Fc'], root, stdout=stream)
    with (backup / 'database.dump').open('rb') as stream:
        command(['docker', 'compose', 'exec', '-T', 'postgres', 'pg_restore', '--list'], root, stdin=stream, stdout=subprocess.DEVNULL)
    container = output(['docker', 'compose', 'ps', '-q', 'app'], root)
    old_image = output(['docker', 'inspect', container, '--format', '{{.Image}}'], root)
    runtime_name = output(['docker', 'inspect', container, '--format', '{{.Config.Image}}'], root)
    rollback_image = 'sameframe-app:before-' + args.name + '-' + args.revision
    new_image = 'sameframe-app:' + args.name + '-' + args.revision
    command(['docker', 'tag', old_image, rollback_image], root)
    record = {
        'revision': args.revision, 'root': str(root), 'status': 'backed_up',
        'created_at_utc': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        'backup_directory': str(backup), 'runtime_image_name': runtime_name,
        'rollback_image': rollback_image, 'previous_image_id': old_image,
        'new_image': new_image, 'database_migration': args.database_note,
        'previous_web_sha256': digest(root / 'web/main.dart.js'),
        'backup_sha256': {name: digest(backup / name) for name in ['sameframe', 'web.tgz', 'database.dump']},
        'server_sha256': digest(build / 'sameframe'), 'web_sha256': digest(stage / 'main.dart.js'),
        'added_web_files': [p.relative_to(stage).as_posix() for p in stage.rglob('*')
            if p.is_file() and not (root / 'web' / p.relative_to(stage)).exists()],
        'rollback_command': 'python3 ' + str(release / 'release.py') + ' --rollback',
        'rollback_check_command': 'python3 ' + str(release / 'release.py') + ' --check-rollback',
        'database_rollback_policy': 'Preserve new user activity; normal rollback does not restore the database.',
    }
    save(manifest, record)
    (build / 'Dockerfile').write_text('FROM ' + rollback_image + '\nCOPY --chown=65532:65532 sameframe /sameframe\n')
    try:
        command(['docker', 'build', '-t', new_image, '.'], build)
        install_binary(build / 'sameframe', root)
        command(['docker', 'tag', new_image, runtime_name], root)
        command(['docker', 'compose', 'up', '-d', '--no-build', '--no-deps', '--force-recreate', 'app'], root)
        ready()
        request = urllib.request.Request('http://127.0.0.1:18082/api/v1/points/redeem-month', data=b'{}', headers={'Content-Type': 'application/json'})
        try:
            urllib.request.urlopen(request, timeout=5)
            raise RuntimeError('Redemption endpoint unexpectedly accepted anonymous access')
        except urllib.error.HTTPError as error:
            if error.code != 401:
                raise
        install_web(stage, root)
        record['status'] = 'deployed'
        record['deployed_at_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
        save(manifest, record)
        check_rollback(record)
    except Exception:
        rollback(record, manifest)
        raise
    print(json.dumps(record, ensure_ascii=False))


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', default='/opt/sameframe')
    parser.add_argument('--revision')
    parser.add_argument('--name', default='membership')
    parser.add_argument('--database-note', default='020_membership_rewards (additive)')
    parser.add_argument('--web-archive')
    parser.add_argument('--server-binary')
    parser.add_argument('--rollback', action='store_true')
    parser.add_argument('--check-rollback', action='store_true')
    args = parser.parse_args()
    if args.rollback or args.check_rollback:
        manifest = Path(__file__).resolve().parent / 'deployment.json'
        record = json.loads(manifest.read_text())
        if args.rollback:
            rollback(record, manifest)
        else:
            check_rollback(record)
        print(json.dumps({'status': record['status'], 'rollback_verified': True}))
    else:
        if not args.web_archive or not args.server_binary:
            parser.error('--web-archive and --server-binary are required')
        deploy(args)


if __name__ == '__main__':
    main()
