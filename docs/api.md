# Core API (`/api/v1`)

Auth: `Authorization: Bearer <jwt>` from `/auth/login` or `/auth/register`. Errors are `{"error": "..."}`.

| Method & path | Access | Purpose |
|---|---|---|
| GET `/network` | public | Stations, lines, fares, data note |
| GET `/snapshot` | public | Full live state |
| GET `/stations`, `/stations/{id}` | public | Live crowding; departures per direction |
| GET `/stations/{id}/arrivals?line=&dest=` | public | Next trains with coach recommendation |
| GET `/trains`, `/trains/{id}?dest=` | public | Positions, coach loads |
| GET `/routes/search?from=&to=` | public | Distinct routes, ETA range, next departure |
| GET `/crowd/stations/{id}` | public | Live value + 6 h history |
| GET `/predictions/crowd?station=` | public | +15/30/60 min forecast |
| WS `/realtime` | public | Event stream |
| POST `/auth/register`, `/auth/login`; GET `/auth/me` | public / user | Accounts |
| POST `/tickets` `{from,to,profile}`; GET `/tickets`, `/tickets/{id}` | user | Ticketing |
| POST `/tickets/validate` `{token}` | operator, admin | Single-use validation |
| GET `/incidents?status=` | operator, analyst, admin | List |
| POST `/incidents`; PATCH `/incidents/{id}` `{status}` | operator, admin | Create / update |
| GET `/admin/overview`, `/ml/models` | operator, analyst, admin | KPIs, model metrics |
| POST `/simulation/scenarios`; DELETE `/simulation/scenarios/{id}` | operator, admin | Live scenarios |
| POST `/simulation/whatif` | operator, analyst, admin | Platform queue projection |
| POST `/events/ingest` | `X-Ingest-Key` | Camera `CROWD_UPDATE` (density 0–1 or people_count) |

Also: `GET /health`, `GET /ready`, `GET /metrics` (Prometheus text).

Scenario body examples:
```json
{"type":"crowd_surge","station_id":"SEALDAH","surge_pct":80,"duration_min":20}
{"type":"train_delay","line_id":"BLUE","delay_min":8}
{"type":"station_closure","station_id":"ESPLANADE","duration_min":15}
```

# ML API (`:8000`)
`POST /ml/crowd/predict`, `POST /ml/anomaly/detect`, `POST /ml/eta/predict`, `GET /ml/models`, `GET /health`. Interactive schema at `/docs`.
