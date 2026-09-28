import json
import socket

import httpx
import pytest

from app import providers
from app.db import Artifact, Provider, SessionLocal, Task
from app.runtime import execute_task, run_capability
from app.security import encrypt, pin_public_endpoint
from test_application import HEADERS, first_agent, provision_provider


@pytest.mark.parametrize("protocol", ["compatible", "anthropic"])
@pytest.mark.asyncio
async def test_provider_contract_pins_ip_and_tls_hostname(monkeypatch, protocol):
    monkeypatch.setattr(
        socket, "getaddrinfo", lambda *a, **k: [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("93.184.215.14", 443))]
    )
    observed = []

    async def handler(request):
        observed.append(request)
        assert request.url.host == "93.184.215.14"
        assert request.headers["Host"] == "model.example.com"
        assert request.extensions["sni_hostname"] == "model.example.com"
        payload = json.loads(request.content)
        assert payload["model"] == "contract-test"
        if protocol == "anthropic":
            assert request.url.path == "/v1/messages"
            assert request.headers["x-api-key"] == "test-secret"
            assert payload["system"] == "system text"
            return httpx.Response(200, json={"content": [{"type": "text", "text": "ok"}]})
        assert request.url.path == "/v1/chat/completions"
        assert request.headers["Authorization"] == "Bearer test-secret"
        assert payload["messages"][0] == {"role": "system", "content": "system text"}
        return httpx.Response(200, json={"choices": [{"message": {"content": "ok"}}]})

    original = httpx.AsyncClient
    monkeypatch.setattr(
        providers.httpx, "AsyncClient", lambda **kwargs: original(transport=httpx.MockTransport(handler), **kwargs)
    )
    config = Provider(
        protocol=protocol, base_url="https://model.example.com/v1", model="contract-test", encrypted_key=encrypt("test-secret")
    )
    result = await providers.complete(config, [{"role": "user", "content": "hello"}], "system text")
    assert result == "ok"
    assert len(observed) == 1


def test_dns_rebinding_rejected(monkeypatch):
    addresses = iter(["93.184.215.14", "127.0.0.1"])
    monkeypatch.setattr(
        socket, "getaddrinfo", lambda *a, **k: [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (next(addresses), 443))]
    )
    with pytest.raises(ValueError):
        pin_public_endpoint("https://model.example.com/v1/messages")


@pytest.mark.asyncio
async def test_unavailable_capability_is_not_success(client, account, monkeypatch):
    agent = first_agent(client)
    provision_provider(agent["id"])
    task = client.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": "编译并部署一个网站"}).json()

    async def mock_complete(*args, **kwargs):
        return json.dumps({"action": "blocked", "reason": "当前没有编译与部署执行器"})

    monkeypatch.setattr(providers, "complete", mock_complete)
    await execute_task(task["id"])
    result = client.get(f"/api/tasks/{task['id']}").json()
    assert result["state"] == "blocked"
    assert result["artifacts"] == []


def test_artifact_revisions_are_preserved(client, account):
    agent = first_agent(client)
    with SessionLocal() as db:
        task = Task(owner_id=account["id"], agent_id=agent["id"], goal="versions")
        db.add(task)
        db.flush()
        first = run_capability(db, task, {"action": "write_file", "path": "report.md", "content": "version one"})
        second = run_capability(db, task, {"action": "write_file", "path": "report.md", "content": "version two"})
        assert first["artifact_id"] != second["artifact_id"]
        assert second["version"] == 2
        assert db.get(Artifact, first["artifact_id"]).content == "version one"
        assert run_capability(db, task, {"action": "read_file", "path": "report.md"})["content"] == "version two"


def test_python_file_syntax_checked_without_execution(client, account):
    agent = first_agent(client)
    with SessionLocal() as db:
        task = Task(owner_id=account["id"], agent_id=agent["id"], goal="python")
        db.add(task)
        db.flush()
        with pytest.raises(ValueError, match="语法"):
            run_capability(db, task, {"action": "write_file", "path": "broken.py", "content": "def broken("})


def test_login_throttled(client):
    for _ in range(12):
        assert (
            client.post(
                "/api/auth/login", headers=HEADERS, json={"email": "not-existing@example.com", "password": "invalid"}
            ).status_code
            == 401
        )
    assert (
        client.post(
            "/api/auth/login", headers=HEADERS, json={"email": "not-existing@example.com", "password": "invalid"}
        ).status_code
        == 429
    )


def test_private_task_contents_not_in_admin_status(client, administrator):
    agent = first_agent(client)
    with SessionLocal() as db:
        db.add(
            Task(
                owner_id=administrator["id"],
                agent_id=agent["id"],
                goal="secret-goal",
                result="secret-result",
                steps=[{"secret": "private-step"}],
            )
        )
        db.commit()
    result = client.get(f"/api/admin/users/{administrator['id']}").json()
    assert result["tasks"]
    assert "goal" not in result["tasks"][0]
    assert "result" not in result["tasks"][0]
    assert "steps" not in result["tasks"][0]


def test_concurrent_task_creation_respects_limit(client, account):
    from concurrent.futures import ThreadPoolExecutor

    agent = first_agent(client)

    def create(index):
        return client.post("/api/tasks", headers=HEADERS, json={"agent_id": agent["id"], "goal": f"报告 {index}"}).status_code

    with ThreadPoolExecutor(max_workers=8) as pool:
        statuses = list(pool.map(create, range(8)))
    assert statuses.count(201) == 3
    assert statuses.count(429) == 5
    assert len(client.get("/api/tasks").json()) == 3


def test_binary_file_cannot_be_faked_with_text(client, account):
    agent = first_agent(client)
    with SessionLocal() as db:
        task = Task(owner_id=account["id"], agent_id=agent["id"], goal="PDF")
        db.add(task)
        db.flush()
        with pytest.raises(ValueError, match="PDF"):
            run_capability(db, task, {"action": "write_file", "path": "report.pdf", "content": "not actually a PDF"})
