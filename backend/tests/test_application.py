import json
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from app import providers
from app.db import Agent, Provider, SessionLocal, Task, User
from app.main import app
from app.runtime import execute_task, run_capability, safe_name
from app.security import encrypt, password_valid, validate_public_url
from conftest import signup

HEADERS = {"X-Universe-Client": "web"}


def first_agent(client):
    return client.get("/api/agents").json()[0]


def provision_provider(agent_id):
    with SessionLocal() as db:
        agent = db.get(Agent, agent_id)
        provider = Provider(
            owner_id=agent.owner_id,
            name="Mock",
            protocol="compatible",
            base_url="https://example.com/v1",
            model="mock",
            encrypted_key=encrypt("test-key"),
        )
        db.add(provider)
        db.flush()
        agent.provider_id = provider.id
        db.commit()
        return provider.id


def test_registration_session_and_password(client, account):
    assert "password_hash" not in account
    assert client.get("/api/me").json()["id"] == account["id"]
    assert len(client.get("/api/agents").json()) == 2
    with SessionLocal() as db:
        stored = db.get(User, account["id"]).password_hash
        assert stored != "test-password-123"
        assert password_valid("test-password-123", stored)
    client.post("/api/auth/logout", headers=HEADERS)
    assert client.get("/api/me").status_code == 401


def test_csrf_and_admin_guard(client, account):
    assert client.post("/api/agents", json={"name": "x"}).status_code == 403
    assert client.get("/api/admin/users").status_code == 403


def test_cross_user_boundaries(client, account):
    agent = first_agent(client)
    memory = client.post(f"/api/agents/{agent['id']}/memories", headers=HEADERS, json={"content": "私人秘密"}).json()
    task = client.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "写报告"}).json()
    with TestClient(app) as other:
        signup(other, "other@example.com")
        assert other.get(f"/api/agents/{agent['id']}/messages").status_code == 404
        assert other.delete(f"/api/memories/{memory['id']}", headers=HEADERS).status_code == 404
        assert other.get(f"/api/tasks/{task['id']}").status_code == 404
        assert other.get(f"/api/universes/{agent['universe_id']}/events").status_code == 404
        assert other.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "写报告"}).status_code == 404


def test_world_event_has_shared_evidence(client, account):
    agents = client.get("/api/agents").json()
    world = agents[0]["universe_id"]
    response = client.post(f"/api/universes/{world}/advance", headers=HEADERS)
    event = response.json()["event"]
    assert set(event["actor_ids"]) == {a["id"] for a in agents}
    for agent in agents:
        memories = client.get(f"/api/agents/{agent['id']}/memories").json()
        assert memories[0]["source_id"] == event["id"]
        assert memories[0]["universe_id"] == world
    assert client.get("/api/posts").json()[0]["event_id"] == event["id"]


def test_migration_atomic_idempotent_and_versioned(client, account):
    agent = first_agent(client)
    payload = {"universe_id": "platform-universe", "expected_version": agent["version"], "operation_id": str(uuid4())}
    response = client.post(f"/api/agents/{agent['id']}/migrate", headers=HEADERS, json=payload)
    assert response.status_code == 200, response.text
    result = response.json()
    repeated = client.post(f"/api/agents/{agent['id']}/migrate", headers=HEADERS, json=payload)
    assert repeated.json() == result
    stale = {**payload, "universe_id": agent["universe_id"], "operation_id": str(uuid4())}
    assert client.post(f"/api/agents/{agent['id']}/migrate", headers=HEADERS, json=stale).status_code == 409
    assert client.get("/api/agents").json()[0]["universe_id"] == "platform-universe"


def test_migration_cannot_enter_foreign_home(client, account):
    agent = first_agent(client)
    with TestClient(app) as other:
        signup(other, "foreign@example.com")
        other_world = first_agent(other)["universe_id"]
    response = client.post(
        f"/api/agents/{agent['id']}/migrate",
        headers=HEADERS,
        json={"universe_id": other_world, "expected_version": 1, "operation_id": str(uuid4())},
    )
    assert response.status_code == 404
    assert first_agent(client)["universe_id"] == agent["universe_id"]


def test_chat_requires_real_provider(client, account):
    agent = first_agent(client)
    response = client.post(
        f"/api/agents/{agent['id']}/messages", headers=HEADERS, json={"content": "你好", "client_id": str(uuid4())}
    )
    assert response.status_code == 409
    assert client.get(f"/api/agents/{agent['id']}/messages").json() == []


def test_chat_memory_forgetting_and_idempotency(client, account, monkeypatch):
    agent = first_agent(client)
    provision_provider(agent["id"])
    memory = client.post(
        f"/api/agents/{agent['id']}/memories", headers=HEADERS, json={"content": "unique-secret-sunflower"}
    ).json()
    client.delete(f"/api/memories/{memory['id']}", headers=HEADERS)
    observed = []

    async def mock_complete(provider, messages, system, **kwargs):
        observed.append(system)
        return "这是来自测试模型的回复"

    monkeypatch.setattr(providers, "complete", mock_complete)
    payload = {"content": "你好", "client_id": str(uuid4())}
    r = client.post(f"/api/agents/{agent['id']}/messages", headers=HEADERS, json=payload)
    assert r.status_code == 200, r.text
    assert "unique-secret-sunflower" not in observed[0]
    client.post(f"/api/agents/{agent['id']}/messages", headers=HEADERS, json=payload)
    assert len(observed) == 1
    assert len(client.get(f"/api/agents/{agent['id']}/messages").json()) == 2


@pytest.mark.asyncio
async def test_generic_task_writes_real_artifact(client, account, monkeypatch):
    agent = first_agent(client)
    provision_provider(agent["id"])
    task = client.post(
        "/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "做一份带有计算说明的 CSV 表格"}
    ).json()
    actions = iter(
        [
            {"action": "write_file", "path": "report.csv", "content": "项目,金额\n设计,300\n开发,700\n合计,1000\n"},
            {"action": "read_file", "path": "report.csv"},
            {"action": "finish", "summary": "已生成 CSV 文件；未执行外部操作。"},
        ]
    )

    async def mock_complete(*args, **kwargs):
        return json.dumps(next(actions), ensure_ascii=False)

    monkeypatch.setattr(providers, "complete", mock_complete)
    await execute_task(task["id"])
    result = client.get(f"/api/tasks/{task['id']}").json()
    assert result["state"] == "succeeded"
    assert len(result["steps"]) == 3
    artifact = result["artifacts"][0]
    content = client.get(f"/api/artifacts/{artifact['id']}/download")
    assert "合计,1000" in content.text
    assert "attachment" in content.headers["Content-Disposition"]
    with TestClient(app) as other:
        signup(other, "artifact-other@example.com")
        assert other.get(f"/api/artifacts/{artifact['id']}").status_code == 404


@pytest.mark.asyncio
async def test_task_waits_for_configuration(client, account):
    agent = first_agent(client)
    task = client.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "生成一个网页"}).json()
    await execute_task(task["id"])
    assert client.get(f"/api/tasks/{task['id']}").json()["state"] == "waiting_configuration"


@pytest.mark.asyncio
async def test_cancel_during_model_call_prevents_artifact(client, account, monkeypatch):
    agent = first_agent(client)
    provision_provider(agent["id"])
    task = client.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "生成网页"}).json()

    async def mock_complete(*args, **kwargs):
        with SessionLocal() as db:
            db.get(Task, task["id"]).state = "cancelled"
            db.commit()
        return json.dumps({"action": "write_file", "path": "late.html", "content": "late"})

    monkeypatch.setattr(providers, "complete", mock_complete)
    await execute_task(task["id"])
    result = client.get(f"/api/tasks/{task['id']}").json()
    assert result["state"] == "cancelled"
    assert result["artifacts"] == []


@pytest.mark.parametrize("path", ["../secret", "/etc/passwd", "a/../../x", "C:\\secret", "bad\nname"])
def test_workspace_paths_rejected(path):
    with pytest.raises(ValueError):
        safe_name(path)


@pytest.mark.parametrize(
    "url", ["http://example.com/v1", "https://127.0.0.1/v1", "https://169.254.169.254/", "https://user:secret@example.com/v1"]
)
def test_ssrf_rejected(url):
    with pytest.raises(ValueError):
        validate_public_url(url)


def test_finish_requires_actual_output(client, account):
    agent = first_agent(client)
    with SessionLocal() as db:
        task = Task(owner_id=account["id"], agent_id=agent["id"], goal="example")
        db.add(task)
        db.flush()
        with pytest.raises(ValueError):
            run_capability(db, task, {"action": "finish", "summary": "假装完成"})


def test_admin_status_and_audit(client, administrator):
    with TestClient(app) as other:
        target = signup(other, "normal@example.com")
        info = client.get(f"/api/admin/users/{target['id']}").json()
        assert info["presence"] == "online"
        assert "password_hash" not in info
        assert "persona" not in info["agents"][0]
        r = client.patch(
            f"/api/admin/users/{target['id']}/status", headers=HEADERS, json={"status": "suspended", "reason": "integration test"}
        )
        assert r.status_code == 200
        assert other.get("/api/me").status_code == 403
    events = client.get("/api/admin/audits").json()
    assert events[0]["action"] == "user.status"
    assert events[0]["detail"]["reason"] == "integration test"
    assert (
        client.patch(
            f"/api/admin/users/{administrator['id']}/status", headers=HEADERS, json={"status": "suspended", "reason": "self"}
        ).status_code
        == 409
    )
