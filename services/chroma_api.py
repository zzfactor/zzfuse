from fastapi import (
    APIRouter, HTTPException, Query, Request, Depends, status, UploadFile, File
)
from pydantic import BaseModel, Field
import chromadb
from urllib.parse import urlparse
import logging
from deps import get_admin_api_key
from typing import Optional

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/chroma", tags=["Chroma"])

# --- Dependency: Chroma Client Management ---
async def get_chroma_client(
    chroma_url: str = Query(..., description="Chroma server base URL (e.g., http://localhost:8010)"),
    request: Request = None
) -> chromadb.HttpClient:
    if request is None or not hasattr(request.app.state, "chroma_clients"):
        logger.error("App state or chroma_clients cache not initialized.")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Server configuration error: Chroma client cache not available."
        )
    try:
        parsed = urlparse(chroma_url)
        host, port = parsed.hostname, parsed.port
    except ValueError:
        host, port = None, None
    if not host or not port or parsed.scheme not in ("http", "https"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid Chroma URL. Provide both host and port (e.g., http://localhost:8010)."
        )
    key = f"{parsed.scheme}://{host}:{port}"
    if key not in request.app.state.chroma_clients:
        try:
            client = chromadb.HttpClient(host=host, port=port, ssl=parsed.scheme == "https")
            client.heartbeat()
            request.app.state.chroma_clients[key] = client
            logger.info(f"Connected to ChromaDB at {key}")
        except Exception as e:
            logger.error(f"Failed to connect to ChromaDB at {key}: {e}", exc_info=True)
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Failed to connect to ChromaDB at {key}: {e}"
            )
    return request.app.state.chroma_clients[key]

# --- Pydantic Models ---
class CreateCollectionRequest(BaseModel):
    name: str
    metadata: dict = Field(default_factory=dict)

class ModifyCollectionRequest(BaseModel):
    name: str
    new_name: Optional[str] = None
    metadata: Optional[dict] = None

class AddDocumentsRequest(BaseModel):
    collection: str
    documents: list[str]
    ids: list[str]
    metadatas: list[dict] = Field(default_factory=list)
    embeddings: list[list[float]] | None = None

class QueryRequest(BaseModel):
    collection: str
    query_texts: Optional[list[str]] = None
    query_embeddings: Optional[list[list[float]]] = None
    n_results: int = 5
    where: Optional[dict] = None
    where_document: Optional[dict] = None
    include: Optional[list[str]] = None

class UpdateDocumentsRequest(BaseModel):
    collection: str
    ids: list[str]
    documents: list[str] = Field(default_factory=list)
    metadatas: list[dict] = Field(default_factory=list)
    embeddings: list[list[float]] | None = None

class DeleteDocumentsRequest(BaseModel):
    collection: str
    ids: list[str] = Field(default_factory=list)
    where: dict = Field(default_factory=dict)
    where_document: dict = Field(default_factory=dict)

class CloneCollectionRequest(BaseModel):
    source: str
    target: str
    metadata: dict = Field(default_factory=dict)

# --- Endpoints ---

@router.post("/create-collection")
async def create_collection(
    req: CreateCollectionRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        metadata = req.metadata or {"created_by": "api"}
        col = client.get_or_create_collection(req.name, metadata=metadata)
        logger.info(f"Collection '{col.name}' created or exists.")
        return {"status": "created_or_exists", "collection": col.name, "metadata": col.metadata}
    except Exception as e:
        logger.error(f"Failed to create collection '{req.name}': {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"Failed to create collection: {e}")

@router.get("/list-collections")
async def list_collections(client: chromadb.HttpClient = Depends(get_chroma_client)):
    try:
        collections = client.list_collections()
        logger.info(f"Listed {len(collections)} collections.")
        return {"collections": [c.name for c in collections]}
    except Exception as e:
        logger.error(f"Failed to list collections: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"Failed to list collections: {e}")

@router.delete("/delete-collection", dependencies=[Depends(get_admin_api_key)])
async def delete_collection(
    name: str = Query(..., description="Collection name to delete."),
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        client.delete_collection(name)
        logger.info(f"Collection '{name}' deleted.")
        return {"status": "deleted", "collection": name}
    except Exception as e:
        logger.error(f"Failed to delete collection '{name}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{name}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to delete collection: {e}")

@router.get("/get-collection")
async def get_collection(
    name: str = Query(..., description="Collection name to get."),
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(name)
        logger.info(f"Retrieved collection '{name}'.")
        return {"name": col.name, "metadata": col.metadata}
    except Exception as e:
        logger.error(f"Failed to get collection '{name}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{name}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to get collection: {e}")

@router.post("/modify-collection")
async def modify_collection(
    req: ModifyCollectionRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(req.name)
        if req.new_name and req.metadata:
            col.modify(name=req.new_name, metadata=req.metadata)
        elif req.new_name:
            col.modify(name=req.new_name)
        elif req.metadata:
            col.modify(metadata=req.metadata)
        else:
            return {"status": "no changes requested"}
        logger.info(f"Collection '{req.name}' modified. New name: {req.new_name or req.name}")
        return {"status": "modified", "collection": req.new_name or req.name}
    except Exception as e:
        logger.error(f"Failed to modify collection '{req.name}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{req.name}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to modify collection: {e}")

@router.post("/add-documents")
async def add_documents(
    req: AddDocumentsRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(req.collection)
        col.add(
            documents=req.documents,
            ids=req.ids,
            metadatas=req.metadatas or None,
            embeddings=req.embeddings or None
        )
        logger.info(f"Added {len(req.ids)} documents to collection '{req.collection}'.")
        return {"status": "added", "count": len(req.ids)}
    except Exception as e:
        logger.error(f"Failed to add documents to collection '{req.collection}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{req.collection}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to add documents: {e}")

@router.post("/query")
async def query_collection(
    req: QueryRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(req.collection)
        results = col.query(
            query_texts=req.query_texts or None,
            query_embeddings=req.query_embeddings or None,
            n_results=req.n_results,
            where=req.where or None,
            where_document=req.where_document or None,
            include=req.include or ["documents", "metadatas", "embeddings"]
        )
        logger.info(f"Queried collection '{req.collection}', found {len(results.get('ids', [[]])[0])} results.")
        return results
    except Exception as e:
        logger.error(f"Failed to query collection '{req.collection}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{req.collection}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to query collection: {e}")

@router.post("/update-documents")
async def update_documents(
    req: UpdateDocumentsRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(req.collection)
        col.update(
            ids=req.ids,
            documents=req.documents or None,
            metadatas=req.metadatas or None,
            embeddings=req.embeddings or None
        )
        logger.info(f"Updated {len(req.ids)} documents in collection '{req.collection}'.")
        return {"status": "updated", "count": len(req.ids)}
    except Exception as e:
        logger.error(f"Failed to update documents in collection '{req.collection}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{req.collection}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to update documents: {e}")

@router.delete("/delete-documents", dependencies=[Depends(get_admin_api_key)])
async def delete_documents(
    collection: str = Query(..., description="Collection name to delete from."),
    ids: list[str] = Query([], description="List of document IDs to delete. If empty, uses 'where' clause."),
    where: str = Query("{}", description="JSON string filter for documents to delete."),
    where_document: str = Query("{}", description="JSON string filter for documents based on content to delete."),
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        where_dict = json.loads(where)
        where_document_dict = json.loads(where_document)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Invalid JSON for 'where' or 'where_document': {e}")
    if not ids and not where_dict and not where_document_dict:
        raise HTTPException(
            status_code=400,
            detail="Either 'ids', 'where', or 'where_document' must be provided for deletion."
        )
    try:
        col = client.get_collection(collection)
        deleted_count = 0
        if ids:
            col.delete(ids=ids)
            deleted_count = len(ids)
        elif where_dict or where_document_dict:
            col.delete(where=where_dict or None, where_document=where_document_dict or None)
            deleted_count = "filtered"
        logger.info(f"Deleted {deleted_count} documents from collection '{collection}'.")
        return {"status": "deleted", "count": deleted_count}
    except Exception as e:
        logger.error(f"Failed to delete documents from collection '{collection}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{collection}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to delete documents: {e}")

@router.get("/collection-stats")
async def collection_stats(
    name: str = Query(..., description="Collection name to get stats for."),
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(name)
        count = col.count()
        logger.info(f"Stats for collection '{name}': count={count}")
        return {
            "name": col.name,
            "metadata": col.metadata,
            "count": count,
            "dimension": getattr(col, "dimension", None)
        }
    except Exception as e:
        logger.error(f"Failed to get collection stats for '{name}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{name}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to get collection stats: {e}")

@router.get("/list-collection-metadata")
async def list_collection_metadata(
    name: str = Query(..., description="Collection name to list metadata for."),
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        col = client.get_collection(name)
        logger.info(f"Metadata for collection '{name}' retrieved.")
        return {"metadata": col.metadata}
    except Exception as e:
        logger.error(f"Failed to list collection metadata for '{name}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Collection '{name}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to list collection metadata: {e}")

@router.get("/database-info")
async def database_info(client: chromadb.HttpClient = Depends(get_chroma_client)):
    try:
        heartbeat = client.heartbeat()
        return {"info": "Limited info available via client.", "heartbeat": heartbeat}
    except Exception as e:
        logger.error(f"Failed to get database info: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"Failed to get database info: {e}")

@router.get("/health")
async def health_check(client: chromadb.HttpClient = Depends(get_chroma_client)):
    try:
        client.heartbeat()
        logger.info("ChromaDB health check successful.")
        return {"status": "ok"}
    except Exception as e:
        logger.error(f"Chroma health check failed: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"Chroma health check failed: {e}")

@router.post("/clone-collection")
async def clone_collection(
    req: CloneCollectionRequest,
    client: chromadb.HttpClient = Depends(get_chroma_client)
):
    try:
        source_col = client.get_collection(req.source)
        target_col = client.get_or_create_collection(req.target, metadata=req.metadata)
        batch = source_col.get(include=["documents", "metadatas", "embeddings", "ids"])
        target_col.add(
            documents=batch["documents"],
            ids=batch["ids"],
            metadatas=batch["metadatas"],
            embeddings=batch["embeddings"]
        )
        logger.info(f"Cloned collection '{req.source}' to '{req.target}'. Added {len(batch['ids'])} documents.")
        return {"status": "cloned", "source": req.source, "target": req.target}
    except Exception as e:
        logger.error(f"Failed to clone collection from '{req.source}' to '{req.target}': {e}", exc_info=True)
        if "not found" in str(e).lower():
            raise HTTPException(status_code=404, detail=f"Source collection '{req.source}' not found.")
        raise HTTPException(status_code=500, detail=f"Failed to clone collection: {e}")

# --- Not implemented: snapshot/restore/rebuild/tune endpoints ---
@router.post("/snapshot-collection")
async def snapshot_collection(
    name: str = Query(..., description="Collection name to snapshot."),
    chroma_url: str = Query(..., description="Chroma server base URL (e.g. http://localhost:8010)"),
    output_path: str = Query("/tmp/chroma_snapshot.sqlite3")
):
    logger.warning("Snapshot functionality not directly exposed by chromadb client.")
    return {"info": "Not implemented: chromadb client does not expose snapshot/export directly."}

@router.post("/restore-collection")
async def restore_collection(
    file: UploadFile = File(...),
    chroma_url: str = Query(..., description="Chroma server base URL (e.g. http://localhost:8010)")
):
    logger.warning("Restore functionality not directly exposed by chromadb client.")
    return {"info": "Not implemented: chromadb client does not expose restore/import directly."}

@router.post("/rebuild-index")
async def rebuild_index(
    name: str = Query(...),
    chroma_url: str = Query(..., description="Chroma server base URL (e.g. http://localhost:8010)")
):
    logger.warning("Rebuild index functionality not directly exposed by chromadb client.")
    return {"info": "Not implemented: chromadb client does not expose index rebuild directly."}

@router.post("/tune-index-parameters")
async def tune_index_parameters(
    name: str = Query(...),
    chroma_url: str = Query(..., description="Chroma server base URL (e.g. http://localhost:8010)")
):
    logger.warning("Tune index parameters functionality not directly exposed by chromadb client.")
    return {"info": "Not implemented: chromadb client does not expose index tuning directly."}
