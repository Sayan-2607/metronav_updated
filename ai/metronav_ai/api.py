"""FastAPI service exposing the trained models.

    uvicorn metronav_ai.api:app --host 0.0.0.0 --port 8000
If models/bundle.joblib is missing it is trained on start-up (~20-60 s).
"""
from __future__ import annotations

import json
import logging
import os
import time
from contextlib import asynccontextmanager
from datetime import datetime
from pathlib import Path

import joblib
import numpy as np
import pandas as pd
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from .demand import hour_of
from .models import HORIZONS_MIN, profile_frame
from .network import load_network

log = logging.getLogger("metronav_ai")
MODEL_DIR = Path(os.environ.get("MODEL_DIR", Path(__file__).resolve().parents[1] / "models"))
STATE: dict = {}


def load_bundle():
    path = MODEL_DIR / "bundle.joblib"
    if not path.exists():
        log.warning("no trained bundle at %s; training on synthetic data now", path)
        from .train import main as train_main
        train_main(["--out", str(MODEL_DIR)])
    STATE["bundle"] = joblib.load(path)
    STATE["metrics"] = json.loads((MODEL_DIR / "metrics.json").read_text())
    STATE["net"] = load_network()


@asynccontextmanager
async def lifespan(_app):
    load_bundle()
    yield


app = FastAPI(title="MetroNav AI", version="0.1.0", lifespan=lifespan)


def _station(sid: str) -> tuple[dict, int]:
    net = STATE["net"]
    if sid not in net["station_by_id"]:
        raise HTTPException(404, f"unknown station {sid}")
    return net["station_by_id"][sid], net["station_index"][sid]


class CrowdIn(BaseModel):
    station_id: str
    timestamp: datetime
    current_occupancy: float = Field(ge=0, le=100)
    occupancy_15m_ago: float = Field(ge=0, le=100)
    occupancy_30m_ago: float = Field(ge=0, le=100)
    rain: bool = False
    event_nearby: bool = False
    holiday: bool = False


@app.get("/health")
def health():
    return {"status": "ok", "model_version": STATE.get("bundle", {}).get("version")}


@app.get("/ml/models")
def models():
    return STATE["metrics"]


@app.post("/ml/crowd/predict")
def crowd(req: CrowdIn):
    st, idx = _station(req.station_id)
    f = STATE["bundle"]["crowd"]
    rows = [f.row(st, idx, req.timestamp, req.current_occupancy, req.occupancy_15m_ago, req.occupancy_30m_ago,
                  h, req.rain, req.event_nearby, req.holiday) for h in HORIZONS_MIN]
    p = f.predict(rows)
    return {"station_id": req.station_id, "forecast_15m": round(float(p[0]), 1), "forecast_30m": round(float(p[1]), 1),
            "forecast_60m": round(float(p[2]), 1), "model": f"crowd-hgb {STATE['bundle']['version']}"}


class AnomalyPoint(BaseModel):
    station_id: str
    timestamp: datetime
    observed: float = Field(ge=0, le=100)


class AnomalyIn(BaseModel):
    points: list[AnomalyPoint] = Field(max_length=500)


@app.post("/ml/anomaly/detect")
def anomaly(req: AnomalyIn):
    if not req.points:
        return {"results": []}
    det = STATE["bundle"]["anomaly"]
    rows = []
    for p in req.points:
        st, idx = _station(p.station_id)
        rows.append({"station_idx": idx, "weight": st["weight"], "hour": hour_of(p.timestamp),
                     "weekday": p.timestamp.weekday(), "holiday": 0})
    frame = profile_frame(pd.DataFrame(rows))
    obs = np.array([p.observed for p in req.points])
    exp, z = det.score(frame, obs)
    return {"threshold_z": det.threshold, "results": [
        {"station_id": p.station_id, "expected": round(float(e), 1), "observed": p.observed,
         "z_score": round(float(zz), 2), "is_anomaly": bool(zz >= det.threshold)}
        for p, e, zz in zip(req.points, exp, z)]}


class ETAIn(BaseModel):
    scheduled_min: float = Field(gt=0, le=240)
    stops: int = Field(ge=0, le=100)
    transfers: int = Field(ge=0, le=5)
    mean_crowd: float = Field(ge=0, le=100)
    timestamp: datetime
    line_delayed: bool = False


@app.post("/ml/eta/predict")
def eta(req: ETAIn):
    m = STATE["bundle"]["eta"]
    h = hour_of(req.timestamp)
    X = pd.DataFrame([{"scheduled_min": req.scheduled_min, "stops": req.stops, "transfers": req.transfers,
                       "mean_crowd": req.mean_crowd, "hour_sin": np.sin(2 * np.pi * h / 24),
                       "hour_cos": np.cos(2 * np.pi * h / 24), "is_weekend": int(req.timestamp.weekday() >= 5),
                       "line_delayed": int(req.line_delayed)}])
    p10, p50, p90 = m.predict(X)
    return {"p10_min": round(float(p10[0]), 1), "p50_min": round(float(p50[0]), 1), "p90_min": round(float(p90[0]), 1),
            "model": f"eta-quantile-hgb {STATE['bundle']['version']}"}


@app.middleware("http")
async def timing(request, call_next):
    t = time.perf_counter()
    resp = await call_next(request)
    resp.headers["X-Process-Time-Ms"] = f"{(time.perf_counter() - t) * 1000:.1f}"
    return resp
