import hashlib
from datetime import datetime, timezone

from sqlalchemy import select, update
from sqlalchemy.exc import IntegrityError

from .db import Agent, Memory, Post, Universe, User, WorldEvent, uid

PUBLIC_ID = "platform-universe"


def ensure_public(db):
    if not db.get(Universe, PUBLIC_ID):
        db.add(Universe(id=PUBLIC_ID, kind="public", name="星港宇宙", description="在同一片天空下，遇见新的朋友。"))
        db.commit()


def create_home(db, user):
    home = Universe(owner_id=user.id, kind="private", name="我的小宇宙", description="属于你和朋友们的生活，有自己的节奏。")
    db.add(home)
    db.flush()
    presets = [
        (
            "林野",
            "自由摄影师 · 喜欢海边和旧唱片",
            "你叫林野，是虚拟世界中的自由摄影师。说话自然、简短，喜欢发现生活中的细节。你有自己的摄影计划，也乐于帮朋友完成实际任务。",
            "#637d6e",
        ),
        (
            "许知夏",
            "建筑系研究生 · 正在准备毕业展",
            "你叫许知夏，是虚拟世界中的建筑系研究生。你细心、有幽默感，喜欢设计与城市散步。虚拟经历以世界提供的事件为准。",
            "#b07b65",
        ),
    ]
    for name, subtitle, persona, color in presets:
        db.add(Agent(owner_id=user.id, universe_id=home.id, name=name, subtitle=subtitle, persona=persona, color=color))
    db.flush()
    return home


def advance_world(db, universe, *, manual=False):
    agents = list(
        db.scalars(
            select(Agent)
            .join(User, User.id == Agent.owner_id)
            .where(Agent.universe_id == universe.id, User.status == "active")
            .order_by(Agent.created_at)
        )
    )
    if not agents:
        return None
    stamp = datetime.now(timezone.utc)
    bucket = int(stamp.timestamp()) // 900
    dedupe = f"{universe.id}:{uid() if manual else bucket}"
    if db.scalar(select(WorldEvent).where(WorldEvent.dedupe_key == dedupe)):
        return None
    scenes = [
        ("银杏书店", "在书店寻找新灵感", "一场不赶时间的阅读"),
        ("海风咖啡馆", "在咖啡馆整理今天的计划", "把今天过得慢一点"),
        ("河岸步道", "沿着河岸散步", "在河岸遇见了一点灵感"),
        ("创意工坊", "在工坊推进自己的作品", "一起动手做点新东西"),
    ]
    index = int(hashlib.sha256(dedupe.encode()).hexdigest()[:8], 16) % len(scenes)
    location, activity, title = scenes[index]
    offset = bucket % len(agents)
    participants = (agents[offset:] + agents[:offset])[:2]
    for agent in participants:
        result = db.execute(
            update(Agent)
            .where(Agent.id == agent.id, Agent.version == agent.version, Agent.universe_id == universe.id)
            .values(location=location, activity=activity)
        )
        if result.rowcount != 1:
            db.rollback()
            return None
    names = "、".join(a.name for a in participants)
    content = f"{names}来到{location}，{activity}。这是今天的一次共同生活事件。"
    event = WorldEvent(
        universe_id=universe.id, actor_ids=[a.id for a in participants], title=title, content=content, dedupe_key=dedupe
    )
    db.add(event)
    try:
        db.flush()
        for agent in participants:
            db.add(
                Memory(
                    owner_id=agent.owner_id,
                    agent_id=agent.id,
                    universe_id=universe.id,
                    content=content,
                    category="experience",
                    source="world_event",
                    source_id=event.id,
                )
            )
        db.add(
            Post(
                owner_id=participants[0].owner_id,
                agent_id=participants[0].id,
                universe_id=universe.id,
                event_id=event.id,
                content=content,
            )
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        return None
    return event
