import numpy as np
import pytest
from fastapi.testclient import TestClient

from metronav_ai.demand import base_occupancy, coach_bias, exit_coach, fnv32, time_of_day_factor
from metronav_ai.synthetic import inject_anomalies, station_occupancy


def test_fnv32_known_vectors():
    # Standard FNV-1a 32-bit test vectors
    assert fnv32("") == 0x811C9DC5
    assert fnv32("a") == 0xE40C292C
    assert fnv32("foobar") == 0xBF9CF968


def test_go_parity_values():
    # Values produced by the Go implementation (see services/core/internal/network/parity_test.go)
    assert exit_coach("ESPLANADE", 8) == 2
    assert round(float(base_occupancy(1.0, 18.5, 0)), 4) == 99.2935


def test_demand_shape():
    assert time_of_day_factor(3.0) == pytest.approx(0.02)
    assert time_of_day_factor(9.5) > time_of_day_factor(13.0) > time_of_day_factor(6.5) * 0.5
    assert base_occupancy(1.0, 9.5, 6) < base_occupancy(1.0, 9.5, 0)


def test_coach_bias_mean_one():
    b = coach_bias("HOWRAH", 6)
    assert np.mean(b) == pytest.approx(1.0)
    assert np.argmax(b) == exit_coach("HOWRAH", 6)


def test_synthetic_bounds_and_anomalies():
    df = station_occupancy(days=3)
    assert df["occupancy"].between(0, 100).all()
    assert df["station_id"].nunique() == 40
    an = inject_anomalies(df, frac=0.01)
    assert an["is_anomaly"].sum() > 0


@pytest.fixture(scope="module")
def client(tmp_path_factory):
    import os
    d = tmp_path_factory.mktemp("models")
    from metronav_ai.train import main
    main(["--days", "10", "--test-days", "3", "--eta-samples", "4000", "--out", str(d)])
    os.environ["MODEL_DIR"] = str(d)
    import importlib
    import metronav_ai.api as api
    importlib.reload(api)
    with TestClient(api.app) as c:
        yield c


def test_crowd_endpoint(client):
    r = client.post("/ml/crowd/predict", json={"station_id": "ESPLANADE", "timestamp": "2026-09-28T18:00:00+05:30",
                                              "current_occupancy": 70, "occupancy_15m_ago": 65, "occupancy_30m_ago": 60})
    assert r.status_code == 200
    body = r.json()
    assert all(0 <= body[k] <= 100 for k in ("forecast_15m", "forecast_30m", "forecast_60m"))


def test_anomaly_endpoint_flags_spike(client):
    pts = [{"station_id": "HOWRAH", "timestamp": "2026-09-28T03:00:00+05:30", "observed": 95}]
    r = client.post("/ml/anomaly/detect", json={"points": pts})
    assert r.status_code == 200 and r.json()["results"][0]["is_anomaly"] is True


def test_eta_endpoint_ordered(client):
    r = client.post("/ml/eta/predict", json={"scheduled_min": 30, "stops": 11, "transfers": 1, "mean_crowd": 60,
                                            "timestamp": "2026-09-28T09:30:00+05:30"})
    b = r.json()
    assert b["p10_min"] <= b["p50_min"] <= b["p90_min"]


def test_unknown_station_404(client):
    r = client.post("/ml/crowd/predict", json={"station_id": "NOPE", "timestamp": "2026-09-28T18:00:00+05:30",
                                              "current_occupancy": 1, "occupancy_15m_ago": 1, "occupancy_30m_ago": 1})
    assert r.status_code == 404
