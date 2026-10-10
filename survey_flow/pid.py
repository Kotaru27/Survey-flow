"""Extract a project id (PID) from a survey test link, used to name the
per-project results folder (memory + traces live inside it).

No survey vendor is hardcoded away from what the URL itself carries: the
common Confirmit/Forsta-style `pid`/`projectid`/`surveyid` query parameters
and `/survey/<id>/` path patterns are checked first, then a long numeric id,
and only if nothing is found a stable hash of the link is used.
"""

from __future__ import annotations

import hashlib
import re
from urllib.parse import parse_qs, urlparse

# priority order (case-insensitive) for query parameter names
_PID_QUERY_KEYS = (
    "pid",
    "__pid",
    "projectid",
    "project_id",
    "project",
    "surveyid",
    "survey_id",
    "__surveyid",
    "studyid",
    "study_id",
)

# e.g. https://host/survey/12345/test  |  https://host/CF/project-77/start
_PATH_ID_RE = re.compile(
    r"/(?:survey|cf|project|study|s)/(?:test(?:ing)?/)?([A-Za-z0-9-]{2,64})(?:/|$)",
    re.I,
)

_LOOSE_ID_RE = re.compile(r"(?:^|[?&])(?:pid|id|s)=([A-Za-z0-9_-]{2,64})", re.I)
_DIGITS_RE = re.compile(r"\d{4,}")
_UNSAFE_RE = re.compile(r"[^A-Za-z0-9_-]+")


def extract_pid(url: str) -> str:
    """Best-effort project/PID from the link; always returns a safe folder name."""
    raw = (url or "").strip()
    if not raw:
        return "PID"

    parsed = urlparse(raw)
    query = parse_qs(parsed.query)

    for wanted in _PID_QUERY_KEYS:
        for key, values in query.items():
            if key.lower() == wanted:
                pid = _UNSAFE_RE.sub("", (values[0] if values else "").strip())
                if pid:
                    return pid[:64]

    m = _PATH_ID_RE.search(parsed.path or "")
    if m:
        return m.group(1)

    m = _LOOSE_ID_RE.search(raw)
    if m:
        return _UNSAFE_RE.sub("", m.group(1))[:64]

    digits = sorted(_DIGITS_RE.findall(raw), key=len, reverse=True)
    if digits:
        return digits[0][:64]

    return "PID-" + hashlib.sha1(raw.encode("utf-8")).hexdigest()[:8]
