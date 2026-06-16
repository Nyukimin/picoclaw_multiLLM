from __future__ import annotations

from datetime import date, datetime, timezone


def utcnow_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def parse_date(value: str) -> date:
    return datetime.strptime(value, "%Y-%m-%d").date()


def friday_of_week(day: date) -> date:
    return date.fromordinal(day.toordinal() + (4 - day.weekday()))


def iso(day: date) -> str:
    return day.isoformat()

