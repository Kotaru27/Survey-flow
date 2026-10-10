"""Red/bold instruction detection and parsing (rule-based, no AI).

Two responsibilities:
  1. detect_red_bold()  - is this element a red (optionally bold) instruction?
  2. parse_instruction() - turn instruction text into an answer hint, then
     match the hint against the question's actual options.
"""

from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass, field
from difflib import SequenceMatcher
from typing import Optional

# --------------------------------------------------------------------------- #
# label helpers
# --------------------------------------------------------------------------- #

_PUNCT_RE = re.compile(r"[^\w\s%$€£+/&.'-]", re.UNICODE)
_WS_RE = re.compile(r"\s+")


def norm_label(s: str) -> str:
    """Normalize a label for fuzzy comparison."""
    s = unicodedata.normalize("NFKC", s or "")
    s = s.casefold()
    s = s.replace("'", "").replace("\u2019", "")   # don't -> dont
    s = _PUNCT_RE.sub(" ", s)
    s = _WS_RE.sub(" ", s).strip()
    return s


def _phrase_in(haystack: str, needle: str) -> bool:
    """Word-boundary phrase containment on normalized strings."""
    if not haystack or not needle:
        return False
    return re.search(r"(?<!\w)" + re.escape(needle) + r"(?!\w)", haystack) is not None


# special answer options -----------------------------------------------------
EXCLUSIVE_RE = re.compile(
    r"(none\s+of\s+(these|them|those|the\s+above)|"
    r"do\s+n[o']t\s+know|don'?t\s+know|dont\s+know|"
    r"not\s+sure|no\s+opinion|no\s+answer|no\s+response|"
    r"not\s+applicable|n/?a\b|"
    r"prefer\s+not\s+to\s+(answer|say|disclose|state|tell)|"
    r"rather\s+not\s+(answer|say)|refus(?:e|ed|al)|"
    r"i\s+have\s+not\s+(used|seen|purchased|heard)|"
    r"have\s+n[o']t\s+used|nothing|neither)",
    re.I,
)

OTHER_RE = re.compile(r"^(other|none of these above|other\s*\(.*\)|.*\bspecify\b.*|other[:.\s-].*)$", re.I)

CONSENT_RE = re.compile(
    r"\b(i\s+(agree|consent)|agree\s+to|consent|accept(s|ed)?\s+(the\s+)?(terms|policy)|"
    r"yes[,\s-]+i\s+(agree|accept))\b",
    re.I,
)

# validation messages must never be treated as answer instructions
VALIDATIONISH_RE = re.compile(
    r"(please\s+(answer|enter|select|provide|choose|fill|specify|type|check)|"
    r"is\s+required|invalid|at\s+least\s+\d+|too\s+(long|short)|must\s+be|"
    r"out\s+of\s+range|no\s+answer\s+(selected|given))",
    re.I,
)

PLACEHOLDER_OPTION_RE = re.compile(
    r"^(\s*-{2,}\s*.*|select\s+(one|an?\s+option|please).*|please\s+select.*|"
    r"choose\s+.*|\[.*\]|--.*)$",
    re.I,
)


# --------------------------------------------------------------------------- #
# red / bold detection (works off inline styles, font tags, classes - no JS)
# --------------------------------------------------------------------------- #

_COLOR_NAMES = {
    "red": (255, 0, 0),
    "crimson": (220, 20, 60),
    "firebrick": (178, 34, 34),
    "darkred": (139, 0, 0),
    "tomato": (255, 99, 71),
    "indianred": (205, 92, 92),
}


def _parse_color(value: str) -> Optional[tuple[int, int, int]]:
    value = (value or "").strip().lower()
    if not value:
        return None
    if value in _COLOR_NAMES:
        return _COLOR_NAMES[value]
    if value.startswith("#"):
        h = value[1:]
        if len(h) == 3:
            h = "".join(c * 2 for c in h)
        if len(h) == 6:
            try:
                return int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16)
            except ValueError:
                return None
    m = re.match(r"rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})", value)
    if m:
        return int(m.group(1)), int(m.group(2)), int(m.group(3))
    return None


def _is_red(rgb: tuple[int, int, int]) -> bool:
    r, g, b = rgb
    return r >= 150 and g <= 100 and b <= 100 and (r - max(g, b)) >= 70


def _style_of(tag) -> str:
    return (tag.get("style") or "").replace(" ", "").lower()


def _color_from_style(style: str) -> Optional[tuple[int, int, int]]:
    m = re.search(r"(?:^|;)color:([^;]+)", style)
    if m:
        return _parse_color(m.group(1))
    return None


def _bold_from_style(style: str) -> bool:
    m = re.search(r"(?:^|;)font-weight:([^;]+)", style)
    if not m:
        return False
    v = m.group(1).strip().lower()
    if v in ("bold", "bolder"):
        return True
    try:
        return int(v) >= 600
    except ValueError:
        return False


def element_color_bold(tag, stop_tag=None) -> tuple[bool, bool]:
    """Walk `tag` and its ancestors (up to stop_tag) for red & bold signals.

    Returns (is_red, is_bold).  A `<span style="color:red"><b>text</b></span>`
    is red (from the span) and bold (from the inner b) - handled by checking
    the full ancestor chain of the *text* element.
    """
    red = bold = False
    node = tag
    depth = 0
    while node is not None and node is not getattr(stop_tag, "name", None) and depth < 6:
        name = getattr(node, "name", None)
        style = _style_of(node) if hasattr(node, "get") else ""
        color = _color_from_style(style)
        if color and _is_red(color):
            red = True
        if name in ("b", "strong") or _bold_from_style(style):
            bold = True
        # <font color="red">
        if name == "font" and _parse_color(node.get("color") or "") is not None:
            if _is_red(_parse_color(node.get("color") or "")):
                red = True
        # class tokens
        classes = " ".join(node.get("class") or []) if hasattr(node, "get") else ""
        lowc = classes.lower()
        if "red" in lowc and ("bold" in lowc or "instruction" in lowc):
            red = True
        if "bold" in lowc:
            bold = True
        parent = getattr(node, "parent", None)
        node = parent
        depth += 1
        if stop_tag is not None and node is stop_tag:
            break
    return red, bold


def detect_red_bold(tag, stop_tag=None) -> tuple[bool, bool]:
    """Is this element itself a red/bold instruction carrier?

    Checks the element's own text (not deep children) so nested markup like
    `<span style="color:#f00"><b>text</b></span>` reports red+bold when asked
    about the span, and red+bold when asked about the b (via ancestors).
    """
    return element_color_bold(tag, stop_tag)


# --------------------------------------------------------------------------- #
# instruction parsing
# --------------------------------------------------------------------------- #

@dataclass
class Hint:
    kind: str = "options"             # options | text | number | none
    targets: list[str] = field(default_factory=list)
    numbers: list[float] = field(default_factory=list)
    raw: str = ""


_QUOTE_RE = re.compile(r"""['"“”„`]\s*([^'"“”„`]{1,80}?)\s*['"“”„`]""")
_VERB_RE = re.compile(
    r"\b(?:select|choose|pick|click(?:\s+on)?|answer|mark|tap|opt\s+for|go\s+with|"
    r"enter|type|input|fill\s+in|write|set)\b",
    re.I,
)
_VERB_TAIL_RE = re.compile(
    r"\b(?:(?:select|choose|pick|click(?:\s+on)?|answer|mark|tap|opt\s+for|go\s+with|"
    r"enter(?:\s+a\s+number|\s+an?\s+age)?|type|input|fill\s+in|write|set)\b)"
    r"(?:\s+(?:the|an?\s+one|exactly\s+one|one|all|each|only))?"
    r"\s*(?:option|answer|button|response|value|choice|item)?\s*"
    r"(?:[:\-\u2013\u2014]\s*|\s+)?(.+?)(?:[.!?,;]|$)",
    re.I,
)
_COLON_SPLIT_RE = re.compile(r"[:\u2013\u2014]\s*")
_PART_SPLIT_RE = re.compile(r"\s*(?:,|;|\band\b|\bor\b|&|\balso\b)\s*", re.I)
_NUMBER_RE = re.compile(r"^-?\d+(?:\.\d+)?$")

# verbs that mean "type text" rather than "pick an option"
_TEXT_VERBS_RE = re.compile(r"\b(enter|type|input|write|fill\s+in)\b", re.I)
# phrases that are guidance, not an answer target
_GUIDANCE_RE = re.compile(
    r"\b(avoid|do\s+n[o']t|don'?t|never|make\s+sure|remember|be\s+sure|"
    r"answer\s+(honestly|truthfully)|pick\s+(one|any)\b)",
    re.I,
)


def parse_instruction(text: str) -> Hint:
    """Turn an instruction sentence into a Hint (targets to match against options)."""
    raw = " ".join((text or "").split())
    if not raw:
        return Hint(kind="none")

    hint = Hint(raw=raw)

    # explicit text entry:  enter: "some text"  /  type: abc
    if _TEXT_VERBS_RE.search(raw) and _COLON_SPLIT_RE.search(raw):
        after = _COLON_SPLIT_RE.split(raw, 1)[1].strip()
        after = after.strip("'\"“”")
        if after:
            hint.kind = "text"
            hint.targets = [after]
            return hint

    candidates: list[str] = []
    quoted = [q.strip() for q in _QUOTE_RE.findall(raw)]
    candidates.extend(quoted)

    parts = _COLON_SPLIT_RE.split(raw, 1)
    if len(parts) == 2 and parts[1].strip():
        candidates.append(parts[1].strip())

    m = _VERB_TAIL_RE.search(raw)
    if m and m.group(1):
        candidates.append(m.group(1).strip())

    # whole string if short and looks like an answer, not guidance
    if not candidates and len(raw) <= 60 and not _GUIDANCE_RE.search(raw) and not _VERB_RE.search(raw):
        candidates.append(raw)

    # expand multi-targets
    expanded: list[str] = []
    for c in candidates:
        c = c.strip(" .;:!?")
        for piece in _PART_SPLIT_RE.split(c):
            piece = piece.strip(" .;:!?'\"“”")
            if piece:
                expanded.append(piece)

    # numbers present?
    for piece in expanded:
        if _NUMBER_RE.match(piece):
            try:
                hint.numbers.append(float(piece))
            except ValueError:
                pass

    hint.targets = expanded
    if not expanded and hint.numbers:
        hint.kind = "number"
    elif expanded:
        hint.kind = "options"
    else:
        hint.kind = "none"
    return hint


def match_targets_to_options(targets: list[str], numbers: list[float],
                             option_fields: list, question_kind: str) -> list:
    """Return the option fields matched by hint targets/numbers.

    Matching: exact normalized > substring (either way) > fuzzy ratio >= 0.80.
    Numbers match an option whose label equals the number, or its position
    (1-based) for ordinal scales / NPS.
    """
    matched: list = []
    seen: set[int] = set()

    def add(f) -> None:
        if id(f) not in seen:
            seen.add(id(f))
            matched.append(f)

    # normalize option labels once
    opts = [(f, norm_label(f.label or f.value)) for f in option_fields]

    for t in targets:
        nt = norm_label(t)
        if not nt:
            continue
        # exact
        exact = [f for f, no in opts if no == nt]
        if exact:
            for f in exact:
                add(f)
            continue
        # phrase containment either way (word-boundary: "male" must NOT
        # match inside "female")
        subs = [f for f, no in opts
                if len(nt) >= 3 and (_phrase_in(no, nt) or (len(no) >= 3 and _phrase_in(nt, no)))]
        if subs:
            for f in subs[:4]:
                add(f)
            continue
        # fuzzy (strict: "male"/"female" scores exactly 0.8)
        best, best_ratio = None, 0.0
        for f, no in opts:
            r = SequenceMatcher(None, nt, no).ratio()
            if r > best_ratio:
                best, best_ratio = f, r
        if best is not None and best_ratio >= 0.86:
            add(best)

    # numeric targets: label equals number, or 1-based position (NPS/ordinal)
    for n in hintless_number_iter(numbers):
        want = str(int(n)) if float(n).is_integer() else str(n)
        label_hits = [f for f, no in opts if no == norm_label(want)]
        if label_hits:
            for f in label_hits:
                add(f)
            continue
        idx = int(n) if n >= 0 else -1
        if question_kind in ("single", "grid_single", "nps", "rating", "ranking", "dropdown") \
                and 1 <= idx <= len(opts):
            add(opts[idx - 1][0])

    return matched


def hintless_number_iter(numbers: list[float]):
    return numbers


def labels_match(a: str, b: str) -> bool:
    na, nb = norm_label(a), norm_label(b)
    if not na or not nb:
        return False
    if na == nb:
        return True
    if len(na) >= 3 and (_phrase_in(nb, na) or _phrase_in(na, nb)):
        return True
    return SequenceMatcher(None, na, nb).ratio() >= 0.86
