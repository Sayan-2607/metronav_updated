# Architecture

```
Browser (Next.js) ──HTTPS/WS──▶ Go core :8080 ──HTTP (timeout 800 ms, breaker, 30 s cache)──▶ Python ML :8000
                                    │  ├─ PostgreSQL (source of truth; memory fallback)
                                    │  └─ Redis (live state keys + capped event stream; optional)
Camera host (crowd_counter.py) ──POST /api/v1/events/ingest (X-Ingest-Key)──▶ Go core
```

## Go core packages
| Package | Responsibility |
|---|---|
| `network` | Loads `data/network.json`; demand model shared with Python |
| `route` | Line-expanded graph, Dijkstra per profile, fares |
| `sim` | Simulated trains, crowding, coach loads, scenarios, what-if queue model |
| `realtime` | WebSocket hub, event envelope, per-client topic filter, back-pressure |
| `crowd` | Anomaly monitor → incidents (ML or fallback z-score, 10-min cooldown per station) |
| `ml` | Python client: timeouts, circuit breaker (3 failures → open 15 s) |
| `store` | Postgres + in-memory implementations of one interface |
| `auth` | PBKDF2, HS256 JWT, signed ticket tokens (stdlib only) |
| `api` | Handlers, middleware (CORS, rate limit, logging, metrics, recover), background loop |

## Background loop (every `TICK_MS`, default 2 s)
1. Advance the simulation, record 90 min of density history (for lag features).
2. Broadcast `TRAIN_POSITIONS_UPDATED` and `CROWD_UPDATED`; mirror state to Redis.
3. Every 5 ticks: anomaly detection → `INCIDENT_CREATED`.
4. Every 15 ticks: persist occupancy rows.

## Failure handling
| Failure | Behaviour |
|---|---|
| ML slow/down | Breaker opens; heuristic fallbacks labelled `source: fallback`; errors never cached |
| Postgres unreachable at start | In-memory store; `/ready` shows `memory` |
| Redis unreachable | Mirroring skipped; core unaffected |
| Slow WebSocket client | Its buffer (64 msgs) fills → disconnected; client reconnects with backoff |
| Burst of identical predictions | Coalesced to one upstream call (singleflight) |

## Event envelope
```json
{"event_id":"evt_42","event_type":"CROWD_UPDATED","source":"simulator","timestamp":"...","payload":{...}}
```
Types: `TRAIN_POSITIONS_UPDATED`, `CROWD_UPDATED`, `INCIDENT_CREATED`, `SCENARIO_CHANGED`, `TICKET_BOOKED`.
Clients may send `{"subscribe":["CROWD_UPDATED"]}` to filter.
