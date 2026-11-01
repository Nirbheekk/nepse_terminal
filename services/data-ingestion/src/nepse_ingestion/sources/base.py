"""Source adapter base classes and discovery metadata."""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Any, Dict, List, Optional


class SourceStatus(str, Enum):
    APPROVED = "APPROVED"
    RESTRICTED = "RESTRICTED"
    PERMISSION_UNKNOWN = "PERMISSION_UNKNOWN"
    UNAVAILABLE = "UNAVAILABLE"
    REFERENCE_ONLY = "REFERENCE_ONLY"


@dataclass
class SourceDescriptor:
    slug: str
    organization: str
    url: str
    categories: List[str]
    file_format: str
    historical_coverage: str
    update_frequency: str
    public_access: str
    auth_required: str
    robots_terms: str
    license_status: str
    reliability: str
    parser_complexity: str
    identifiers: List[str]
    status: SourceStatus
    notes: str = ""
    live_fetch_enabled: bool = False


@dataclass
class RawDocument:
    source_slug: str
    document_type: str
    title: str
    source_url: str
    file_name: str
    mime_type: str
    content: bytes
    sha256: str
    fetched_at: str
    published_at: Optional[str] = None
    meta: Dict[str, Any] = field(default_factory=dict)


class SourceAdapter(ABC):
    descriptor: SourceDescriptor

    @abstractmethod
    def discover(self) -> List[Dict[str, Any]]:
        """Return discovery metadata (no network fetch)."""

    def fetch(self) -> RawDocument:
        raise NotImplementedError("Live fetch not implemented for this source")

    @abstractmethod
    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        """Parse raw content into normalized-ish records."""

    @abstractmethod
    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        ...
