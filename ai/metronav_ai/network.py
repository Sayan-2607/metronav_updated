from __future__ import annotations

import json
import os
from functools import lru_cache
from pathlib import Path

DEFAULT = Path(__file__).resolve().parents[2] / "data" / "network.json"


@lru_cache(maxsize=1)
def load_network(path: str | None = None) -> dict:
    p = Path(path or os.environ.get("NETWORK_FILE", DEFAULT))
    with open(p) as f:
        net = json.load(f)
    net["station_index"] = {s["id"]: i for i, s in enumerate(net["stations"])}
    net["station_by_id"] = {s["id"]: s for s in net["stations"]}
    return net
