# ruff: noqa: E402
# Choose an isolated database before importing application configuration.
import os
import tempfile

os.environ["UNIVERSE_DATA"] = tempfile.mkdtemp(prefix="universe-test-")
test_database = os.environ.get("TEST_DATABASE_URL")
if test_database and "_test" not in test_database:
    raise RuntimeError("TEST_DATABASE_URL must point to a dedicated _test database")
os.environ["DATABASE_URL"] = test_database or "sqlite:///" + os.environ["UNIVERSE_DATA"] + "/test.db"
os.environ["ENABLE_WORKER"] = "false"
os.environ["UNIVERSE_SECRET"] = "test-only-secret-not-for-deployment"

import pytest
from fastapi.testclient import TestClient
from app.db import Base, SessionLocal, User, engine
from app.main import app
from app.security import AUTH_BUCKETS


@pytest.fixture(autouse=True)
def fresh_db():
    AUTH_BUCKETS.clear()
    Base.metadata.drop_all(engine)
    Base.metadata.create_all(engine)
    yield


@pytest.fixture
def client():
    with TestClient(app) as test:
        yield test


def signup(client, email="user@example.com", name="测试用户"):
    response = client.post(
        "/api/auth/register",
        headers={"X-Universe-Client": "web"},
        json={"email": email, "name": name, "password": "test-password-123"},
    )
    assert response.status_code == 201, response.text
    return response.json()


@pytest.fixture
def account(client):
    return signup(client)


@pytest.fixture
def administrator(client):
    result = signup(client, "admin@example.com")
    with SessionLocal() as db:
        db.get(User, result["id"]).role = "admin"
        db.commit()
    return result
