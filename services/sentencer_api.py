from fastapi import APIRouter, HTTPException, Query
from pydantic import BaseModel, Field
from sentence_transformers import SentenceTransformer
from starlette.concurrency import run_in_threadpool
import asyncio
import logging
import os
import time
from collections import OrderedDict
from prometheus_client import Counter, Histogram, Gauge

router = APIRouter(prefix="/sentencer", tags=["Embedding"])

# Service configuration
DEFAULT_MODEL_NAME = os.getenv("DEFAULT_MODEL_NAME", "sentence-transformers/static-retrieval-mrl-en-v1")
DEVICE = os.getenv("DEVICE", "cpu")
MAX_MODELS_IN_CACHE = int(os.getenv("MAX_MODELS_IN_CACHE", "3"))
MAX_BATCH_SIZE = int(os.getenv("MAX_BATCH_SIZE", "128"))
MAX_CONCURRENT_REQUESTS = int(os.getenv("MAX_CONCURRENT_REQUESTS", "10"))
PRELOAD_MODELS = os.getenv("PRELOAD_MODELS", "")

# Logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# API Models
class EmbedRequest(BaseModel):
    texts: list[str] = Field(
        ...,
        min_length=1,
        max_length=MAX_BATCH_SIZE,
        examples=[["This is a test.", "That was a test."]]
    )
    model_name: str | None = Field(
        default=DEFAULT_MODEL_NAME,
        description="The name of the SentenceTransformer model to use.",
        examples=["sentence-transformers/all-MiniLM-L6-v2"]
    )

class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model_name: str

class HealthResponse(BaseModel):
    status: str

# Model cache and concurrency
model_cache = OrderedDict()
model_load_lock = asyncio.Lock()
concurrency_limiter = asyncio.Semaphore(MAX_CONCURRENT_REQUESTS)

# Prometheus metrics
cache_hits = Counter('embedding_model_cache_hits_total', 'Number of model cache hits.')
cache_misses = Counter('embedding_model_cache_misses_total', 'Number of model cache misses.')
cache_evictions = Counter('embedding_model_cache_evictions_total', 'Number of model cache evictions.')
model_load_duration = Histogram('embedding_model_load_duration_seconds', 'Duration of model loading in seconds.', ['model_name'])
concurrency_queue_length = Gauge('embedding_concurrency_queue_length', 'Number of requests waiting for the concurrency limiter.')

async def get_model(model_name: str) -> SentenceTransformer:
    if model_name in model_cache:
        cache_hits.inc()
        model_cache.move_to_end(model_name)
        return model_cache[model_name]
    cache_misses.inc()
    async with model_load_lock:
        if model_name in model_cache:
            cache_hits.inc()
            model_cache.move_to_end(model_name)
            return model_cache[model_name]
        if len(model_cache) >= MAX_MODELS_IN_CACHE:
            lru_model_name, _ = model_cache.popitem(last=False)
            logger.info(f"Cache full. Evicting least recently used model: {lru_model_name}")
            cache_evictions.inc()
        logger.info(f"Loading new model '{model_name}' onto device '{DEVICE}'...")
        try:
            start_time = time.time()
            model = await run_in_threadpool(SentenceTransformer, model_name, device=DEVICE)
            load_time = time.time() - start_time
            model_load_duration.labels(model_name=model_name).observe(load_time)
            model_cache[model_name] = model
            logger.info(f"Model '{model_name}' loaded successfully in {load_time:.2f} seconds.")
            return model
        except Exception as e:
            logger.error(f"Failed to load model '{model_name}': {e}", exc_info=True)
            raise HTTPException(status_code=404, detail=f"Model '{model_name}' not found or could not be loaded.")

@router.get("/health", response_model=HealthResponse)
async def get_health():
    return HealthResponse(status="ok")

@router.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest):
    model_name = req.model_name or DEFAULT_MODEL_NAME
    concurrency_queue_length.inc()
    try:
        async with concurrency_limiter:
            concurrency_queue_length.dec()
            model = await get_model(model_name)
            embeddings = await run_in_threadpool(model.encode, req.texts, show_progress_bar=False)
            return EmbedResponse(embeddings=embeddings.tolist(), model_name=model_name)
    except Exception as e:
        logger.error(f"Embedding failed for model '{model_name}': {e}", exc_info=True)
        if isinstance(e, HTTPException):
            raise e
        raise HTTPException(status_code=500, detail=f"Embedding process failed: {e}")

@router.get("/model/dimension")
async def get_model_dimension(model_name: str | None = Query(None, description="Model name to query. If not set, uses default.")):
    model_name = model_name or DEFAULT_MODEL_NAME
    model = await get_model(model_name)
    return {"model_name": model_name, "dimension": model.get_sentence_embedding_dimension()}

@router.get("/model/context-window")
async def get_model_context_window(model_name: str | None = Query(None, description="Model name to query. If not set, uses default.")):
    model_name = model_name or DEFAULT_MODEL_NAME
    model = await get_model(model_name)
    return {"model_name": model_name, "context_window": getattr(model, 'max_seq_length', None)}

@router.get("/model/info")
async def get_model_info(model_name: str | None = Query(None, description="Model name to query. If not set, uses default.")):
    model_name = model_name or DEFAULT_MODEL_NAME
    model = await get_model(model_name)
    return {
        "model_name": model_name,
        "dimension": model.get_sentence_embedding_dimension(),
        "context_window": getattr(model, 'max_seq_length', None),
        "device": DEVICE
    }
