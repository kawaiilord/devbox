import json
import re

import httpx

from .security import decrypt, pin_public_endpoint, validate_public_url


class ProviderError(Exception):
    pass


async def complete(provider, messages, system, *, max_tokens=1600):
    try:
        base = validate_public_url(provider.base_url)
    except ValueError as exc:
        raise ProviderError(str(exc)) from exc
    key = decrypt(provider.encrypted_key)
    headers = {}
    if provider.protocol == "anthropic":
        path = base + ("/messages" if base.endswith("/v1") else "/v1/messages")
        headers = {"x-api-key": key, "anthropic-version": "2023-06-01"}
        payload = {"model": provider.model, "max_tokens": max_tokens, "system": system, "messages": messages}
    else:
        path = base + "/chat/completions"
        headers = {"Authorization": f"Bearer {key}"}
        payload = {
            "model": provider.model,
            "messages": [{"role": "system", "content": system}, *messages],
            "max_tokens": max_tokens,
        }
    try:
        pinned_url, hostname = pin_public_endpoint(path)
        headers["Host"] = hostname
        async with httpx.AsyncClient(timeout=55, follow_redirects=False, trust_env=False) as client:
            async with client.stream(
                "POST", pinned_url, headers=headers, json=payload, extensions={"sni_hostname": hostname}
            ) as response:
                if response.status_code != 200:
                    raise ProviderError(f"模型服务返回 HTTP {response.status_code}，请检查地址、模型、密钥和额度")
                chunks = bytearray()
                async for chunk in response.aiter_bytes():
                    chunks.extend(chunk)
                    if len(chunks) > 2000000:
                        raise ProviderError("模型响应超过大小限制")
        data = json.loads(chunks)
        if provider.protocol == "anthropic":
            value = "".join(item.get("text", "") for item in data.get("content", []) if item.get("type") == "text")
        else:
            value = data["choices"][0]["message"]["content"]
        if not isinstance(value, str) or not value.strip():
            raise ProviderError("模型未返回有效文字")
        return value[:60000]
    except (httpx.HTTPError, ValueError, KeyError, IndexError, TypeError) as exc:
        raise ProviderError("模型连接失败或响应格式不兼容") from exc


def parse_json(text):
    stripped = re.sub(r"^\`\`\`(?:json)?\s*|\s*\`\`\`$", "", text.strip())
    return json.loads(stripped)
