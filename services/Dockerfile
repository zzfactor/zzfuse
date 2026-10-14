FROM python:3.14.8-slim-bookworm@sha256:48b13b003dda20b16f9442b8475aa05fe21bf6579a8c881db92ffb4d8fd20f83

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && pip check

# Optional public model download for deployments that need an offline runtime.
ARG DOWNLOAD_MODELS=""
ENV HF_HOME=/var/cache/zzfuse/models \
    DEVICE=cpu \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
RUN useradd --create-home --uid 10001 app && mkdir -p "$HF_HOME" && chown app:app "$HF_HOME" /app
USER app
RUN python -c "import os; from sentence_transformers import SentenceTransformer; [SentenceTransformer(name.strip(), device='cpu') for name in os.environ.get('DOWNLOAD_MODELS', '').split(',') if name.strip()]"

COPY --chown=app:app . .

EXPOSE 8000

CMD ["sh", "-c", "exec gunicorn -w \"${GUNICORN_WORKERS:-1}\" -k uvicorn_worker.UvicornWorker main:app --bind 0.0.0.0:8000"]
