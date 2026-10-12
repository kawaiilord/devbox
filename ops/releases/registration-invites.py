#!/usr/bin/env python3
"""Deploy invitations with a brief registration pause and policy-aware rollback.

Place membership.py beside this script on the host. Existing accounts can keep
logging in while registration is paused. Rollback preserves user activity.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import urllib.request

ROOT = Path('/opt/sameframe')


class AdminAPI:
    def __init__(self):
        account = json.loads((ROOT / 'admin-access.json').read_text())
        self.headers = {'Content-Type': 'application/json', 'X-Device-ID': account['device'],
                        'X-Device-Name': 'Admin Web', 'X-Device-Platform': 'web-admin',
                        'X-Confirm-Dangerous': 'update-site-policy'}
        session = self.call('/api/v1/auth/login', {'email': account['email'], 'password': account['password']})
        self.headers['Authorization'] = 'Bearer ' + session['access_token']

    def call(self, path, data=None, method=None):
        request = urllib.request.Request('http://127.0.0.1:18082' + path,
            data=json.dumps(data).encode() if data is not None else None, method=method, headers=self.headers)
        with urllib.request.urlopen(request, timeout=25) as response:
            body = json.load(response)
        if body.get('code') != 0:
            raise RuntimeError(body.get('msg', 'Administration request failed'))
        return body.get('data', {})

    def policy(self, patch):
        current = self.call('/api/v1/admin/site-policy')
        # The previous application version may not know the new invitation field.
        current['policy'].update({k: v for k, v in patch.items() if k in current['policy']})
        return self.call('/api/v1/admin/site-policy', current, method='PUT')


def save(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')


def restore_policy(api, record):
    original = record['previous']['policy']
    return api.policy({'registration_enabled': original['registration_enabled'],
                       'registration_requires_invite': original.get('registration_requires_invite', False)})


def deploy(args):
    if not args.revision or any(char not in '0123456789abcdef' for char in args.revision):
        raise ValueError('Supply the built hexadecimal revision')
    release = ROOT / 'releases' / ('registration-invites-' + args.revision)
    pending = ROOT / 'releases' / ('invitation-policy-' + args.revision + '.json')
    if release.exists() or pending.exists():
        raise RuntimeError('Release state already exists; inspect it before retrying')
    api = AdminAPI()
    previous = api.call('/api/v1/admin/site-policy')
    record = {'previous': previous, 'status': 'backed_up', 'revision': args.revision,
              'rollback_command': 'python3 ' + str(release / 'invitation-release.py') + ' --rollback'}
    save(pending, record)
    try:
        api.policy({'registration_enabled': False})
        subprocess.run(['python3', str(Path(__file__).resolve().with_name('membership.py')),
            '--name', 'registration-invites', '--revision', args.revision,
            '--database-note', '021_registration_invites (additive)',
            '--web-archive', args.web_archive, '--server-binary', args.server_binary], check=True)
        shutil.copyfile(__file__, release / 'invitation-release.py')
        save(release / 'invitation-policy.json', record)
        current = api.policy({'registration_enabled': previous['policy']['registration_enabled'],
                              'registration_requires_invite': True})
        rules = api.call('/api/v1/auth/options')
        if not rules.get('registration_invite_required') or rules.get('registration_email_code_required'):
            raise RuntimeError('Deployed registration requirements do not match invitation mode')
        record.update(status='deployed', current=current,
                      deployed_at_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
        save(release / 'invitation-policy.json', record)
        save(pending, record)
    except Exception:
        manifest = release / 'deployment.json'
        if manifest.exists() and json.loads(manifest.read_text()).get('status') == 'deployed':
            subprocess.run(['python3', str(release / 'release.py'), '--rollback'], check=True)
        restore_policy(AdminAPI(), record)
        record['status'] = 'failed_restored'
        save(pending, record)
        raise
    print(json.dumps({'status': record['status'], 'revision': args.revision,
                      'rollback_command': record['rollback_command']}))


def rollback():
    release = Path(__file__).resolve().parent
    path = release / 'invitation-policy.json'
    record = json.loads(path.read_text())
    api = AdminAPI()
    api.policy({'registration_enabled': False})
    subprocess.run(['python3', str(release / 'release.py'), '--rollback'], check=True)
    restore_policy(AdminAPI(), record)
    record['status'] = 'rolled_back'
    record['rolled_back_at_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    save(path, record)
    print('Previous application and registration policy restored; new database activity retained.')


if __name__ == '__main__':
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--revision')
    parser.add_argument('--web-archive')
    parser.add_argument('--server-binary')
    parser.add_argument('--rollback', action='store_true')
    args = parser.parse_args()
    if args.rollback:
        rollback()
    elif args.web_archive and args.server_binary:
        deploy(args)
    else:
        parser.error('--web-archive and --server-binary are required')
