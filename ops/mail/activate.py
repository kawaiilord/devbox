#!/usr/bin/env python3
"""Enable sending only after the provider and DNS prerequisites pass."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import time

from admin_api import AdminAPI
from check import check, STATE, HOST


def run(*args):
    subprocess.run(args, check=True)


def activate(recipient):
    if not recipient or '@' not in recipient or any(c in recipient for c in '\r\n'):
        raise ValueError('Specify the user-authorized test recipient')
    result = check()
    (STATE / 'readiness.json').write_text(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['ready']:
        print(json.dumps(result, ensure_ascii=False, indent=2))
        raise SystemExit('Sending stays disabled: finish DNS, PTR and outbound TCP 25 first.')
    run('docker', 'run', '--rm', '-v', '/opt/sameframe/letsencrypt:/etc/letsencrypt',
        '-v', '/opt/sameframe/acme:/var/www/acme', 'certbot/certbot:v5.4.0',
        'certonly', '--webroot', '-w', '/var/www/acme', '--cert-name', HOST, '-d', HOST,
        '--non-interactive', '--agree-tos')
    run('postconf', '-e', 'smtpd_tls_cert_file=/opt/sameframe/letsencrypt/live/' + HOST + '/fullchain.pem')
    run('postconf', '-e', 'smtpd_tls_key_file=/opt/sameframe/letsencrypt/live/' + HOST + '/privkey.pem')
    credentials = json.loads((STATE / 'smtp-credentials.json').read_text())
    credentials['host'] = HOST
    (STATE / 'smtp-credentials.json').write_text(json.dumps(credentials))
    api = AdminAPI()
    old = api.call('/api/v1/admin/smtp')
    master = Path('/etc/postfix/master.cf')
    old_master = master.read_text()
    gate = '  -o smtpd_recipient_restrictions=check_recipient_access,hash:/etc/postfix/sameframe-recipient-gate,defer'
    if gate not in old_master:
        raise RuntimeError('Expected the closed submission gate')
    try:
        master.write_text(old_master.replace(gate, '  -o smtpd_recipient_restrictions=reject_non_fqdn_recipient'))
        run('postfix', 'check')
        run('postfix', 'reload')
        api.call('/api/v1/admin/smtp', {'version': old['version'], 'config': {**old['config'], **credentials, 'enabled': True}}, method='PUT')
        api.call('/api/v1/admin/smtp/test', {'to': recipient})
    except Exception:
        master.write_text(old_master)
        run('postfix', 'reload')
        current = api.call('/api/v1/admin/smtp')
        api.call('/api/v1/admin/smtp', {'version': current['version'], 'config': {**current['config'], 'enabled': False}}, method='PUT')
        raise
    manifest = STATE / 'deployment.json'
    data = json.loads(manifest.read_text())
    data.update(status='enabled_test_queued', enabled_at_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), pending=['Check delivery log and recipient inbox'])
    manifest.write_text(json.dumps(data, ensure_ascii=False, indent=2))
    print('SMTP enabled; one test message queued. Inspect /var/log/sameframe-mail.log and confirm receipt before reporting delivery success.')


if __name__ == '__main__':
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--test-to', required=True)
    activate(parser.parse_args().test_to)
