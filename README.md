# MetroNav

## Crowd Intelligence, Smart Routing and Predictive Metro Operations Platform

MetroNav is an intelligent metro transportation platform designed to combine real-time crowd intelligence, predictive analytics, route optimization, coach-level recommendations, smart ticketing, live network monitoring, and operational simulation into a single system.

The platform uses a Go-based core backend for high-performance routing and real-time services, a Python-based machine learning service for crowd and demand prediction, and a Next.js frontend for passenger and operations interfaces.

MetroNav is designed as an engineering prototype and simulation platform. It does not connect to real metro infrastructure or operational railway systems.

---

## Overview

Traditional metro journey planners primarily focus on static route information such as stations, lines, transfers, and estimated travel time.

MetroNav extends this concept by introducing crowd-aware decision making.

The system can combine:

* Metro network topology
* Current station occupancy
* Historical demand patterns
* Predicted future crowd levels
* Coach-level congestion
* Journey duration
* Number of transfers
* Service delays
* Events and holidays
* Weather conditions
* Simulated real-time passenger movement

The platform can then provide passengers and operators with more context-aware information.

### Example passenger flow

A passenger can:

1. Select an origin station.
2. Select a destination station.
3. Request a route.
4. Compare possible routes.
5. View estimated journey time.
6. View predicted crowd levels.
7. Receive coach recommendations.
8. Select a coach during ticket booking.
9. Monitor the journey through the live interface.

### Example operator flow

An operator can:

1. Open the operations dashboard.
2. Monitor network activity.
3. Start or control simulations.
4. Observe station and coach congestion.
5. Inspect demand patterns.
6. Identify anomalies.
7. Run what-if scenarios.
8. Monitor system metrics.

---

# Key Features

## 1. Intelligent Route Planning

MetroNav provides route planning across a simulated metro network.

The routing engine considers:

* Origin station
* Destination station
* Available connections
* Transfers
* Travel time
* Network topology
* Crowd conditions

The Go backend contains the core routing implementation.

---

## 2. Crowd Intelligence

The platform models passenger occupancy at stations and coaches.

Crowd information can be represented as:

* Current occupancy
* Historical occupancy
* Predicted occupancy
* Station congestion
* Coach congestion
* Crowd intensity
* Demand trends

Crowd levels are represented on a normalized scale from 0 to 100.

---

## 3. Machine Learning Crowd Prediction

MetroNav includes a dedicated Python machine learning service.

The ML service predicts future crowd levels for multiple time horizons:

* 15 minutes
* 30 minutes
* 60 minutes

The model uses features such as:

* Current occupancy
* Previous occupancy
* Time
* Station
* Historical demand
* Rain conditions
* Nearby events
* Holiday information

The project currently uses synthetic data for model development and evaluation.

This makes the system suitable for demonstrating the architecture without requiring access to proprietary metro passenger datasets.

---

## 4. Demand Forecasting

The demand forecasting pipeline generates and processes historical passenger demand data.

The training system supports:

* Synthetic data generation
* Feature engineering
* Model training
* Model evaluation
* Baseline comparison
* Model persistence
* Metrics generation

The training pipeline can be executed using:

```powershell
cd ai
python -m metronav_ai.train
```

---

## 5. Anomaly Detection

MetroNav includes an anomaly detection service for identifying unusual occupancy patterns.

The anomaly system can detect unexpected changes in observed crowd levels.

Potential examples include:

* Sudden station crowd increases
* Unexpected occupancy drops
* Abnormal passenger demand
* Unusual network conditions

The anomaly API accepts station-level observations and returns detected anomalies.

---

## 6. ETA Prediction

MetroNav includes a machine learning-assisted ETA prediction service.

ETA prediction considers:

* Scheduled travel time
* Number of stops
* Number of transfers
* Mean crowd level
* Timestamp
* Delay conditions

The service provides an estimated travel time and can be compared against a baseline estimate.

---

## 7. Coach-Level Crowd Intelligence

Instead of treating an entire train as a single occupancy value, MetroNav models congestion at coach level.

This allows the application to provide recommendations such as:

* Less crowded coach
* Moderately crowded coach
* Highly crowded coach
* Coach-level occupancy visualization

The frontend contains a dedicated coach visualization component.

---

## 8. Smart Ticketing

The ticketing interface allows users to simulate metro journey booking.

The workflow can include:

* Origin selection
* Destination selection
* Route selection
* Coach selection
* Journey information
* Ticket information

The ticketing functionality is part of the simulated platform and does not represent a connection to an actual metro ticketing system.

---

## 9. Live Network Interface

The frontend includes a live network interface for displaying simulated metro conditions.

The interface can visualize:

* Stations
* Routes
* Crowd levels
* Train movement
* Network conditions
* Journey information

The application is designed to support real-time updates through the backend architecture.

---

## 10. Operations Dashboard

MetroNav includes an administrative operations dashboard.

The dashboard provides interfaces for:

* Network monitoring
* Simulation
* Crowd monitoring
* System activity
* Operational analysis

The operations interface is intended for demonstration and experimentation.

---

## 11. Network Simulation

The Go backend includes a simulation engine for generating dynamic network activity.

Simulation functionality can model:

* Passenger movement
* Station demand
* Crowd changes
* Train movement
* Network conditions
* What-if scenarios

This enables the platform to demonstrate real-time behavior without connecting to real metro infrastructure.

---

## 12. Computer Vision Crowd Counting

MetroNav also includes an optional computer vision pipeline.

Location:

```text
ai/cv/crowd_counter.py
```

The computer vision pipeline is designed to estimate crowd levels from camera footage.

It can be extended to support:

* Person detection
* Crowd counting
* Occupancy estimation
* Camera-based station monitoring

The CV component is optional and is separated from the primary ML prediction service.

---

# System Architecture

```text
                         MetroNav Platform
                                |
              +-----------------+-----------------+
              |                 |                 |
              v                 v                 v
       Next.js Frontend     Go Core API       Python ML API
          Port 3000          Port 8080          Port 8000
              |                 |                 |
              |                 |                 |
              |          +------+-------+         |
              |          |              |         |
              |          v              v         |
              |       Routing       Simulation    |
              |       Engine        Engine        |
              |          |              |         |
              |          +------+-------+         |
              |                 |                 |
              |                 v                 |
              |           Realtime Hub             |
              |                                   |
              +----------------+------------------+
                               |
                       PostgreSQL / Redis
                         Optional Storage
```

---

# Technology Stack

## Frontend

* Next.js
* React
* TypeScript
* Tailwind CSS

## Backend

* Go
* REST APIs
* WebSocket/realtime architecture
* JWT authentication

## Machine Learning

* Python
* FastAPI
* NumPy
* Pandas
* Scikit-learn
* Joblib

## Computer Vision

* Python
* OpenCV

## Databases

* PostgreSQL
* Redis

Both PostgreSQL and Redis are optional for the basic local development environment because the core service supports an in-memory mode.

## Infrastructure

* Docker
* Docker Compose
* GitHub Actions
* Prometheus

---

# Project Structure

```text
metronav/
|
├── ai/
│   ├── cv/
│   │   ├── __init__.py
│   │   └── crowd_counter.py
│   │
│   ├── metronav_ai/
│   │   ├── __init__.py
│   │   ├── api.py
│   │   ├── demand.py
│   │   ├── models.py
│   │   ├── network.py
│   │   ├── synthetic.py
│   │   └── train.py
│   │
│   ├── models/
│   │   └── .gitkeep
│   │
│   ├── tests/
│   │   ├── test_ai.py
│   │   └── test_cv_counter.py
│   │
│   ├── Dockerfile
│   ├── requirements.txt
│   ├── requirements-dev.txt
│   └── requirements-cv.txt
│
├── apps/
│   └── web/
│       ├── app/
│       │   ├── admin/
│       │   ├── live/
│       │   ├── login/
│       │   ├── tickets/
│       │   ├── globals.css
│       │   ├── layout.tsx
│       │   └── page.tsx
│       │
│       ├── components/
│       │   ├── CoachStrip.tsx
│       │   ├── Nav.tsx
│       │   ├── NetworkMap.tsx
│       │   ├── RouteStrip.tsx
│       │   └── StationSelect.tsx
│       │
│       ├── lib/
│       │   ├── api.ts
│       │   ├── format.ts
│       │   ├── hooks.ts
│       │   └── types.ts
│       │
│       ├── package.json
│       └── Dockerfile
│
├── data/
│   └── network.json
│
├── docs/
│   ├── api.md
│   ├── architecture.md
│   └── ml.md
│
├── infrastructure/
│   └── monitoring/
│       └── prometheus.yml
│
├── services/
│   └── core/
│       ├── cmd/
│       │   ├── loadtest/
│       │   └── server/
│       │
│       ├── internal/
│       │   ├── api/
│       │   ├── auth/
│       │   ├── cache/
│       │   ├── config/
│       │   ├── crowd/
│       │   ├── metrics/
│       │   ├── ml/
│       │   ├── network/
│       │   ├── realtime/
│       │   ├── route/
│       │   ├── sim/
│       │   └── store/
│       │
│       ├── Dockerfile
│       ├── go.mod
│       └── go.sum
│
├── .github/
│   └── workflows/
│       └── ci.yml
│
├── .env.example
├── .gitignore
├── docker-compose.yml
└── README.md
```

---

# Local Development

## Prerequisites

Install the following:

* Git
* Go
* Python 3.11+
* Node.js 18+
* npm

PostgreSQL and Redis are optional for the basic development environment.

---

# 1. Clone the Repository

```bash
git clone https://github.com/Sayan-2607/metronav_updated.git
cd metronav_updated
```

---

# 2. Configure Environment Variables

Create the local environment file:

```powershell
Copy-Item .env.example .env
```

Do not commit `.env`.

The repository intentionally contains `.env.example` files instead of real credentials.

---

# 3. Start the Machine Learning Service

Open a terminal:

```powershell
cd ai
```

Create a virtual environment:

```powershell
python -m venv .venv
```

Activate it:

```powershell
.\.venv\Scripts\Activate.ps1
```

Install dependencies:

```powershell
python -m pip install --upgrade pip
pip install -r requirements-dev.txt
```

---

# 4. Train the ML Models

Run:

```powershell
python -m metronav_ai.train
```

The training pipeline generates the required model artifacts locally.

Generated model files are intentionally excluded from Git using `.gitignore`.

After training, start the ML API:

```powershell
uvicorn metronav_ai.api:app --reload --port 8000
```

The ML service will be available at:

```text
http://localhost:8000
```

Swagger API documentation:

```text
http://localhost:8000/docs
```

---

# 5. Verify the ML Service

Open:

```text
http://localhost:8000/docs
```

Available endpoints include:

```text
GET  /health
GET  /ml/models
POST /ml/crowd/predict
POST /ml/anomaly/detect
POST /ml/eta/predict
```

You can also test:

```text
http://localhost:8000/health
```

---

# 6. Start the Go Core API

Open a second terminal.

```powershell
cd services/core
```

Set the ML service URL:

```powershell
$env:ML_URL="http://localhost:8000"
```

Start the Go server:

```powershell
go run ./cmd/server
```

The core API runs on:

```text
http://localhost:8080
```

The Go service provides the main application backend, including:

* Authentication
* Routing
* Network operations
* Simulation
* Crowd processing
* ML integration
* Realtime functionality
* Metrics

---

# 7. Start the Next.js Frontend

Open a third terminal:

```powershell
cd apps/web
```

Install dependencies:

```powershell
npm install
```

Start the development server:

```powershell
npm run dev
```

Open:

```text
http://localhost:3000
```

---

# Running MetroNav Locally

MetroNav requires three primary processes during native development.

```text
Terminal 1
Python ML Service
localhost:8000

Terminal 2
Go Core API
localhost:8080

Terminal 3
Next.js Frontend
localhost:3000
```

The communication flow is:

```text
Browser
   |
   v
Next.js
   |
   v
Go Core API
   |
   v
Python ML Service
```

---

# Docker Development

MetroNav also includes Docker configuration.

From the repository root:

```powershell
docker compose up --build
```

This is intended to start the services defined in `docker-compose.yml`.

To stop the services:

```powershell
docker compose down
```

To rebuild:

```powershell
docker compose up --build
```

For development and debugging, native execution is recommended because it provides easier access to individual service logs.

---

# Authentication

MetroNav uses the Go backend for authentication.

The development environment includes a demo administrator account configured through environment variables.

Default development credentials:

```text
Email:
admin@metronav.local

Password:
admin12345
```

These credentials are intended only for local development.

For any deployment outside a local development environment, change the administrator password and use secure secrets.

---

# API Services

## Go Core API

```text
http://localhost:8080
```

Responsibilities:

* Authentication
* Route planning
* Network management
* Simulation
* Crowd processing
* Realtime communication
* ML service integration
* Metrics

---

## Python ML API

```text
http://localhost:8000
```

Swagger:

```text
http://localhost:8000/docs
```

Responsibilities:

* Crowd prediction
* Demand forecasting
* Anomaly detection
* ETA prediction
* Model management

---

# Machine Learning Pipeline

The ML workflow is:

```text
Synthetic / Historical Data
          |
          v
Data Generation
          |
          v
Feature Engineering
          |
          v
Model Training
          |
          v
Validation
          |
          v
Model Evaluation
          |
          v
Model Artifact
          |
          v
FastAPI ML Service
          |
          v
Go Core API
          |
          v
Next.js Application
```

---

# Model Inputs

## Crowd Prediction

The crowd prediction API accepts information such as:

```text
station_id
timestamp
current_occupancy
occupancy_15m_ago
occupancy_30m_ago
rain
event_nearby
holiday
```

Occupancy is represented as a value between:

```text
0 and 100
```

---

## ETA Prediction

ETA prediction uses:

```text
scheduled_min
stops
transfers
mean_crowd
timestamp
line_delayed
```

---

## Anomaly Detection

The anomaly detection endpoint accepts station-level observations.

Each observation contains:

```text
station_id
timestamp
observed
```

The service supports batches of observations for analysis.

---

# ML Evaluation

The current development dataset is synthetic.

Example training configuration:

```text
Stations: 40
Historical period: 56 days
Time interval: 15 minutes
Rows: 152,320
Test period: 14 days
```

During development, the model was evaluated against persistence and profile-based baselines.

Example development results:

```text
Crowd prediction

15-minute MAE
Model:       2.491
Persistence: 3.434
Profile:     5.085

30-minute MAE
Model:       3.140
Persistence: 5.643
Profile:     5.107

60-minute MAE
Model:       3.742
Persistence: 9.856
Profile:     5.138
```

ETA development evaluation:

```text
Model MAE:          1.356 minutes
Baseline MAE:       2.558 minutes

Model median error: 1.051 minutes
Baseline median:    1.889 minutes
```

These results are based on synthetic development data and should not be interpreted as real-world metro performance.

---

# Computer Vision

MetroNav contains an optional computer vision crowd-counting module:

```text
ai/cv/crowd_counter.py
```

The component is designed to process camera input and estimate the number of people in a monitored area.

The CV layer can eventually be integrated with the crowd intelligence pipeline:

```text
Camera
  |
  v
Person Detection
  |
  v
Crowd Count
  |
  v
Occupancy Estimation
  |
  v
MetroNav Core
  |
  v
Prediction and Monitoring
```

The current repository treats the CV system as an optional component rather than requiring a camera for normal application operation.

---

# Real-Time Architecture

MetroNav is designed around a real-time event flow.

```text
Simulation / Sensor / CV
          |
          v
     Crowd Monitor
          |
          v
      Go Core API
          |
          v
     Realtime Hub
          |
          v
      Web Client
```

This architecture allows the frontend to consume changing network conditions without repeatedly rebuilding the entire application state.

---

# Data Layer

MetroNav supports multiple storage approaches.

## PostgreSQL

PostgreSQL is intended for persistent application data.

Potential data includes:

* Users
* Tickets
* Journeys
* Network information
* Historical measurements
* Operational data

## Redis

Redis can be used for:

* Caching
* Temporary state
* Realtime workloads
* Fast-access information

## In-Memory Mode

For local development, the Go service can operate using an in-memory store.

This allows the core service to be developed without requiring PostgreSQL or Redis.

---

# Monitoring

The repository includes Prometheus configuration:

```text
infrastructure/monitoring/prometheus.yml
```

The monitoring architecture can be extended to collect:

* API latency
* Request counts
* Error rates
* Simulation metrics
* ML request metrics
* Crowd processing metrics
* System health

---

# Testing

## Python Tests

From the `ai` directory:

```powershell
pytest
```

or:

```powershell
python -m pytest
```

---

## Go Tests

From:

```text
services/core
```

run:

```powershell
go test ./...
```

The Go project includes tests covering areas such as:

* Authentication
* Routing
* Simulation
* Crowd monitoring
* Network behavior
* API utilities

---

# Load Testing

MetroNav contains a Go load-testing command:

```text
services/core/cmd/loadtest
```

Run it with:

```powershell
cd services/core
go run ./cmd/loadtest
```

The load-test implementation can be extended to evaluate:

* Concurrent API requests
* Route calculation performance
* Realtime workloads
* Backend throughput
* Response latency

---

# CI/CD

GitHub Actions configuration is available at:

```text
.github/workflows/ci.yml
```

The CI pipeline is intended to validate the project automatically.

The project can be extended with additional checks for:

* Go tests
* Python tests
* TypeScript compilation
* Next.js build
* Formatting
* Static analysis
* Security scanning

---

# Security Considerations

MetroNav is currently a development and research prototype.

Important security practices include:

* Never commit `.env`
* Never commit production credentials
* Change default administrator credentials
* Store secrets using environment variables
* Use HTTPS in production
* Use secure JWT configuration
* Validate API input
* Apply rate limiting
* Restrict administrative endpoints
* Protect database credentials
* Use secure password hashing
* Apply appropriate authorization rules

The repository's `.gitignore` intentionally excludes:

```text
.env
.venv/
venv/
node_modules/
.next/
__pycache__/
*.pyc
*.log
```

and generated ML model artifacts.

---

# Development Environment

Recommended development environment:

```text
Operating System:
Windows / Linux / macOS

Frontend:
Node.js 18+

Backend:
Go

Machine Learning:
Python 3.11+

Database:
PostgreSQL

Cache:
Redis

Containerization:
Docker / Docker Compose
```

---

# Current Limitations

MetroNav is an engineering prototype and has several limitations.

## Synthetic Data

The current machine learning system is trained and evaluated using synthetic data.

Real metro datasets would be required for production-grade model validation.

## Simulated Network

The metro network is represented through project data and simulation.

It is not connected to a real metro control system.

## Simulated Tickets

Ticketing functionality is intended for demonstration.

It does not issue real transportation tickets.

## Computer Vision

The CV module is optional and requires an appropriate camera/video input pipeline.

## Production Deployment

The current project is primarily designed for local development, research, demonstrations, and portfolio presentation.

Additional production engineering would be required before deployment at transportation-infrastructure scale.

---

# Future Development

Potential future improvements include:

## Passenger Intelligence

* Personalized route recommendations
* Accessibility-aware routing
* Low-crowd route preferences
* Travel history
* Personalized travel patterns

## Advanced Crowd Prediction

* Graph neural networks
* Temporal transformers
* Spatiotemporal forecasting
* Multi-station forecasting
* Real passenger demand datasets
* Probabilistic forecasting

## Computer Vision

* Multi-camera tracking
* Improved person detection
* Station zone detection
* Platform density estimation
* Coach-level camera analytics

## Operations Intelligence

* Predictive disruption detection
* Automated incident alerts
* Network-wide demand forecasting
* Train headway optimization
* Platform congestion prediction
* What-if operational planning

## Smart Ticketing

* QR-based tickets
* Dynamic fare calculation
* Digital ticket validation
* Payment integration
* Passenger flow analytics

## Infrastructure

* Kubernetes deployment
* Horizontal API scaling
* Distributed event streaming
* Kafka integration
* Production observability
* Distributed tracing

---

# Design Philosophy

MetroNav is designed around several principles.

### Separate Intelligence from Application Logic

Machine learning functionality is isolated inside the Python service while the Go service manages core application logic.

### Keep the Core Backend Fast

Go handles routing, simulation, realtime processing, and API operations.

### Make ML Replaceable

The ML API is accessed through a service boundary so models can be replaced without redesigning the entire application.

### Design for Real-Time Data

The system is structured around continuously changing network conditions rather than static route information alone.

### Develop with Simulation First

Simulation allows the system to be developed and demonstrated without requiring access to real transportation infrastructure.

---

# Project Goals

MetroNav aims to demonstrate how modern software engineering and machine learning can be combined to build intelligent transportation applications.

The project brings together:

```text
Software Engineering
        +
Machine Learning
        +
Computer Vision
        +
Real-Time Systems
        +
Optimization
        +
Data Engineering
        +
Smart Ticketing
        +
Transportation Simulation
```

The objective is not simply to calculate the shortest route, but to explore how a journey planner can become a context-aware transportation intelligence platform.

---

# Repository

GitHub:

https://github.com/Sayan-2607/metronav_updated

---

# Documentation

Additional technical documentation is available in:

```text
docs/
├── api.md
├── architecture.md
└── ml.md
```

---

# Quick Reference

## Start ML

```powershell
cd ai
.\.venv\Scripts\Activate.ps1
uvicorn metronav_ai.api:app --reload --port 8000
```

## Start Go Core

```powershell
cd services/core
$env:ML_URL="http://localhost:8000"
go run ./cmd/server
```

## Start Frontend

```powershell
cd apps/web
npm install
npm run dev
```

## URLs

```text
Frontend
http://localhost:3000

Go API
http://localhost:8080

ML API
http://localhost:8000

ML Swagger
http://localhost:8000/docs
```

---

# Local Demo Credentials

```text
Email:
admin@metronav.local

Password:
admin12345
```

Use these credentials only for the local development configuration.

---

# License

If this project is intended for public distribution, add an explicit open-source license such as MIT, Apache-2.0, or another license appropriate to the project.

Until a license is added, the repository should not be assumed to grant broad permission to copy, modify, or redistribute the source code.

---

# Author

Sayan Ghosh

B.Tech Computer Science and Engineering

KIIT University

GitHub:

https://github.com/Sayan-2607

LinkedIn:

https://linkedin.com/in/sayan-g-600ab5307

---

# Project Status

MetroNav is an active engineering prototype focused on:

* Intelligent metro routing
* Crowd prediction
* Smart ticketing
* Coach-level intelligence
* Real-time simulation
* Machine learning
* Computer vision
* Transportation analytics
* Operational decision support

The architecture is intentionally modular so that individual components can evolve independently as the project moves toward more realistic datasets, improved prediction models, stronger realtime infrastructure, and production-oriented deployment.
