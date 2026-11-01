"""Configuration for the NEPSE ingestion package."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class Settings:
    database_url: str = field(
        default_factory=lambda: os.getenv(
            "NEPSE_DATABASE_URL",
            "postgresql+psycopg://nepse:nepse@localhost:5432/nepse",
        )
    )
    test_database_url: str = field(
        default_factory=lambda: os.getenv(
            "NEPSE_TEST_DATABASE_URL",
            "sqlite:///./nepse_terminal_test.db",
        )
    )
    raw_storage_dir: Path = field(
        default_factory=lambda: Path(os.getenv("NEPSE_RAW_STORAGE", "./var/raw"))
    )
    user_agent: str = field(
        default_factory=lambda: os.getenv(
            "NEPSE_USER_AGENT", "nepse-ingestion/0.1 (+research; contact: local)"
        )
    )
    request_timeout: int = field(
        default_factory=lambda: int(os.getenv("NEPSE_REQUEST_TIMEOUT", "30"))
    )
    max_retries: int = field(
        default_factory=lambda: int(os.getenv("NEPSE_MAX_RETRIES", "3"))
    )
    rate_limit_seconds: float = field(
        default_factory=lambda: float(os.getenv("NEPSE_RATE_LIMIT_SECONDS", "1.0"))
    )
    live_fetch_enabled: bool = field(
        default_factory=lambda: os.getenv("NEPSE_LIVE_FETCH_ENABLED", "false").lower()
        == "true"
    )


settings = Settings()
