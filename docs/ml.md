# Data and models

## Synthetic demand model (`ai/metronav_ai/demand.py`, mirrored in Go)
`occ% = 100 · (0.08 + 0.95 · tod(hour) · weekday_factor · station_weight)`, with `tod` = 0.15 + Gaussian peaks at 9.5 h (amp 0.85), 18.5 h (0.80) and 13.5 h (0.25), capped at 1, and 0.02 outside 06:00–22:30. Saturday ×0.75, Sunday ×0.55.
Training data adds holidays (×0.6), rain days (×1.08), nearby events (up to +25 points) and AR(1) noise (φ = 0.8, σ = 3). **All assumptions; none fitted to real ridership.**

## Models (`python -m metronav_ai.train`)
| Model | Method | Evaluation |
|---|---|---|
| Crowd forecast | HistGradientBoosting; features: station, calendar, current + 15/30-min lags, horizon, usual-pattern value at target time | Time split (last 14 days); MAE/RMSE/MAPE (MAPE ignores targets < 5%)/R² vs persistence and usual-pattern baselines |
| Anomaly | HGB usual-load profile + per-station residual σ; flag z ≥ 3 (over-crowding only) | Injected spikes on held-out days: precision/recall/F1/FPR |
| ETA | Three quantile HGB models (0.1/0.5/0.9), ordered at inference | 80/20 split; MAE, median and p95 error vs timetable; interval coverage |

Metrics are written to `ai/models/metrics.json` and shown on the Operations page.

## Using real data
Replace `synthetic.station_occupancy` with a loader producing the same columns (`station_id, station_idx, weight, timestamp, hour, weekday, holiday, rain, event, occupancy`), and replace the `profile_target` feature with a historical mean per station and time slot. Re-evaluate before claiming anything.

## Camera counting
`ai/cv/crowd_counter.py` counts detections whose bottom-centre (feet) falls inside a normalised floor polygon, smooths with a 15-frame median, and counts crossings of an optional entry line using ByteTrack IDs. Only counts are sent. Capacity for the zone must come from a site survey. Evaluate counting MAE against manual counts on your footage.
