import os
import logging
from fastapi import Depends, HTTPException, status
from fastapi.security import APIKeyHeader

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

USER_API_KEY = os.getenv("USER_API_KEY", "")
ADMIN_API_KEY = os.getenv("ADMIN_API_KEY", "")
API_KEYS = {
    key: role
    for key, role in ((USER_API_KEY, "user"), (ADMIN_API_KEY, "admin"))
    if key
}
API_KEY_HEADER = APIKeyHeader(name="X-API-Key", auto_error=False)

def get_api_key(api_key: str = Depends(API_KEY_HEADER)):
    if not api_key or api_key not in API_KEYS:
        logger.warning("Unauthorized: missing or invalid API key")
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid or missing API Key",
        )
    return api_key

def get_admin_api_key(api_key: str = Depends(get_api_key)):
    if API_KEYS.get(api_key) != "admin":
        logger.warning("Forbidden: admin API key required")
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Admin privileges required",
        )
    return api_key
