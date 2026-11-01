"""HTTP fetcher with bounded retries, timeouts and rate limiting."""

from __future__ import annotations

import hashlib
import time
from dataclasses import dataclass
from typing import Optional

import requests

from ..config import settings


@dataclass
class FetchResult:
    url: str
    status_code: int
    content: bytes
    sha256: str
    content_type: str
    etag: Optional[str] = None
    last_modified: Optional[str] = None


class RateLimiter:
    def __init__(self, min_interval: float):
        self.min_interval = min_interval
        self._last = 0.0

    def wait(self):
        now = time.monotonic()
        elapsed = now - self._last
        if elapsed < self.min_interval:
            time.sleep(self.min_interval - elapsed)
        self._last = time.monotonic()


class HttpFetcher:
    def __init__(self, rate_limiter: RateLimiter | None = None):
        self.rate_limiter = rate_limiter or RateLimiter(settings.rate_limit_seconds)
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": settings.user_agent})

    def fetch(self, url: str, *, conditional_headers: dict | None = None) -> FetchResult:
        headers = dict(conditional_headers or {})
        last_exc: Exception | None = None
        for attempt in range(settings.max_retries):
            self.rate_limiter.wait()
            try:
                resp = self.session.get(url, headers=headers, timeout=settings.request_timeout)
                if resp.status_code == 304:
                    return FetchResult(
                        url=url,
                        status_code=304,
                        content=b"",
                        sha256="",
                        content_type=resp.headers.get("Content-Type", ""),
                        etag=resp.headers.get("ETag"),
                        last_modified=resp.headers.get("Last-Modified"),
                    )
                resp.raise_for_status()
                content = resp.content
                return FetchResult(
                    url=url,
                    status_code=resp.status_code,
                    content=content,
                    sha256=hashlib.sha256(content).hexdigest(),
                    content_type=resp.headers.get("Content-Type", ""),
                    etag=resp.headers.get("ETag"),
                    last_modified=resp.headers.get("Last-Modified"),
                )
            except requests.RequestException as exc:
                last_exc = exc
                time.sleep(2 ** attempt)
        assert last_exc is not None
        raise last_exc
