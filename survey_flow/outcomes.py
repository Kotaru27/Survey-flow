"""Classify a page: question / validation / interstitial / complete / terminate / quota-full.

Pure text heuristics — no AI.  Detection order matters: an end page often still
says "Thank you", so terminate and quota signals are checked first.
"""

from __future__ import annotations

import re
from typing import Optional

QUOTA_PATTERNS = [
    re.compile(r"quota[^\n]{0,40}(full|reached|limit|exceeded|already\s*(been\s*)?filled)"),
    re.compile(r"(over|over[- ])?quota[- ]full"),
    re.compile(r"you\s+(have\s+)?(been\s+)?over\s*quota"),
    re.compile(r"the\s+quota\s+(for|cell)[^\n]{0,60}(full|reached)"),
    re.compile(r"too\s+many\s+(respondents|participants|answers|completions)"),
    re.compile(r"quota-?full|overquota"),
]

TERMINATE_PATTERNS = [
    # end-page phrasings (context required so question text like
    # "Have you ever been terminated?" never matches)
    re.compile(r"\b(survey|interview|session|screening|screen|respondent)[^\n]{0,40}terminat"),
    re.compile(r"\byou\s+have\s+been\s+terminat"),
    re.compile(r"\byou\s+(?:are|were|will\s+be)\s+terminat"),
    re.compile(r"\bwe\s+(?:must|have\s+to|need\s+to)\s+terminat"),
    re.compile(r"\bterminat(?:e|ed|ion)\s+(?:page|screen|status|message|point|here|now)"),
    re.compile(r"\bdisqualifi(?:ed|cation)\b"),
    re.compile(r"\bscreened\s+out\b"),
    re.compile(r"\byou\s+(?:are|were|have\s+been)\s+not\s+qualifi"),
    re.compile(r"\bnot\s+qualifi(?:ed|er)\b"),
    re.compile(r"\bdo(?:es)?\s+not\s+qualify\b"),
    re.compile(r"\b(do|does|did)\s+n[o']t\s+qualify\b"),
    re.compile(r"\bdoes\s+not\s+(meet|match)[^\n]{0,40}(criteria|requirements)"),
    re.compile(r"\bunfortunately[^\n]{0,120}\b(qualify|selected|invited|participat)"),
    re.compile(r"\bnot\s+(eligible|eligible\s+for|invited)\b"),
    re.compile(r"\bno\s+longer\s+(eligible|needed|required)\b"),
    re.compile(r"\bthank\s+you\s+for\s+your\s+interest[^\n]{0,80}(unable|cannot|unable to)"),
    re.compile(r"\bwe\s+(are\s+)?unable\s+to\s+(include|proceed|invite)[^\n]{0,60}"),
    re.compile(r"/terminates?\b"),
    re.compile(r"[?&/]term(inate)?[=/&]"),
]

COMPLETE_PATTERNS = [
    re.compile(r"survey\s+is\s+now\s+complete"),
    re.compile(r"\b(survey|interview|session)\s+(has\s+been\s+)?(completed|finished|ended)\b"),
    re.compile(r"thank\s+you[^\n]{0,100}\b(complet|participat|respond|finished|ended)"),
    re.compile(r"thank\s+you\s+for\s+(taking\s+part|your\s+participation|completing)"),
    re.compile(r"(your\s+)?(responses?|answers?|submission)\s+(has|have)\s+been\s+(recorded|saved|submitted)"),
    re.compile(r"\byou\s+have\s+(successfully\s+)?(completed|finished)\b"),
    re.compile(r"/(complete|completed|finished|thanks|thankyou)\b"),
    re.compile(r"[?&](end|finished|complete)=?(1|true)?\b"),
]

VALIDATION_PATTERNS = [
    re.compile(r"please\s+(answer|enter|select|provide|choose|fill|specify|type|input|check|pick)"),
    re.compile(r"\bis\s+required\b"),
    re.compile(r"\brequired\s+(field|answer|question)\b"),
    re.compile(r"(must|needs?\s+to)\s+be\s+(entered|provided|selected|a\s+(number|valid))"),
    re.compile(r"\binvalid\s+(value|input|number|entry|format|email|date|response)\b"),
    re.compile(r"\bselect\s+at\s+least\s+\d+"),
    re.compile(r"\bchoose\s+at\s+least\s+\d+"),
    re.compile(r"\bat\s+least\s+\d+\s+(answer|option|selection|item|response|choice)"),
    re.compile(r"\b(no\s+more\s+than|at\s+most|maximum\s+of|max\.?)\s+\d+"),
    re.compile(r"\bnumber\s+(must\s+be\s+)?between\b"),
    re.compile(r"\b(out\s+of\s+range|too\s+(short|long|small|large|high|low))\b"),
    re.compile(r"\bmust\s+(be\s+)?(a\s+)?(whole\s+)?number\b"),
    re.compile(r"\b(text\s+is\s+)?too\s+(long|short)\b"),
    re.compile(r"\b(valid|correct)\s+(email|phone|postcode|zip|format)\b"),
    re.compile(r"\b(rank|ranking)\s+(everything|all\s+items|each\s+item)\b"),
    re.compile(r"\bduplicate\s+rank"),
    re.compile(r"\b(total|sum)\s+(must|should)\s+(be|equal)\b"),
    re.compile(r"\bmissing\s+(an?\s+)?answer"),
    re.compile(r"\bno\s+answer\s+(selected|given|provided)\b"),
    re.compile(r"\b(answer|select|complete)\s+(the\s+)?(following|all|each|this)\s+(question|row|item)"),
    re.compile(r"\bplease\s+complete\b"),
]

# class/id hints for error boxes
ERROR_ELEM_RE = re.compile(r"(error|validation|errormessage|validationmessage|alert)", re.I)
ERROR_TEXT_RE = re.compile(
    r"(please\s+(answer|enter|select|provide|choose|fill|specify|type|check)|is\s+required|"
    r"invalid|at\s+least\s+\d+|too\s+(long|short)|must\s+be|out\s+of\s+range)",
    re.I,
)


def _search_any(patterns: list[re.Pattern], text: str) -> Optional[re.Match]:
    for p in patterns:
        m = p.search(text)
        if m:
            return m
    return None


def find_validation(text: str) -> list[str]:
    """Return validation messages found in `text` (trimmed, deduped)."""
    out: list[str] = []
    seen: set[str] = set()
    for line in text.splitlines():
        s = line.strip()
        if not s or len(s) > 300:
            continue
        if _search_any(VALIDATION_PATTERNS, s):
            key = s.lower()
            if key not in seen:
                seen.add(key)
                out.append(s)
    return out


def classify_text(text: str, url: str = "") -> tuple[str, list[str]]:
    """Classify visible text (plus URL) as an end status.

    Returns (status, [evidence messages]) where status is one of
    'quota_full' | 'terminate' | 'complete' | '' (not an end page).
    """
    low = " ".join(text.lower().split())
    ulow = url.lower()

    for pat in QUOTA_PATTERNS:
        m = pat.search(low) or pat.search(ulow)
        if m:
            return "quota_full", [_evidence(low, m)]

    for pat in TERMINATE_PATTERNS:
        m = pat.search(low) or pat.search(ulow)
        if m:
            return "terminate", [_evidence(low, m)]

    for pat in COMPLETE_PATTERNS:
        m = pat.search(low)
        if m:
            return "complete", [_evidence(low, m)]

    return "", []


def _evidence(low_text: str, m: re.Match) -> str:
    start = max(0, m.start() - 60)
    end = min(len(low_text), m.end() + 80)
    return low_text[start:end].strip(" .;:-")

