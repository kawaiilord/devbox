import base64
import hashlib
import hmac
import ipaddress
import os
import secrets
import socket
import time
from collections import deque
from datetime import datetime, timedelta, timezone
from urllib.parse import urlparse

from cryptography.fernet import Fernet
from fastapi import Depends, HTTPException, Request
from sqlalchemy.orm import Session

from .config import DATA
from .db import LoginSession, User, now, session

AUTH_BUCKETS = {}


def allow_auth(key):
    stamp = time.monotonic()
    bucket = AUTH_BUCKETS.setdefault(key, deque())
    while bucket and bucket[0] < stamp - 60:
        bucket.popleft()
    if len(AUTH_BUCKETS) > 10000:
        for old_key in list(AUTH_BUCKETS):
            if not AUTH_BUCKETS[old_key] or AUTH_BUCKETS[old_key][-1] < stamp - 60:
                del AUTH_BUCKETS[old_key]
        if len(AUTH_BUCKETS) > 10000:
            return False
    if len(bucket) >= 12:
        return False
    bucket.append(stamp)
    return True


def password_hash(password: str) -> str:
    salt = secrets.token_bytes(16)
    digest = hashlib.scrypt(password.encode(), salt=salt, n=16384, r=8, p=1)
    return salt.hex() + ":" + digest.hex()


def password_valid(password: str, stored: str) -> bool:
    salt, expected = stored.split(":")
    actual = hashlib.scrypt(password.encode(), salt=bytes.fromhex(salt), n=16384, r=8, p=1)
    return hmac.compare_digest(actual.hex(), expected)


def cipher():
    secret = os.environ.get("UNIVERSE_SECRET")
    if not secret:
        path = DATA / ".secret"
        try:
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except FileExistsError:
            pass
        else:
            with os.fdopen(fd, "w") as f:
                f.write(secrets.token_urlsafe(48))
        secret = path.read_text().strip()
    return Fernet(base64.urlsafe_b64encode(hashlib.sha256(secret.encode()).digest()))


def encrypt(value):
    return cipher().encrypt(value.encode()).decode()


def decrypt(value):
    return cipher().decrypt(value.encode()).decode()


def create_session(db, user):
    token = secrets.token_urlsafe(48)
    db.add(
        LoginSession(
            id=hashlib.sha256(token.encode()).hexdigest(),
            user_id=user.id,
            expires_at=(datetime.now(timezone.utc) + timedelta(days=7)).isoformat(),
        )
    )
    db.commit()
    return token


def current_user(request: Request, db: Session = Depends(session)):
    token = request.cookies.get("universe_session", "")
    auth = db.get(LoginSession, hashlib.sha256(token.encode()).hexdigest())
    if not auth or auth.expires_at < now():
        raise HTTPException(401, "请先登录")
    user = db.get(User, auth.user_id)
    if not user or user.status != "active":
        raise HTTPException(403, "账户暂不可用")
    user.last_seen_at = now()
    db.commit()
    return user


def admin_user(user: User = Depends(current_user)):
    if user.role != "admin":
        raise HTTPException(403, "需要管理员权限")
    return user


def owned(db, cls, obj_id, user):
    obj = db.get(cls, obj_id)
    if not obj or obj.owner_id != user.id:
        raise HTTPException(404, "记录不存在")
    return obj


def validate_public_url(value):
    parsed = urlparse(value)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.fragment or parsed.query:
        raise ValueError("接口地址必须是无凭证的 HTTPS 公网地址")
    if parsed.port not in (None, 443):
        raise ValueError("接口必须使用 HTTPS 标准端口")
    try:
        addresses = socket.getaddrinfo(parsed.hostname, 443, type=socket.SOCK_STREAM)
    except socket.gaierror as exc:
        raise ValueError("无法解析接口域名") from exc
    if not addresses or any(not ipaddress.ip_address(item[4][0]).is_global for item in addresses):
        raise ValueError("不允许访问内网、回环或云实例元数据地址")
    return value.rstrip("/")


def pin_public_endpoint(value):
    import httpx

    validate_public_url(value)
    parsed = urlparse(value)
    addresses = socket.getaddrinfo(parsed.hostname, 443, type=socket.SOCK_STREAM)
    if not addresses or any(not ipaddress.ip_address(item[4][0]).is_global for item in addresses):
        raise ValueError("接口地址解析到非公网地址")
    return httpx.URL(value).copy_with(host=addresses[0][4][0]), parsed.hostname
