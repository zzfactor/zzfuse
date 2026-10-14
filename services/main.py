import logging
from contextlib import asynccontextmanager
from fastapi import FastAPI, Depends
from chroma_api import router as chroma_router
from sentencer_api import router as sentencer_router
from prometheus_fastapi_instrumentator import Instrumentator
from deps import get_api_key

# Set up logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.chroma_clients = {}
    try:
        yield
    finally:
        logger.info("Application shutdown: Cleaning up ChromaDB client cache.")
        app.state.chroma_clients.clear()


app = FastAPI(
    title="zzfuse Embedding & Chroma Service",
    description="Unified API for sentence embeddings and Chroma collection management.",
    version="1.0.0",
    lifespan=lifespan,
)

Instrumentator().instrument(app).expose(app)

# Include routers with appropriate dependencies
# Sentencer router now requires API key validation
app.include_router(sentencer_router, dependencies=[Depends(get_api_key)])
# All chroma endpoints require a valid API key
app.include_router(
    chroma_router,
    dependencies=[Depends(get_api_key)]
)
