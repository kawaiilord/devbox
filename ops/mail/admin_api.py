"""Local authenticated configuration client. Never print credentials or tokens."""
import json
from pathlib import Path
import urllib.request


class AdminAPI:
    def __init__(self, root=Path('/opt/sameframe')):
        account = json.loads((root / 'admin-access.json').read_text())
        self.headers = {'Content-Type': 'application/json',
                        'X-Device-ID': account['device'],
                        'X-Device-Name': 'Admin Web', 'X-Device-Platform': 'web-admin'}
        session = self.call('/api/v1/auth/login', {
            'email': account['email'], 'password': account['password']})
        self.headers['Authorization'] = 'Bearer ' + session['access_token']

    def call(self, path, data=None, method=None):
        request = urllib.request.Request('http://127.0.0.1:18082' + path,
            data=json.dumps(data).encode() if data is not None else None,
            headers=self.headers, method=method)
        with urllib.request.urlopen(request, timeout=25) as response:
            body = json.load(response)
        if body.get('code') != 0:
            raise RuntimeError(body.get('msg', 'Administration request failed'))
        return body.get('data', {})
