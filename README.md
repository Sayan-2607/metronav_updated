# MetroNav

Metro journey planning with live crowding, coach recommendations, ticketing and an operations console, built as an engineering prototype on a simulated network.

**Stack:** Go (core API, routing, realtime) · Python (FastAPI, scikit-learn) · Next.js + TypeScript + Tailwind · PostgreSQL · Redis · Docker Compose

> **Read this first.** Train positions, station crowding and coach loads come from a built-in simulator. The ML models are trained on synthetic data. The network topology follows public Kolkata Metro station order, but segment times, fares, capacities and headways are approximations. Nothing here is fit for real operational use. See [Known limitations](#known-limitations).

---

## What it does

| Area | What is implemented |
|---|---|
| Journey planning | Dijkstra on a line-expanded graph (node = line+station). Four preference profiles (fastest, least crowded, fewest changes, balanced). Identical itineraries are merged so users only see real trade-offs. |
| Coach recommendation | For the next train, each coach gets the score `0.7·occupancy/100 + 0.3·distance-to-exit/(n−1)`; lowest wins. The formula is returned by the API. |
| Arrival prediction | Quantile gradient-boosting (10th/50th/90th percentile), shown as a range. |
| Crowd forecasting | Gradient boosting at +15/30/60 min, compared against persistence and usual-pattern baselines. |
| Anomaly detection | Residual z-score against a learned usual-load profile. Detections create incidents automatically. Causes are listed as unverified possibilities. |
| Live network | WebSocket stream of train positions and station crowding. Slow clients are disconnected instead of blocking others. |
| Ticketing | HMAC-signed QR tokens. Validation is atomic, so a ticket cannot be used twice. **No payment integration.** |
| Operations | KPIs, incident workflow (open → acknowledged → resolved), ticket check, model metrics. |
| Simulation | Inject delays, surges or closures into the live network. Separately, a "what if" fluid-queue projection for one platform, returned with its assumptions. |
| Camera pipeline | `ai/cv/crowd_counter.py`: YOLO + ByteTrack people counting in a floor zone, sending only aggregate counts. Frames are never stored. **Not evaluated; see limitations.** |
| Engineering | JWT + role-based access (PASSENGER / OPERATOR / ANALYST / ADMIN), PBKDF2 password hashing, rate limiting, audit log, Prometheus `/metrics`, `/health` and `/ready`. Fallbacks for Postgres, Redis and ML outages. Prediction cache with request coalescing. |

## Quick start (Docker)

```bash
cp .env.example .env     # then set JWT_SECRET, TICKET_SECRET, INGEST_KEY, ADMIN_PASSWORD
docker compose up --build
```

| Service | URL |
|---|---|
| Web app | http://localhost:3000 |
| Core API | http://localhost:8080 (`/ready` shows dependency status) |
| ML service | http://localhost:8000/docs |
| Prometheus (optional) | `docker compose --profile monitoring up` → http://localhost:9090 |

Notes on the Docker setup:
- The ML image trains its models during `docker build`, which takes about 20–60 s.
- **The Docker setup has not been run by the author.** Docker was not available in the build environment. Each service was run and tested natively, but the images and compose file are unverified. Expect to fix small issues on the first run.

**Demo accounts** (created on first start, password = `ADMIN_PASSWORD`, default `admin12345`):
- `admin@metronav.local` (ADMIN)
- `operator@metronav.local` (OPERATOR)

Change the password before sharing any deployment. Passengers register in the app.

## Run without Docker

Postgres and Redis are optional. Without them, the core uses an in-memory store (data is lost on restart) and skips Redis mirroring.

```bash
# 1. ML service (Python 3.12)
cd ai
pip install -r requirements-dev.txt
python -m metronav_ai.train                       # writes models/bundle.joblib + metrics.json
uvicorn metronav_ai.api:app --port 8000

# 2. Core (Go 1.22+)
cd services/core
ML_URL=http://localhost:8000 go run ./cmd/server
# optional: DATABASE_URL=postgres://... REDIS_ADDR=localhost:6379

# 3. Web (Node 20+)
cd apps/web
npm install && npm run dev                       # http://localhost:3000
```

A few things to know:
- `SIM_SPEED=10` runs the simulated clock 10× faster, so a whole evening peak plays out in about 20 minutes.
- The simulated clock starts at the current time in Asia/Kolkata. At night the network is correctly quiet.

**Demo walkthrough:**
1. Sign in as the operator.
2. Open **Simulation** and start a crowd surge of +80% at Sealdah.
3. Watch **Live network** and **Operations**. An anomaly incident should appear within about 10 seconds of real time.

## Tests

```bash
cd services/core && go vet ./... && go test ./...          # 6 packages
cd ai && python -m pytest -q tests                          # 10 tests
cd apps/web && npx tsc --noEmit && npm run build
```

Notable tests:
- PBKDF2 checked against the RFC 7914 test vector. JWT rejects `alg=none`, a wrong secret and expired tokens. Tampered tickets are rejected.
- Routing: interchange at Esplanade, closure producing "no route", crowding raising cost, profile de-duplication.
- Simulator invariants, arrival ordering, scenarios, camera-observation override.
- Parity tests that pin the same demand-model values in Go and Python, so the two implementations cannot drift apart.
- Cache request coalescing: 50 concurrent misses produce one upstream call, and errors are never cached.

## Measured results

All numbers below were produced in the build environment. Rerun them yourself before quoting any.

### Models: synthetic data only

Setup: 40 stations × 56 days at 15-minute resolution (152,320 rows). The last 14 days were held out.

| Crowd forecast (MAE, % points) | Model | Persistence | Usual pattern | Model R² |
|---|---|---|---|---|
| +15 min | **2.49** | 3.43 | 5.09 | 0.969 |
| +30 min | **3.13** | 5.64 | 5.11 | 0.951 |
| +60 min | **3.73** | 9.86 | 5.14 | 0.931 |

**Anomaly detection.** Threshold z ≥ 3, with synthetic +20–40 point spikes injected into 0.5% of held-out points:
- Precision 0.53, recall 0.96, F1 0.68, false-positive rate 0.43%.
- Low precision is expected here: rain and event effects in the synthetic data are not labelled as anomalies, but they still trip the detector.

**Arrival time**, median prediction on held-out synthetic journeys:

| | Mean abs. error | 95th pct. error |
|---|---|---|
| Model | 1.36 min | 3.70 min |
| Timetable | 2.56 min | 7.17 min |

The 10–90% range contained the actual time 77.7% of the time (target 80%).

These numbers show the models learn the patterns the simulator encodes. **They are not evidence of accuracy on real ridership.**

### Load test

Command: `go run ./cmd/loadtest -c 50 -d 20s -ws 500`, run against the core with the ML service up and `RATE_LIMIT_RPS=0`. Machine: 1 CPU, Linux, in-memory store.

- 78,067 requests in 20 s, about **3,900 req/s**
- Latency: p50 11.8 ms, p95 31.9 ms, p99 40.0 ms
- 0 errors
- 500 of 500 WebSocket clients connected, 11,000 messages received
- The prediction cache served the whole run with 5 model calls; every response used ML output, none used fallbacks

The request mix covered route search, station detail, snapshot and crowd forecast. Client and server shared the same CPU, so treat these figures as indicative only.

### Degradation

**ML service down.** In an earlier run the ML process had exited. The core kept serving all requests with 0 errors: the circuit breaker opened and fallback estimates were used, labelled `"source":"fallback"` in responses.

**Postgres + Redis (native, not Docker).** Verified end to end:
- Registration and duplicate-email rejection.
- Role enforcement: a passenger gets 403 on ticket validation.
- Atomic single-use validation.
- Incident lifecycle and audit rows.
- Network seeding.
- Occupancy persistence, including a camera-sourced row.
- Redis state keys and the event stream being written.

## Known limitations

- **Simulated data everywhere.** The demand curve (peaks at 9:30 and 18:30, weekend factors, station weights) is an assumption, not fitted to ridership.
- **Coach loads and exit positions are synthetic.** Exit positions come from a station-ID hash. The "Board" advice uses the train's current load, not a forecast of its load when it reaches you.
- **Payment is not integrated.** Tickets are issued free. Firebase auth and push notifications were not used; auth is the Go service's own JWT.
- **The camera pipeline is untested with real video or a real model.** Only its zone-counting and line-crossing logic has unit tests. Measure counting MAE on your own labelled footage before making any accuracy claim.
- **The "what if" projection is a fluid-queue approximation.** It has no crowd dynamics, train bunching or passenger route switching.
- **Architecture is a single deployable Go service (modular monolith), not microservices.** Routing, ticketing, simulation, realtime, incidents and ML-client code are separate packages behind narrow interfaces, so they can be split later.
- **Rate limiting is per process.** `cache.Redis.Allow` exists for a shared limit but isn't wired in by default.
- **The simulator runs in one process.** Running several core replicas would give each its own simulated network.
- **Accessibility routing (step-free) is not implemented** because the network data has no accessibility attributes.

## Repository layout

```
data/network.json            topology + assumptions (shared by Go and Python)
services/core/               Go: cmd/server, cmd/loadtest, internal/{route,sim,api,auth,store,realtime,crowd,ml,cache,metrics,network,config}
ai/metronav_ai/              Python: demand model, synthetic data, training, FastAPI service
ai/cv/crowd_counter.py       camera counting pipeline (optional deps)
apps/web/                    Next.js app: planner, live map, tickets, operations, simulation
infrastructure/monitoring/   Prometheus config
docs/                        architecture, API, data and ML notes
docker-compose.yml, .github/workflows/ci.yml
```

More detail: [docs/architecture.md](docs/architecture.md) · [docs/api.md](docs/api.md) · [docs/ml.md](docs/ml.md)
#   m e t r o n a v _ u p d a t e d  
 