#!/usr/bin/env python3
"""Replace only the private provider image, retaining its data and configuration."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

import yaml


def run(root, args, capture=False):
    return subprocess.run(args, cwd=root, check=True, capture_output=capture, text=capture)


def image_config(root, image):
    path = root / 'compose.override.yml'
    config = yaml.safe_load(path.read_text())
    config['services']['guoguo']['image'] = image
    pending = path.with_suffix('.next.yml')
    pending.write_text(yaml.safe_dump(config, allow_unicode=True, sort_keys=False))
    run(root, ['docker', 'compose', '-f', 'compose.yml', '-f', str(pending), 'config', '--quiet'])
    pending.replace(path)
    run(root, ['docker', 'compose', 'up', '-d', '--no-build', '--no-deps', '--force-recreate', 'guoguo'])


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', default='/opt/sameframe')
    parser.add_argument('--revision')
    parser.add_argument('--binary')
    parser.add_argument('--rollback', action='store_true')
    args = parser.parse_args()
    root = Path(args.root)
    if args.rollback:
        manifest = Path(__file__).resolve().parent / 'provider-update.json'
        record = json.loads(manifest.read_text())
        image_config(root, record['rollback_image'])
        record['status'] = 'rolled_back'
        manifest.write_text(json.dumps(record, indent=2) + '\n')
        print('Provider image restored; data preserved')
        return
    if not args.revision or len(args.revision) != 40 or any(c not in 'abcdef0123456789' for c in args.revision):
        parser.error('exact commit hash required')
    release = root / 'releases' / ('guoguo-provider-update-' + args.revision)
    release.mkdir(mode=0o700)
    shutil.copy2(root/'compose.override.yml', release/'compose.override.before.yml')
    shutil.copy2(__file__, release/'update-guoguo.py')
    previous = run(root, ['docker', 'inspect', 'sameframe-guoguo', '--format', '{{.Image}}'], True).stdout.strip()
    rollback_image = 'sameframe-guoguo:before-' + args.revision
    run(root, ['docker', 'tag', previous, rollback_image])
    binary = Path(args.binary)
    shutil.copyfile(binary, release/'guoguo')
    (release/'guoguo').chmod(0o755)
    (release/'Dockerfile').write_text('FROM ' + rollback_image + '\nCOPY --chown=65532:65532 guoguo /usr/local/bin/guoguo\n')
    image = 'sameframe-guoguo:' + args.revision
    run(release, ['docker', 'build', '-t', image, '.'])
    record = {'status':'prepared','revision':args.revision,'previous_image':previous,'rollback_image':rollback_image,
              'image':image,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),
              'rollback_command':'python3 ' + str(release/'update-guoguo.py') + ' --rollback',
              'data_policy':'Retain provider data; rollback changes only the image key in Compose'}
    manifest = release/'provider-update.json'
    manifest.write_text(json.dumps(record, indent=2)+'\n')
    try:
        image_config(root, image)
        healthy = False
        for _ in range(40):
            state = run(root, ['docker','inspect','sameframe-guoguo','--format','{{.State.Health.Status}}'], True).stdout.strip()
            if state == 'healthy':
                healthy = True
                break
            time.sleep(1)
        if not healthy:
            raise RuntimeError('Updated provider did not become healthy')
    except Exception:
        image_config(root, rollback_image)
        record['status'] = 'failed_rolled_back'
        manifest.write_text(json.dumps(record, indent=2)+'\n')
        raise
    record['status'] = 'deployed'
    manifest.write_text(json.dumps(record, indent=2)+'\n')
    print(json.dumps(record))


if __name__ == '__main__':
    main()
