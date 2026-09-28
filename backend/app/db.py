from datetime import datetime, timezone
from uuid import uuid4

from sqlalchemy import Column, Integer, String, Text, ForeignKey, JSON, UniqueConstraint, create_engine, event
from sqlalchemy.orm import DeclarativeBase, sessionmaker

from .config import DATABASE_URL


def uid():
    return str(uuid4())


def now():
    return datetime.now(timezone.utc).isoformat()


class Base(DeclarativeBase):
    pass


class User(Base):
    __tablename__ = "users"
    id = Column(String, primary_key=True, default=uid)
    email = Column(String, unique=True, nullable=False)
    name = Column(String, nullable=False)
    password_hash = Column(Text, nullable=False)
    role = Column(String, default="user", nullable=False)
    status = Column(String, default="active", nullable=False)
    created_at = Column(String, default=now, nullable=False)
    last_seen_at = Column(String, default=now, nullable=False)


class LoginSession(Base):
    __tablename__ = "sessions"
    id = Column(String, primary_key=True)
    user_id = Column(String, ForeignKey("users.id"), nullable=False)
    expires_at = Column(String, nullable=False)


class Universe(Base):
    __tablename__ = "universes"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=True)
    kind = Column(String, nullable=False)
    name = Column(String, nullable=False)
    description = Column(Text, default="")
    created_at = Column(String, default=now)


class Provider(Base):
    __tablename__ = "providers"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    name = Column(String, nullable=False)
    protocol = Column(String, nullable=False)
    base_url = Column(Text, nullable=False)
    model = Column(String, nullable=False)
    encrypted_key = Column(Text, nullable=False)
    created_at = Column(String, default=now)


class Agent(Base):
    __tablename__ = "agents"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    universe_id = Column(String, ForeignKey("universes.id"), nullable=False)
    provider_id = Column(String, ForeignKey("providers.id"), nullable=True)
    name = Column(String, nullable=False)
    subtitle = Column(String, nullable=False)
    persona = Column(Text, nullable=False)
    color = Column(String, default="#5b8e78")
    activity = Column(String, default="正在熟悉新生活")
    location = Column(String, default="银杏街")
    version = Column(Integer, default=1, nullable=False)
    created_at = Column(String, default=now)


class Message(Base):
    __tablename__ = "messages"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    agent_id = Column(String, ForeignKey("agents.id"), nullable=False)
    role = Column(String, nullable=False)
    content = Column(Text, nullable=False)
    state = Column(String, default="sent", nullable=False)
    client_id = Column(String, nullable=False)
    created_at = Column(String, default=now, nullable=False)
    __table_args__ = (UniqueConstraint("owner_id", "client_id"),)


class Memory(Base):
    __tablename__ = "memories"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    agent_id = Column(String, ForeignKey("agents.id"), nullable=False)
    universe_id = Column(String, ForeignKey("universes.id"), nullable=True)
    content = Column(Text, nullable=False)
    category = Column(String, default="preference")
    source = Column(String, default="user")
    source_id = Column(String, nullable=True)
    state = Column(String, default="active", nullable=False)
    created_at = Column(String, default=now, nullable=False)


class WorldEvent(Base):
    __tablename__ = "world_events"
    id = Column(String, primary_key=True, default=uid)
    universe_id = Column(String, ForeignKey("universes.id"), nullable=False)
    actor_ids = Column(JSON, nullable=False)
    title = Column(String, nullable=False)
    content = Column(Text, nullable=False)
    kind = Column(String, default="life")
    dedupe_key = Column(String, unique=True, nullable=False)
    created_at = Column(String, default=now, nullable=False)


class Post(Base):
    __tablename__ = "posts"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    agent_id = Column(String, ForeignKey("agents.id"), nullable=True)
    universe_id = Column(String, ForeignKey("universes.id"), nullable=False)
    event_id = Column(String, ForeignKey("world_events.id"), nullable=True, unique=True)
    content = Column(Text, nullable=False)
    created_at = Column(String, default=now, nullable=False)


class Task(Base):
    __tablename__ = "tasks"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    agent_id = Column(String, ForeignKey("agents.id"), nullable=False)
    goal = Column(Text, nullable=False)
    state = Column(String, default="queued", nullable=False)
    result = Column(Text, default="")
    steps = Column(JSON, default=list, nullable=False)
    lease_until = Column(String, nullable=True)
    attempts = Column(Integer, default=0)
    created_at = Column(String, default=now, nullable=False)
    updated_at = Column(String, default=now, nullable=False)


class Artifact(Base):
    __tablename__ = "artifacts"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    task_id = Column(String, ForeignKey("tasks.id"), nullable=False)
    name = Column(String, nullable=False)
    version = Column(Integer, default=1, nullable=False)
    media_type = Column(String, nullable=False)
    content = Column(Text, nullable=False)
    created_at = Column(String, default=now, nullable=False)
    __table_args__ = (UniqueConstraint("task_id", "name", "version"),)


class Operation(Base):
    __tablename__ = "operations"
    id = Column(String, primary_key=True, default=uid)
    owner_id = Column(String, ForeignKey("users.id"), nullable=False)
    key = Column(String, nullable=False)
    request_hash = Column(String, nullable=False)
    result = Column(JSON, nullable=False)
    created_at = Column(String, default=now)
    __table_args__ = (UniqueConstraint("owner_id", "key"),)


class Audit(Base):
    __tablename__ = "audits"
    id = Column(String, primary_key=True, default=uid)
    actor_id = Column(String, ForeignKey("users.id"), nullable=False)
    action = Column(String, nullable=False)
    target_id = Column(String, nullable=False)
    detail = Column(JSON, default=dict)
    created_at = Column(String, default=now)


engine = create_engine(
    DATABASE_URL,
    connect_args={"check_same_thread": False, "timeout": 20} if DATABASE_URL.startswith("sqlite") else {},
    pool_pre_ping=True,
)
SessionLocal = sessionmaker(bind=engine, expire_on_commit=False)


if DATABASE_URL.startswith("sqlite"):

    @event.listens_for(engine, "connect")
    def sqlite_config(connection, connection_record):
        connection.execute("PRAGMA foreign_keys=ON")
        connection.execute("PRAGMA journal_mode=WAL")


def init_db():
    Base.metadata.create_all(engine)


def session():
    with SessionLocal() as db:
        yield db


def record(obj, exclude=()):
    return {c.name: getattr(obj, c.name) for c in obj.__table__.columns if c.name not in exclude}
