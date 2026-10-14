# zzfuse Python service

FastAPI serves sentence embeddings and authenticated Chroma operations. Python 3.11 is the tested runtime. Direct dependency pins live in `requirements.in`; `requirements.txt` locks the resolved graph across platforms. Linux uses the official CPU PyTorch wheels by default.

## Local development

From the repository root:

```sh
make python-install
cp services/.env.example services/.env
```

Set separate `USER_API_KEY` and `ADMIN_API_KEY` values in the local environment file, then:

```sh
cd services
.venv/bin/uvicorn main:app --env-file .env --host 127.0.0.1 --port 8000
```

No built-in API key grants access. Send `X-API-Key` on protected requests. The admin key is required for destructive Chroma operations.

## API

- `POST /sentencer/embed`: `{ "texts": ["text"], "model_name": "sentence-transformers/all-MiniLM-L6-v2" }`; returns embeddings and the model name.
- `GET /sentencer/health`: service health behind API-key authentication.
- `GET /sentencer/model/dimension`, `/sentencer/model/context-window`, `/sentencer/model/info`: optional `model_name` query.
- `/chroma/*`: collection/document operations; `chroma_url` selects the server and must include HTTP/HTTPS scheme, host, and port.
- `/docs` and `/openapi.json`: generated API documentation.

Models load on demand and use an LRU cache. `DEFAULT_MODEL_NAME`, `DEVICE`, `MAX_MODELS_IN_CACHE`, `MAX_BATCH_SIZE`, and `MAX_CONCURRENT_REQUESTS` control operation. Changing models or embedding dimensions requires compatible vector storage. `PRELOAD_MODELS` is an older Compose setting; actual model warmup currently happens in the Docker build.

## Tests and upgrades

```sh
make python-test
cd services
uv pip compile requirements.in --python-version 3.11 --universal --index-strategy unsafe-best-match --emit-index-url -o requirements.txt
```

Tests exercise real FastAPI/Pydantic contracts with mocked inference and Chroma connections; they do not download weights or contact a server. For a different interpreter or environment, override `PYTHON` on the Makefile target.

The Dockerfile uses the standalone Uvicorn worker with Gunicorn and pre-downloads its configured models. Compose forwards API keys from the shell. An image build downloads weights; it is separate from the unit checks.
