"""HMAC-SHA256 master token validator for inter-node authentication."""
import hashlib
import hmac
import base64
import time
import os
from fastapi import Request, HTTPException


MASTER_TOKEN_HEADER = "X-Master-Token"
_SECRET = os.getenv("HMAC_SECRET", "change-me-in-production").encode()
_TTL = int(os.getenv("TOKEN_TTL_SECONDS", "30"))


def validate_master_token(token: str) -> bool:
    """Returns True if token is valid, not expired, and HMAC matches."""
    try:
        ts_str, mac_b64 = token.split(".", 1)
    except ValueError:
        return False

    try:
        ts = int(ts_str)
    except ValueError:
        return False

    now = int(time.time())
    if now - ts > _TTL or ts - now > 5:
        return False

    try:
        got = base64.urlsafe_b64decode(mac_b64 + "==")
    except Exception:
        return False

    expected = hmac.new(_SECRET, ts_str.encode(), hashlib.sha256).digest()
    return hmac.compare_digest(got, expected)


def require_master_token(request: Request):
    """FastAPI dependency — raises 403 if master token is absent or invalid."""
    token = request.headers.get(MASTER_TOKEN_HEADER, "")
    if not token or not validate_master_token(token):
        raise HTTPException(status_code=403, detail="invalid or missing master token")