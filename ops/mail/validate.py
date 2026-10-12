#!/usr/bin/env python3
"""Verify local TLS, SMTP authentication, relay rejection and DKIM signing.

Only delivers to the private local test mailbox; no external recipient is used.
"""
import json
from pathlib import Path
import smtplib
import ssl
import subprocess
import time
from email.message import EmailMessage
from email.parser import BytesParser
from email.utils import formatdate, make_msgid

import dkim
from admin_api import AdminAPI

STATE = Path('/opt/sameframe/mail')


def smtp(c):
    connection = smtplib.SMTP(c['host'], c['port'], timeout=12)
    connection.ehlo()
    connection.starttls(context=ssl.create_default_context())
    connection.ehlo()
    return connection


def validate():
    c = json.loads((STATE / 'smtp-credentials.json').read_text())
    results = []
    with smtp(c) as connection:
        connection.mail(c['from'])
        status, _ = connection.rcpt('relay-check@example.invalid')
        if status < 500:
            raise RuntimeError('Anonymous submission must be rejected')
        results.append('TLS certificate validated; unauthenticated relay rejected')
    with smtplib.SMTP('127.0.0.1', 25, timeout=10) as connection:
        connection.ehlo()
        connection.mail(c['from'])
        status, _ = connection.rcpt('relay-check@example.invalid')
        if status < 500:
            raise RuntimeError('Port 25 allowed unauthenticated relay')
        results.append('Port 25 rejects relay to external domains')
    with smtp(c) as connection:
        try:
            connection.login(c['username'], 'deliberately-invalid-password')
            raise RuntimeError('Invalid password accepted')
        except smtplib.SMTPAuthenticationError:
            results.append('Invalid SMTP password rejected')
    with smtp(c) as connection:
        connection.login(c['username'], c['password'])
        connection.mail('someone-else@example.invalid')
        status, _ = connection.rcpt(c['from'])
        if status < 500:
            raise RuntimeError('Unowned sender address accepted')
        results.append('Authenticated sender restricted to the configured address')
    with smtp(c) as connection:
        connection.login(c['username'], c['password'])
        connection.mail(c['from'])
        status, _ = connection.rcpt('pending-check@example.invalid')
        if status != 450:
            raise RuntimeError('Pending setup must block external delivery: ' + str(status))
        results.append('External delivery gate is closed while setup is pending')
    message = EmailMessage()
    message['From'] = c['from']
    message['To'] = c['from']
    message['Subject'] = 'SameFrame local mail acceptance'
    message['Date'] = formatdate(localtime=False)
    marker = make_msgid(domain='sameframe.top')
    # SameFrame's current SMTP sender omits Message-ID; Postfix must supply it.
    message['X-SameFrame-Acceptance'] = marker
    message.set_content('同帧自建发信服务内部验收：此邮件仅投递到本机测试邮箱。')
    # Exercise the same network namespace and trust store as the deployed app.
    payload = 'import smtplib,ssl,json\nc=json.loads(' + repr(json.dumps(c)) + ')\n'
    payload += 'with smtplib.SMTP(c["host"],c["port"],timeout=15) as s:\n'
    payload += ' s.ehlo();s.starttls(context=ssl.create_default_context());s.ehlo();s.login(c["username"],c["password"])\n'
    payload += ' s.sendmail(c["from"],[c["from"]],' + repr(message.as_bytes()) + ')\n'
    payload += 'print("Application network: authenticated TLS submission accepted")\n'
    subprocess.run(['docker', 'exec', '-i', 'sameframe-app-1', 'python3', '-'],
                   input=payload, text=True, check=True)
    delivered = None
    for _ in range(40):
        for path in Path('/var/lib/sameframe-mail/Maildir').glob('*/*'):
            if path.is_file() and marker.encode() in path.read_bytes():
                delivered = path
                break
        if delivered:
            break
        time.sleep(.25)
    if not delivered:
        raise RuntimeError('Local test message did not arrive')
    if not BytesParser().parsebytes(delivered.read_bytes()).get('Message-ID'):
        raise RuntimeError('Postfix did not add the required Message-ID header')
    records = json.loads((STATE / 'dns-records.json').read_text())
    public = next(record['content'] for record in records if '._domainkey' in record['name'])
    selector = json.loads((STATE / 'deployment.json').read_text())['selector']
    def lookup(name, **kwargs):
        if name.rstrip(b'.') != (selector + '._domainkey.sameframe.top').encode():
            raise RuntimeError('Unexpected DKIM key lookup')
        return public.encode()
    if not dkim.verify(delivered.read_bytes(), dnsfunc=lookup):
        raise RuntimeError('DKIM signature did not verify against the generated public key')
    results.append('Message from application network delivered locally; Message-ID added and DKIM signature verified')
    settings = AdminAPI().call('/api/v1/admin/smtp')['config']
    if settings['enabled'] or not settings['has_password']:
        raise RuntimeError('Expected prepared, encrypted and disabled application SMTP')
    results.append('Application SMTP prepared with encrypted credentials and stays disabled')
    result = {'external_mail_sent': False, 'passed': results}
    (STATE / 'local-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2))
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    validate()
