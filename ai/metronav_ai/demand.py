"""Synthetic demand model. Mirrors services/core/internal/network/demand.go
formula-for-formula; keep both in sync."""
from __future__ import annotations

import math

import numpy as np


def time_of_day_factor(hour: float | np.ndarray) -> np.ndarray:
    h = np.asarray(hour, dtype=float)

    def g(mu, sigma, amp):
        return amp * np.exp(-((h - mu) ** 2) / (2 * sigma**2))

    v = 0.15 + g(9.5, 1.2, 0.85) + g(18.5, 1.5, 0.80) + g(13.5, 2.0, 0.25)
    v = np.minimum(v, 1.0)
    return np.where((h < 6.0) | (h > 22.5), 0.02, v)


def weekday_factor(weekday: int | np.ndarray) -> np.ndarray:
    """weekday: Python convention, Monday=0 ... Sunday=6."""
    wd = np.asarray(weekday)
    return np.where(wd == 5, 0.75, np.where(wd == 6, 0.55, 1.0))


def base_occupancy(weight, hour, weekday) -> np.ndarray:
    v = 100 * (0.08 + 0.95 * time_of_day_factor(hour) * weekday_factor(weekday) * np.asarray(weight))
    return np.clip(v, 0, 100)


def fnv32(s: str) -> int:
    h = 0x811C9DC5
    for b in s.encode():
        h ^= b
        h = (h * 0x01000193) & 0xFFFFFFFF
    return h


def exit_coach(station_id: str, coaches: int) -> int:
    return fnv32(station_id) % coaches


def coach_bias(station_id: str, coaches: int) -> list[float]:
    e = exit_coach(station_id, coaches)
    denom = max(coaches - 1, 1)
    raw = [1 + 0.35 * (1 - abs(c - e) / denom) for c in range(coaches)]
    m = sum(raw) / coaches
    return [r / m for r in raw]


def hour_of(ts) -> float:
    return ts.hour + ts.minute / 60 + ts.second / 3600


__all__ = ["time_of_day_factor", "weekday_factor", "base_occupancy", "fnv32", "exit_coach", "coach_bias", "hour_of", "math"]
