#!/usr/bin/env python3
"""Prepare a private-authenticated Postfix/OpenDKIM sender on the SameFrame host.

Public application sending remains disabled until DNS, PTR and direct delivery
have been verified. Run on the production host, never in a developer checkout.
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

from admin_api import AdminAPI

ROOT = Path('/opt/sameframe')
STATE = ROOT / 'mail'
DOMAIN = 'sameframe.top'
HOST = 'mail.sameframe.top'
SELECTOR = 'sf202610'
SENDER = 'no-reply@' + DOMAIN


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def put(path, value, mode=0o644):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value)
    path.chmod(mode)


def record(data):
    put(STATE / 'deployment.json', json.dumps(data, ensure_ascii=False, indent=2) + '\n', 0o600)


def install(resume=False):
    if os.geteuid() != 0:
        raise RuntimeError('Run this on the SameFrame server as root')
    if STATE.exists() and not resume:
        raise RuntimeError('Mail state already exists; inspect it before proceeding')
    if not resume and (shutil.which('postfix') or shutil.which('opendkim')):
        raise RuntimeError('An existing mail service must be reviewed before replacement')
    if not (ROOT / ('letsencrypt/live/' + DOMAIN + '/fullchain.pem')).is_file():
        raise RuntimeError('The existing site certificate is required')
    os.umask(0o077)
    api = AdminAPI()
    if resume:
        data = json.loads((STATE / 'deployment.json').read_text())
        backup = Path(data['backup_directory'])
        if data['status'] != 'backed_up' or (STATE / 'smtp-credentials.json').exists():
            raise RuntimeError('Resume is only for interruption before credential creation')
    else:
        STATE.mkdir(mode=0o700)
        backup = STATE / ('backup-' + time.strftime('%Y%m%d-%H%M%S', time.gmtime()))
        backup.mkdir(mode=0o700)
        previous = api.call('/api/v1/admin/smtp')
        if previous['config']['enabled'] or previous['config'].get('has_password'):
            raise RuntimeError('Expected the previously unconfigured SMTP service')
        put(backup / 'smtp-before.json', json.dumps(previous), 0o600)
        files = ['/etc/aliases', '/etc/mailname', '/opt/sameframe/renew-tls.sh']
        existed = []
        for name in files:
            path = Path(name)
            if path.exists():
                target = backup / path.relative_to('/')
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(path, target)
                existed.append(name)
        with (backup / 'database.dump').open('wb') as stream:
            run('docker', 'compose', 'exec', '-T', 'postgres', 'pg_dump', '-U', 'sameframe', '-d', 'sameframe', '-Fc', cwd=ROOT, stdout=stream)
        with (backup / 'database.dump').open('rb') as stream:
            run('docker', 'compose', 'exec', '-T', 'postgres', 'pg_restore', '--list', cwd=ROOT, stdin=stream, stdout=subprocess.DEVNULL)
        data = {'status': 'backed_up', 'backup_directory': str(backup),
                'previous_files': existed, 'domain': DOMAIN, 'sender': SENDER,
                'mail_hostname': HOST, 'selector': SELECTOR, 'previous_app_revision': '4bbf313',
                'started_at_utc': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                'rollback_command': 'python3 /opt/sameframe/mail-ops/install.py --rollback',
                'database_rollback_policy': 'Do not restore the database to undo mail configuration.',
                'backup_sha256': {p.relative_to(backup).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                                  for p in backup.rglob('*') if p.is_file()}}
        record(data)
    # Debian packages must not briefly expose their default SMTP policy.
    run('systemctl', 'mask', '--runtime', 'postfix.service', 'postfix@-.service', 'opendkim.service')
    env = {**os.environ, 'DEBIAN_FRONTEND': 'noninteractive'}
    run('apt-get', 'update', env=env)
    run('apt-get', '-o', 'DPkg::Lock::Timeout=300', 'install', '-y', '--no-install-recommends', 'postfix', 'opendkim',
        'opendkim-tools', 'sasl2-bin', 'libsasl2-modules', 'ca-certificates', 'python3-dkim', env=env)
    for name in ['/etc/postfix', '/etc/opendkim.conf']:
        path = Path(name)
        target = backup / 'package-defaults' / path.relative_to('/')
        target.parent.mkdir(parents=True, exist_ok=True)
        if path.is_dir():
            shutil.copytree(path, target)
        else:
            shutil.copy2(path, target)
    password = secrets.token_urlsafe(36)
    credentials = {'host': DOMAIN, 'port': 587, 'security': 'starttls',
                   'username': SENDER, 'password': password, 'from': SENDER,
                   'from_name': '同帧 SameFrame', 'base_url': 'https://' + DOMAIN}
    put(STATE / 'smtp-credentials.json', json.dumps(credentials), 0o600)
    Path('/etc/postfix/sasl').mkdir(parents=True, exist_ok=True)
    Path('/etc/postfix/sasl').chmod(0o755)
    run('saslpasswd2', '-p', '-c', '-f', '/etc/postfix/sasl/sasldb2', '-u', DOMAIN, 'no-reply', input=password + '\n', text=True)
    run('chown', 'root:postfix', '/etc/postfix/sasl/sasldb2')
    Path('/etc/postfix/sasl/sasldb2').chmod(0o640)
    put('/etc/postfix/sasl/smtpd.conf', 'pwcheck_method: auxprop\nauxprop_plugin: sasldb\nsasldb_path: /etc/postfix/sasl/sasldb2\nmech_list: PLAIN LOGIN\n')
    keydir = Path('/etc/opendkim/keys') / DOMAIN
    keydir.mkdir(parents=True, exist_ok=True)
    run('opendkim-genkey', '-b', '2048', '-d', DOMAIN, '-s', SELECTOR, '-D', str(keydir))
    run('chown', '-R', 'opendkim:opendkim', '/etc/opendkim')
    (keydir / (SELECTOR + '.private')).chmod(0o600)
    keydir.chmod(0o750)
    put('/etc/opendkim.conf', f'''Syslog yes
SyslogSuccess yes
Mode s
UserID opendkim
UMask 007
Canonicalization relaxed/simple
SignatureAlgorithm rsa-sha256
OversignHeaders From
Domain {DOMAIN}
Selector {SELECTOR}
KeyFile {keydir}/{SELECTOR}.private
Socket inet:8891@127.0.0.1
InternalHosts 127.0.0.1,172.19.0.0/16
PidFile /run/opendkim/opendkim.pid
''')
    run('useradd', '--system', '--create-home', '--home-dir', '/var/lib/sameframe-mail', '--shell', '/usr/sbin/nologin', 'sfmail')
    aliases = Path('/etc/aliases').read_text() if Path('/etc/aliases').exists() else ''
    aliases = '\n'.join(line for line in aliases.splitlines() if not line.split(':')[0].strip() in ['root', 'postmaster', 'no-reply', 'abuse'])
    put('/etc/aliases', aliases + '\nroot: sfmail\npostmaster: sfmail\nno-reply: sfmail\nabuse: sfmail\n')
    run('newaliases')
    put('/etc/mailname', DOMAIN + '\n')
    main = f'''compatibility_level = 3.10
myhostname = {HOST}
mydomain = {DOMAIN}
myorigin = {DOMAIN}
mydestination = {DOMAIN}, {HOST}, localhost, localhost.localdomain
inet_interfaces = all
inet_protocols = ipv4
relayhost =
relay_domains =
mynetworks = 127.0.0.0/8
alias_maps = hash:/etc/aliases
alias_database = hash:/etc/aliases
home_mailbox = Maildir/
mailbox_size_limit = 52428800
message_size_limit = 262144
authorized_submit_users = root
smtpd_banner = $myhostname ESMTP
always_add_missing_headers = yes
smtpd_tls_cert_file = /opt/sameframe/letsencrypt/live/{DOMAIN}/fullchain.pem
smtpd_tls_key_file = /opt/sameframe/letsencrypt/live/{DOMAIN}/privkey.pem
smtpd_tls_security_level = may
smtpd_tls_mandatory_protocols = >=TLSv1.2
smtpd_tls_auth_only = yes
smtpd_sasl_type = cyrus
smtpd_sasl_path = smtpd
cyrus_sasl_config_path = /etc/postfix/sasl
smtpd_sasl_auth_enable = no
smtpd_sasl_security_options = noanonymous
smtpd_sender_login_maps = hash:/etc/postfix/sameframe-senders
smtpd_relay_restrictions = reject_unauth_destination
smtpd_recipient_restrictions = reject_non_fqdn_recipient
smtpd_recipient_limit = 5
smtpd_client_connection_count_limit = 10
smtpd_client_connection_rate_limit = 20
smtpd_client_message_rate_limit = 30
smtpd_client_recipient_rate_limit = 60
smtpd_client_event_limit_exceptions =
disable_vrfy_command = yes
smtp_tls_security_level = encrypt
smtp_tls_CAfile = /etc/ssl/certs/ca-certificates.crt
smtp_tls_loglevel = 1
smtp_helo_name = {HOST}
smtp_destination_concurrency_limit = 2
smtp_destination_rate_delay = 2s
smtp_connect_timeout = 15s
maximal_queue_lifetime = 10m
bounce_queue_lifetime = 1d
milter_default_action = tempfail
milter_protocol = 6
smtpd_milters = inet:127.0.0.1:8891
non_smtpd_milters = inet:127.0.0.1:8891
maillog_file = /var/log/sameframe-mail.log
'''
    put('/etc/postfix/main.cf', main)
    put('/etc/postfix/sameframe-senders', SENDER + ' ' + SENDER + '\n')
    put('/etc/postfix/sameframe-recipient-gate', DOMAIN + ' OK\n')
    run('postmap', '/etc/postfix/sameframe-senders')
    run('postmap', '/etc/postfix/sameframe-recipient-gate')
    master = Path('/etc/postfix/master.cf').read_text()
    master = '\n'.join(line for line in master.splitlines()
                       if not (len(line.split()) > 1 and line.split()[0] in ['smtp', 'submission', 'submissions']
                               and line.split()[1] == 'inet'))
    master += '''
# SameFrame receives only local-domain mail on 25; no relay or login here.
smtp inet n - n - 12 smtpd
  -o smtpd_sasl_auth_enable=no
  -o smtpd_relay_restrictions=reject_unauth_destination
# Application submission requires verified TLS, a private password and an owned sender.
submission inet n - n - 6 smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_relay_restrictions=permit_sasl_authenticated,reject
  -o smtpd_sender_restrictions=reject_sender_login_mismatch
  -o smtpd_recipient_restrictions=check_recipient_access,hash:/etc/postfix/sameframe-recipient-gate,defer
  -o milter_macro_daemon_name=ORIGINATING
'''
    put('/etc/postfix/master.cf', master)
    run('opendkim', '-n', '-x', '/etc/opendkim.conf')
    run('postfix', 'check')
    # Debian's chroot copy can inherit systemd-resolved ownership.
    resolver = Path('/var/spool/postfix/etc/resolv.conf')
    if resolver.is_file() and not resolver.is_symlink():
        os.chown(resolver, 0, 0)
    renewal = ROOT / 'renew-tls.sh'
    script = renewal.read_text()
    script = script.replace('  docker compose restart mediamtx\n', '  docker compose restart mediamtx\n  if systemctl is-active --quiet postfix.service; then postfix reload; fi\n')
    renewal.write_text(script)
    put('/etc/logrotate.d/sameframe-mail', '/var/log/sameframe-mail.log {\n  daily\n  rotate 7\n  compress\n  missingok\n  notifempty\n  postrotate\n    /usr/sbin/postfix reload >/dev/null 2>&1 || true\n  endscript\n}\n')
    run('systemctl', 'unmask', '--runtime', 'postfix.service', 'postfix@-.service', 'opendkim.service')
    run('systemctl', 'enable', '--now', 'opendkim')
    run('systemctl', 'enable', '--now', 'postfix')
    current = api.call('/api/v1/admin/smtp')
    cfg = {**current['config'], **credentials, 'enabled': False}
    api.call('/api/v1/admin/smtp', {'version': current['version'], 'config': cfg}, method='PUT')
    dns = (keydir / (SELECTOR + '.txt')).read_text()
    import re
    dkim = ''.join(re.findall(r'"([^"]*)"', dns))
    records = [
        {'type': 'A', 'name': 'mail', 'content': '38.76.190.179', 'proxied': False},
        {'type': 'MX', 'name': '@', 'content': HOST, 'priority': 10},
        {'type': 'TXT', 'name': '@', 'content': 'v=spf1 ip4:38.76.190.179 -all'},
        {'type': 'TXT', 'name': SELECTOR + '._domainkey', 'content': dkim},
        {'type': 'TXT', 'name': '_dmarc', 'content': 'v=DMARC1; p=none; adkim=s; aspf=s'},
    ]
    put(STATE / 'dns-records.json', json.dumps(records, ensure_ascii=False, indent=2), 0o644)
    data.update(status='prepared_not_enabled', packages=run('dpkg-query', '-W', '-f=${Package} ${Version}\n', 'postfix', 'opendkim', 'sasl2-bin', capture_output=True, text=True).stdout.splitlines(),
                prepared_at_utc=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                pending=['Outbound TCP 25 currently times out', 'PTR is missing', 'Publish DNS records', 'Real recipient delivery test'])
    record(data)
    print(json.dumps({'status': data['status'], 'backup': str(backup), 'dns_records': str(STATE / 'dns-records.json')}))


def rollback():
    data = json.loads((STATE / 'deployment.json').read_text())
    backup = Path(data['backup_directory'])
    for name, expected in data['backup_sha256'].items():
        if hashlib.sha256((backup / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError('Backup checksum mismatch: ' + name)
    api = AdminAPI()
    current = api.call('/api/v1/admin/smtp')
    previous = json.loads((backup / 'smtp-before.json').read_text())
    api.call('/api/v1/admin/smtp', {'version': current['version'], 'config': previous['config'], 'clear_password': True}, method='PUT')
    run('systemctl', 'disable', '--now', 'postfix', 'opendkim')
    subprocess.run(['systemctl', 'stop', 'postfix@-.service'], check=False, stderr=subprocess.DEVNULL)
    for name in data['previous_files']:
        shutil.copy2(backup / Path(name).relative_to('/'), name)
    data['status'] = 'rolled_back_services_disabled'
    data['rollback_time_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    record(data)
    print('Previous application SMTP and renewal configuration restored; mail services disabled. Mail queues, keys and package files retained.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--rollback', action='store_true')
    parser.add_argument('--resume', action='store_true')
    args = parser.parse_args()
    rollback() if args.rollback else install(resume=args.resume)
