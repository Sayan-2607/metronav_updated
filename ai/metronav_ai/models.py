"""Feature builders and model wrappers shared by training and serving."""
from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timedelta

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingRegressor

from .demand import base_occupancy, hour_of

HORIZONS_MIN = (15, 30, 60)

CROWD_FEATURES = [
    "station_idx", "weight", "hour_sin", "hour_cos", "t_hour_sin", "t_hour_cos", "weekday", "is_weekend",
    "holiday", "rain", "event", "occ_now", "lag15", "lag30", "horizon_min", "profile_target",
]
PROFILE_FEATURES = ["station_idx", "weight", "hour_sin", "hour_cos", "weekday", "is_weekend", "holiday"]
ETA_FEATURES = ["scheduled_min", "stops", "transfers", "mean_crowd", "hour_sin", "hour_cos", "is_weekend", "line_delayed"]


def _cyc(h):
    a = 2 * np.pi * np.asarray(h, dtype=float) / 24
    return np.sin(a), np.cos(a)


def crowd_training_frame(df: pd.DataFrame) -> pd.DataFrame:
    """Build supervised rows (t -> t+h) without crossing day boundaries."""
    df = df.sort_values(["station_id", "timestamp"]).copy()
    df["date"] = df["timestamp"].dt.date
    g = df.groupby(["station_id", "date"])["occupancy"]
    df["lag15"] = g.shift(1)
    df["lag30"] = g.shift(2)
    parts = []
    for h in HORIZONS_MIN:
        k = h // 15
        p = df.copy()
        p["target"] = g.shift(-k)
        p["t_hour"] = p["hour"] + h / 60
        p["horizon_min"] = h
        parts.append(p)
    out = pd.concat(parts, ignore_index=True).dropna(subset=["lag15", "lag30", "target"])
    out["occ_now"] = out["occupancy"]
    out["hour_sin"], out["hour_cos"] = _cyc(out["hour"])
    out["t_hour_sin"], out["t_hour_cos"] = _cyc(out["t_hour"])
    out["is_weekend"] = (out["weekday"] >= 5).astype(int)
    out["profile_target"] = base_occupancy(out["weight"], out["t_hour"], out["weekday"])
    return out


def profile_frame(df: pd.DataFrame) -> pd.DataFrame:
    out = df.copy()
    out["hour_sin"], out["hour_cos"] = _cyc(out["hour"])
    out["is_weekend"] = (out["weekday"] >= 5).astype(int)
    return out


def eta_frame(df: pd.DataFrame) -> pd.DataFrame:
    out = df.copy()
    out["hour_sin"], out["hour_cos"] = _cyc(out["hour"])
    out["is_weekend"] = (out["weekday"] >= 5).astype(int)
    return out


def hgb(**kw) -> HistGradientBoostingRegressor:
    params = dict(max_iter=300, learning_rate=0.08, max_leaf_nodes=31, random_state=0)
    params.update(kw)
    return HistGradientBoostingRegressor(**params)


@dataclass
class CrowdForecaster:
    model: HistGradientBoostingRegressor

    def row(self, station: dict, idx: int, ts: datetime, occ_now: float, lag15: float, lag30: float,
            horizon: int, rain: bool, event: bool, holiday: bool) -> dict:
        h = hour_of(ts)
        th = h + horizon / 60
        tts = ts + timedelta(minutes=horizon)
        hs, hc = _cyc(h)
        ts_, tc = _cyc(th)
        return {
            "station_idx": idx, "weight": station["weight"], "hour_sin": float(hs), "hour_cos": float(hc),
            "t_hour_sin": float(ts_), "t_hour_cos": float(tc), "weekday": ts.weekday(), "is_weekend": int(ts.weekday() >= 5),
            "holiday": int(holiday), "rain": int(rain), "event": int(event), "occ_now": occ_now, "lag15": lag15,
            "lag30": lag30, "horizon_min": horizon,
            "profile_target": float(base_occupancy(station["weight"], hour_of(tts), tts.weekday())),
        }

    def predict(self, rows: list[dict]) -> np.ndarray:
        X = pd.DataFrame(rows)[CROWD_FEATURES]
        return np.clip(self.model.predict(X), 0, 100)


@dataclass
class AnomalyDetector:
    model: HistGradientBoostingRegressor
    sigma: dict[int, float]  # residual std per station_idx
    threshold: float = 3.0
    default_sigma: float = field(default=5.0)

    def expected(self, rows: pd.DataFrame) -> np.ndarray:
        return np.clip(self.model.predict(rows[PROFILE_FEATURES]), 0, 100)

    def score(self, rows: pd.DataFrame, observed: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
        exp = self.expected(rows)
        sig = np.array([self.sigma.get(int(i), self.default_sigma) for i in rows["station_idx"]])
        return exp, (np.asarray(observed) - exp) / sig


@dataclass
class ETAModel:
    q10: HistGradientBoostingRegressor
    q50: HistGradientBoostingRegressor
    q90: HistGradientBoostingRegressor

    def predict(self, X: pd.DataFrame) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        X = X[ETA_FEATURES]
        p10, p50, p90 = self.q10.predict(X), self.q50.predict(X), self.q90.predict(X)
        # quantile models are fitted independently and can cross; enforce ordering
        lo = np.minimum.reduce([p10, p50, p90])
        hi = np.maximum.reduce([p10, p50, p90])
        mid = np.clip(p50, lo, hi)
        return lo, mid, hi
