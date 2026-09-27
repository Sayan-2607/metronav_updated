"""Train and evaluate all MetroNav models on synthetic data.

    python -m metronav_ai.train [--days 56] [--out models]

Writes models/bundle.joblib and models/metrics.json. Metrics are computed on a
held-out time range (last 14 days) of the SYNTHETIC dataset and compared with
naive baselines, so the numbers show relative skill, not real-world accuracy.
"""
from __future__ import annotations

import argparse
import json
import time
from datetime import datetime, timezone
from pathlib import Path

import joblib
import numpy as np
import pandas as pd
from sklearn.metrics import mean_absolute_error, mean_squared_error, r2_score

from . import __version__
from .demand import base_occupancy
from .models import (CROWD_FEATURES, ETA_FEATURES, PROFILE_FEATURES, AnomalyDetector, CrowdForecaster,
                     ETAModel, crowd_training_frame, eta_frame, hgb, profile_frame)
from .synthetic import inject_anomalies, journeys, station_occupancy


def regression_metrics(y, p, mape_floor=5.0) -> dict:
    y, p = np.asarray(y), np.asarray(p)
    m = y >= mape_floor  # MAPE is undefined/unstable near zero
    return {
        "mae": round(float(mean_absolute_error(y, p)), 3),
        "rmse": round(float(np.sqrt(mean_squared_error(y, p))), 3),
        "mape_pct": round(float(np.mean(np.abs((y[m] - p[m]) / y[m])) * 100), 2),
        "r2": round(float(r2_score(y, p)), 4),
        "n": int(len(y)),
    }


def train_crowd(df: pd.DataFrame, test_days: int) -> tuple[CrowdForecaster, dict]:
    frame = crowd_training_frame(df)
    cutoff = frame["timestamp"].max().normalize() - pd.Timedelta(days=test_days - 1)
    tr, te = frame[frame["timestamp"] < cutoff], frame[frame["timestamp"] >= cutoff]
    model = hgb(categorical_features=[0])  # station_idx is column 0
    model.fit(tr[CROWD_FEATURES], tr["target"])
    pred = np.clip(model.predict(te[CROWD_FEATURES]), 0, 100)
    out = {"model": "HistGradientBoostingRegressor", "features": CROWD_FEATURES,
           "train_rows": int(len(tr)), "test_rows": int(len(te)), "by_horizon": {}}
    for h in sorted(te["horizon_min"].unique()):
        m = te["horizon_min"] == h
        out["by_horizon"][f"{h}m"] = {
            "model": regression_metrics(te.loc[m, "target"], pred[m]),
            "baseline_persistence": regression_metrics(te.loc[m, "target"], te.loc[m, "occ_now"]),
            "baseline_profile": regression_metrics(te.loc[m, "target"], te.loc[m, "profile_target"]),
        }
    return CrowdForecaster(model), out


def train_anomaly(df: pd.DataFrame, test_days: int, threshold: float = 3.0) -> tuple[AnomalyDetector, dict]:
    cutoff = df["timestamp"].max().normalize() - pd.Timedelta(days=test_days - 1)
    tr = profile_frame(df[df["timestamp"] < cutoff])
    model = hgb(categorical_features=[0])
    model.fit(tr[PROFILE_FEATURES], tr["occupancy"])
    resid = tr["occupancy"] - model.predict(tr[PROFILE_FEATURES])
    sigma = resid.groupby(tr["station_idx"]).std().to_dict()
    det = AnomalyDetector(model=model, sigma={int(k): float(v) for k, v in sigma.items()}, threshold=threshold)

    te = profile_frame(inject_anomalies(df[df["timestamp"] >= cutoff].reset_index(drop=True)))
    _, z = det.score(te, te["occupancy"].to_numpy())
    pred = (z >= threshold).astype(int)
    y = te["is_anomaly"].to_numpy()
    tp = int(((pred == 1) & (y == 1)).sum()); fp = int(((pred == 1) & (y == 0)).sum())
    fn = int(((pred == 0) & (y == 1)).sum()); tn = int(((pred == 0) & (y == 0)).sum())
    prec = tp / (tp + fp) if tp + fp else 0.0
    rec = tp / (tp + fn) if tp + fn else 0.0
    f1 = 2 * prec * rec / (prec + rec) if prec + rec else 0.0
    return det, {
        "method": "profile regression (HGB) + per-station residual z-score, one-sided",
        "threshold_z": threshold, "features": PROFILE_FEATURES,
        "eval": {"precision": round(prec, 4), "recall": round(rec, 4), "f1": round(f1, 4),
                 "false_positive_rate": round(fp / (fp + tn), 5), "tp": tp, "fp": fp, "fn": fn, "tn": tn,
                 "injected": "0.5% of held-out points +20..+40 points"},
        "note": "Natural events/rain in the synthetic data are not labelled anomalies and cause some false positives.",
    }


def train_eta(n: int) -> tuple[ETAModel, dict]:
    df = eta_frame(journeys(n))
    k = int(len(df) * 0.8)
    tr, te = df.iloc[:k], df.iloc[k:]
    models = {}
    for q in (0.1, 0.5, 0.9):
        m = hgb(loss="quantile", quantile=q, max_iter=250)
        m.fit(tr[ETA_FEATURES], tr["actual_min"])
        models[q] = m
    eta = ETAModel(models[0.1], models[0.5], models[0.9])
    p10, p50, p90 = eta.predict(te)
    err = np.abs(te["actual_min"].to_numpy() - p50)
    base_err = np.abs(te["actual_min"].to_numpy() - te["scheduled_min"].to_numpy())
    cover = float(np.mean((te["actual_min"] >= p10) & (te["actual_min"] <= p90)))
    return eta, {
        "model": "HistGradientBoostingRegressor x3 (quantile loss 0.1/0.5/0.9)", "features": ETA_FEATURES,
        "train_rows": int(len(tr)), "test_rows": int(len(te)),
        "p50": {"mae_min": round(float(err.mean()), 3), "median_ae_min": round(float(np.median(err)), 3),
                "p95_ae_min": round(float(np.quantile(err, 0.95)), 3)},
        "baseline_schedule": {"mae_min": round(float(base_err.mean()), 3), "median_ae_min": round(float(np.median(base_err)), 3),
                              "p95_ae_min": round(float(np.quantile(base_err, 0.95)), 3)},
        "interval_p10_p90": {"coverage": round(cover, 4), "target": 0.8,
                             "mean_width_min": round(float(np.mean(p90 - p10)), 3)},
    }


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--days", type=int, default=56)
    ap.add_argument("--test-days", type=int, default=14)
    ap.add_argument("--eta-samples", type=int, default=60_000)
    ap.add_argument("--out", default=str(Path(__file__).resolve().parents[1] / "models"))
    a = ap.parse_args(argv)
    t0 = time.time()
    out = Path(a.out); out.mkdir(parents=True, exist_ok=True)

    df = station_occupancy(days=a.days)
    crowd, crowd_m = train_crowd(df, a.test_days)
    anomaly, anomaly_m = train_anomaly(df, a.test_days)
    eta, eta_m = train_eta(a.eta_samples)

    version = f"{__version__}+{datetime.now(timezone.utc):%Y%m%d%H%M}"
    joblib.dump({"crowd": crowd, "anomaly": anomaly, "eta": eta, "version": version}, out / "bundle.joblib", compress=3)
    metrics = {
        "version": version, "trained_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "data": {"kind": "synthetic", "stations": int(df["station_id"].nunique()), "days": a.days,
                 "rows": int(len(df)), "step_min": 15, "test_days": a.test_days},
        "crowd_forecast": crowd_m, "anomaly_detection": anomaly_m, "eta": eta_m,
        "training_seconds": round(time.time() - t0, 1),
        "disclaimer": "Metrics are measured on synthetic data generated by metronav_ai.synthetic; they are not evidence of real-world accuracy.",
    }
    (out / "metrics.json").write_text(json.dumps(metrics, indent=2))
    print(json.dumps({k: metrics[k] for k in ("version", "data", "training_seconds")}, indent=2))
    for h, v in crowd_m["by_horizon"].items():
        print(f"crowd {h}: model MAE {v['model']['mae']} | persistence {v['baseline_persistence']['mae']} | profile {v['baseline_profile']['mae']}")
    print("anomaly:", anomaly_m["eval"])
    print("eta:", eta_m["p50"], "baseline:", eta_m["baseline_schedule"], "interval:", eta_m["interval_p10_p90"])


if __name__ == "__main__":
    main()


__all__ = ["main", "base_occupancy"]
