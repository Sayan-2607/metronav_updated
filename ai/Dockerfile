FROM python:3.12-slim
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 NETWORK_FILE=/app/data/network.json MODEL_DIR=/app/ai/models
COPY ai/requirements.txt ai/requirements.txt
RUN pip install --no-cache-dir -r ai/requirements.txt
COPY data data
COPY ai ai
WORKDIR /app/ai
# Train on synthetic data at build time so the container starts ready (~20-60 s).
RUN python -m metronav_ai.train
RUN useradd -r -u 10001 app && chown -R app /app
USER app
EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=3s --retries=5 CMD python -c "import urllib.request;urllib.request.urlopen('http://localhost:8000/health',timeout=2)"
CMD ["uvicorn", "metronav_ai.api:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "2"]
