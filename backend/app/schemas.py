from typing import Literal
from pydantic import BaseModel, ConfigDict, Field


class Input(BaseModel):
    model_config = ConfigDict(extra="forbid")


class Register(Input):
    email: str = Field(min_length=5, max_length=180, pattern=r"^[^\s@]+@[^\s@]+\.[^\s@]+$")
    name: str = Field(min_length=1, max_length=40)
    password: str = Field(min_length=10, max_length=128)


class Login(Input):
    email: str = Field(max_length=180)
    password: str = Field(max_length=128)


class AgentCreate(Input):
    name: str = Field(min_length=1, max_length=40)
    subtitle: str = Field(default="自由生活的朋友", max_length=80)
    persona: str = Field(default="真诚、自然，有自己的兴趣和目标。", max_length=3000)
    color: str = Field(default="#5b8e78", pattern=r"^#[0-9a-fA-F]{6}$")


class AgentUpdate(Input):
    provider_id: str | None = None


class ProviderInput(Input):
    name: str = Field(min_length=1, max_length=80)
    protocol: Literal["compatible", "anthropic"] = "compatible"
    base_url: str = Field(max_length=1000)
    model: str = Field(min_length=1, max_length=120)
    api_key: str = Field(min_length=1, max_length=2000)


class ChatInput(Input):
    content: str = Field(min_length=1, max_length=8000)
    client_id: str = Field(min_length=8, max_length=100)


class MemoryInput(Input):
    content: str = Field(min_length=1, max_length=2000)
    category: Literal["preference", "promise", "experience"] = "preference"


class MigrateInput(Input):
    universe_id: str
    expected_version: int
    operation_id: str = Field(min_length=8, max_length=100)


class TaskInput(Input):
    agent_id: str
    goal: str = Field(min_length=2, max_length=8000)


class PostInput(Input):
    content: str = Field(min_length=1, max_length=3000)
    universe_id: str


class AdminStatus(Input):
    status: Literal["active", "suspended"]
    reason: str = Field(min_length=3, max_length=300)
