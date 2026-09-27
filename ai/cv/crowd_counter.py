"""Privacy-first platform crowd counter.

Detects and tracks people in a video stream (YOLO + ByteTrack via Ultralytics),
counts them inside a platform zone polygon and across an entry line, and POSTs
aggregate CROWD_UPDATE events to the MetroNav core. Frames and per-person data
are never stored or transmitted: only counts leave this process.

    pip install -r requirements-cv.txt
    python cv/crowd_counter.py --source video.mp4 --station ESPLANADE \
        --zone "0.05,0.55 0.95,0.55 0.95,0.98 0.05,0.98" --capacity 400

Status: this module is written against the documented Ultralytics and OpenCV
APIs but has NOT been evaluated in this repository (no labelled station
footage is included). Counting accuracy (MAE vs. manual counts) must be
measured on your own footage before any claim is made about it.
"""
from __future__ import annotations

import argparse
import json
import os
import time
import urllib.request
from collections import deque
from dataclasses import dataclass, field

import cv2
import numpy as np


def parse_polygon(s: str) -> np.ndarray:
    """'x,y x,y ...' in normalised [0,1] coordinates."""
    pts = [tuple(map(float, p.split(","))) for p in s.split()]
    if len(pts) < 3:
        raise ValueError("zone needs at least 3 points")
    return np.array(pts, dtype=np.float32)


def side_of_line(p, a, b) -> float:
    return (b[0] - a[0]) * (p[1] - a[1]) - (b[1] - a[1]) * (p[0] - a[0])


@dataclass
class ZoneCounter:
    """Aggregates per-frame detections into privacy-safe statistics."""
    zone: np.ndarray                      # normalised polygon
    line: tuple | None = None             # ((x1,y1),(x2,y2)) normalised entry line
    window: int = 15                      # frames for smoothing
    history: deque = field(default_factory=lambda: deque(maxlen=15))
    last_side: dict = field(default_factory=dict)
    entries: int = 0
    exits: int = 0

    def update(self, centres: list[tuple[float, float]], track_ids: list[int | None]) -> int:
        in_zone = 0
        for (x, y), tid in zip(centres, track_ids):
            if cv2.pointPolygonTest(self.zone.reshape(-1, 1, 2), (x, y), False) >= 0:
                in_zone += 1
            if self.line is not None and tid is not None:
                s = np.sign(side_of_line((x, y), *self.line))
                prev = self.last_side.get(tid)
                if prev is not None and s != 0 and s != prev:
                    if s > 0:
                        self.entries += 1
                    else:
                        self.exits += 1
                if s != 0:
                    self.last_side[tid] = s
        if len(self.last_side) > 5000:  # forget stale tracks; ids are ephemeral anyway
            self.last_side.clear()
        self.history.append(in_zone)
        return in_zone

    def smoothed(self) -> float:
        return float(np.median(self.history)) if self.history else 0.0


def post_event(api: str, key: str, station: str, people: int, density: float, zone: str, flow: dict) -> None:
    body = json.dumps({
        "event_type": "CROWD_UPDATE", "source": "station_camera", "station_id": station,
        "payload": {"people_count": people, "density": round(density, 4), "zone": zone, **flow},
    }).encode()
    req = urllib.request.Request(f"{api}/api/v1/events/ingest", data=body, method="POST",
                                 headers={"Content-Type": "application/json", "X-Ingest-Key": key})
    with urllib.request.urlopen(req, timeout=3) as r:
        r.read()


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--source", required=True, help="video file, RTSP URL or camera index")
    ap.add_argument("--station", required=True)
    ap.add_argument("--zone", required=True, help="normalised polygon 'x,y x,y ...'")
    ap.add_argument("--line", help="normalised entry line 'x1,y1 x2,y2'")
    ap.add_argument("--capacity", type=int, required=True, help="people the zone holds at 100%% (from station survey)")
    ap.add_argument("--model", default="yolov8n.pt")
    ap.add_argument("--conf", type=float, default=0.35)
    ap.add_argument("--every", type=float, default=5.0, help="seconds between events")
    ap.add_argument("--api", default=os.environ.get("METRONAV_API", "http://localhost:8080"))
    ap.add_argument("--key", default=os.environ.get("INGEST_KEY", "dev-ingest-key"))
    ap.add_argument("--dry-run", action="store_true", help="print events instead of posting")
    a = ap.parse_args()

    from ultralytics import YOLO  # imported lazily so unit tests don't need it

    model = YOLO(a.model)
    line = None
    if a.line:
        p = parse_polygon(a.line + " 0,0")[:2]
        line = (tuple(p[0]), tuple(p[1]))
    counter = ZoneCounter(zone=parse_polygon(a.zone), line=line)
    source = int(a.source) if a.source.isdigit() else a.source
    last = time.monotonic()

    # stream=True yields results frame by frame; persist=True keeps ByteTrack state
    for res in model.track(source=source, stream=True, persist=True, classes=[0], conf=a.conf,
                           tracker="bytetrack.yaml", verbose=False):
        h, w = res.orig_shape
        centres, ids = [], []
        if res.boxes is not None and len(res.boxes):
            xyxy = res.boxes.xyxy.cpu().numpy()
            tids = res.boxes.id.cpu().numpy().astype(int).tolist() if res.boxes.id is not None else [None] * len(xyxy)
            for (x1, y1, x2, y2), tid in zip(xyxy, tids):
                # bottom-centre ~ feet position, the right point to test against a floor zone
                centres.append(((x1 + x2) / 2 / w, y2 / h))
                ids.append(tid)
        counter.update(centres, ids)
        del res  # frame is discarded here; nothing is written to disk

        if time.monotonic() - last >= a.every:
            last = time.monotonic()
            people = int(round(counter.smoothed()))
            density = min(people / a.capacity, 1.5)
            flow = {"entries": counter.entries, "exits": counter.exits}
            if a.dry_run:
                print(json.dumps({"station": a.station, "people": people, "density": density, **flow}))
            else:
                try:
                    post_event(a.api, a.key, a.station, people, density, "PLATFORM", flow)
                except Exception as e:  # keep counting even if the API is briefly down
                    print(f"post failed: {e}")
            counter.entries = counter.exits = 0


if __name__ == "__main__":
    main()
