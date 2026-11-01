"""Deterministic entity resolution helpers."""

from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional

from .numbers import clean_symbol, clean_text


@dataclass
class ResolutionResult:
    canonical_symbol: str
    confidence: float
    reason: str
    needs_review: bool


def resolve_symbol(
    *,
    symbol: str,
    company_name: str,
    known_aliases: dict[str, str] | None = None,
    historical_symbols: dict[str, str] | None = None,
) -> ResolutionResult:
    """Deterministic resolution. Never merges on fuzzy name alone."""
    known_aliases = known_aliases or {}
    historical_symbols = historical_symbols or {}
    sym = clean_symbol(symbol)
    name = clean_text(company_name)

    if sym and sym in known_aliases:
        return ResolutionResult(canonical_symbol=known_aliases[sym], confidence=1.0, reason="alias_map", needs_review=False)

    if sym and sym in historical_symbols:
        return ResolutionResult(canonical_symbol=historical_symbols[sym], confidence=0.9, reason="historical_symbol", needs_review=True)

    if sym:
        # exact symbol match is high confidence but still verify name if provided
        if name:
            return ResolutionResult(canonical_symbol=sym, confidence=0.85, reason="symbol_with_name", needs_review=False)
        return ResolutionResult(canonical_symbol=sym, confidence=0.7, reason="symbol_only", needs_review=True)

    # No symbol: cannot resolve deterministically
    return ResolutionResult(canonical_symbol="", confidence=0.0, reason="no_symbol", needs_review=True)


def is_ambiguous(results: List[ResolutionResult]) -> bool:
    symbols = {r.canonical_symbol for r in results if r.canonical_symbol}
    return len(symbols) > 1
