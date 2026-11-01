"""SQLAlchemy base and session helpers."""

from __future__ import annotations

from sqlalchemy import create_engine
from sqlalchemy.orm import DeclarativeBase, sessionmaker


class Base(DeclarativeBase):
    pass


def make_engine(url: str, **kwargs):
    return create_engine(url, future=True, **kwargs)


def make_session_factory(url: str):
    engine = make_engine(url)
    return sessionmaker(bind=engine, future=True, expire_on_commit=False), engine
