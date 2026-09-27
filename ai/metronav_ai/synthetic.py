"""Synthetic datasets. Every row produced here is simulated.

Station occupancy (percent of platform capacity) at 15-minute resolution:
    occ = base(weight, hour, weekday) * holiday * rain + event_boost + AR(1) noise
Assumptions (documented, not fitted to real data):
    - holidays scale demand by 0.6
    - rain days (p=0.25) scale demand by 1.08 (modal shift from road)
    - a nearby event (p=0.02 per station-day) adds up to +25 points over ~3 h
    - AR(1) noise: phi=0.8, innovation sigma=3
"""
from __future__ import annotations

import numpy as np
import pandas as pd

from .demand import base_occupancy
from .network import load_network

STEP_MIN = 15
SERVICE_START, SERVICE_END = 6.0, 23.0


def station_occupancy(days: int = 56, start: str = "2026-01-05", seed: int = 7) -> pd.DataFrame:
    rng = np.random.default_rng(seed)
    net = load_network()
    dates = pd.date_range(start, periods=days, freq="D")
    holidays = set(rng.choice(days, size=max(1, days // 20), replace=False).tolist())
    rain = rng.random(days) < 0.25
    steps = int((SERVICE_END - SERVICE_START) * 60 / STEP_MIN)
    hours = SERVICE_START + np.arange(steps) * STEP_MIN / 60

    frames = []
    for s in net["stations"]:
        sid, w = s["id"], s["weight"]
        idx = net["station_index"][sid]
        for d, day in enumerate(dates):
            wd = day.weekday()
            base = base_occupancy(w, hours, wd)
            mult = (0.6 if d in holidays else 1.0) * (1.08 if rain[d] else 1.0)
            occ = base * mult
            event = np.zeros(steps)
            if rng.random() < 0.02:
                centre = rng.uniform(16, 21)
                event = 25 * np.exp(-((hours - centre) ** 2) / (2 * 0.8**2))
            noise = np.zeros(steps)
            for k in range(1, steps):
                noise[k] = 0.8 * noise[k - 1] + rng.normal(0, 3)
            occ = np.clip(occ + event + noise, 0, 100)
            frames.append(pd.DataFrame({
                "station_id": sid, "station_idx": idx, "weight": w,
                "timestamp": day + pd.to_timedelta(hours, unit="h"),
                "hour": hours, "weekday": wd, "holiday": int(d in holidays),
                "rain": int(rain[d]), "event": (event > 5).astype(int), "occupancy": occ,
            }))
    df = pd.concat(frames, ignore_index=True)
    return df.sort_values(["station_id", "timestamp"]).reset_index(drop=True)


def inject_anomalies(df: pd.DataFrame, frac: float = 0.005, seed: int = 11) -> pd.DataFrame:
    """Copy of df with labelled positive spikes (+20..+40 points)."""
    rng = np.random.default_rng(seed)
    out = df.copy()
    out["is_anomaly"] = 0
    n = int(len(out) * frac)
    idx = rng.choice(len(out), size=n, replace=False)
    out.loc[idx, "occupancy"] = np.clip(out.loc[idx, "occupancy"] + rng.uniform(20, 40, n), 0, 100)
    # only count as anomalies the spikes that actually moved the value (not clipped away)
    out.loc[idx, "is_anomaly"] = (out.loc[idx, "occupancy"] - df.loc[idx, "occupancy"] > 10).astype(int)
    return out


def journeys(n: int = 60_000, seed: int = 5) -> pd.DataFrame:
    """Synthetic journeys for ETA modelling. Ground truth:
    actual = sched*(1 + 0.06*crowd/100 + 0.04*peak) + transfer_delays + disruption + noise
    """
    rng = np.random.default_rng(seed)
    sched = rng.uniform(5, 60, n)
    stops = np.maximum(1, np.round(sched / 2.6 + rng.normal(0, 1, n))).astype(int)
    transfers = rng.choice([0, 1, 2], size=n, p=[0.6, 0.33, 0.07])
    crowd = np.clip(rng.normal(45, 22, n), 0, 100)
    hour = rng.uniform(6, 23, n)
    weekday = rng.integers(0, 7, n)
    delayed = (rng.random(n) < 0.05).astype(int)
    peak = np.exp(-((hour - 9.5) ** 2) / 2.88) + np.exp(-((hour - 18.5) ** 2) / 4.5)
    transfer_delay = np.array([rng.gamma(2, 0.6, t).sum() if t else 0.0 for t in transfers])
    disruption = delayed * rng.uniform(3, 12, n)
    noise = rng.normal(0, 0.03 * sched + 0.5)
    actual = sched * (1 + 0.06 * crowd / 100 + 0.04 * peak) + transfer_delay + disruption + noise
    return pd.DataFrame({
        "scheduled_min": sched, "stops": stops, "transfers": transfers, "mean_crowd": crowd,
        "hour": hour, "weekday": weekday, "line_delayed": delayed, "actual_min": np.maximum(actual, 1),
    })
