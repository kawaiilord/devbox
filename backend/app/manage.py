import os

from sqlalchemy import select

from .db import SessionLocal, User, init_db
from .security import password_hash
from .world import create_home, ensure_public


def bootstrap():
    email = os.environ.get("UNIVERSE_ADMIN_EMAIL", "").strip().lower()
    password = os.environ.get("UNIVERSE_ADMIN_PASSWORD", "")
    if not email or len(password) < 10:
        raise SystemExit("请通过环境变量设置 UNIVERSE_ADMIN_EMAIL 和至少 10 位的 UNIVERSE_ADMIN_PASSWORD")
    init_db()
    with SessionLocal() as db:
        ensure_public(db)
        if db.scalar(select(User).where(User.email == email)):
            raise SystemExit("账户已存在，未修改密码或权限")
        user = User(email=email, name="宇宙管理员", role="admin", password_hash=password_hash(password))
        db.add(user)
        db.flush()
        create_home(db, user)
        db.commit()
    print("管理员和个人宇宙已创建。")


if __name__ == "__main__":
    bootstrap()
