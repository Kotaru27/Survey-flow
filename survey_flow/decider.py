"""Answer decisions: rule-based, no AI.

Priority per question:
  1. red instruction (explicitly tells the tester which answer avoids a terminate)
  2. persona (consistent test-scenario values, e.g. age/email)
  3. strategy among options NOT remembered as terminating (random/first/round-robin)

Validation messages are read back (per the QA guide) and used to correct the
answer on retry: "select at least 4" -> select 4, "between 18 and 25" -> pick
inside that range, etc.
"""

from __future__ import annotations

import json
import logging
import random
import re
from pathlib import Path
from typing import Optional

from .instructions import (
    CONSENT_RE,
    EXCLUSIVE_RE,
    Hint,
    match_targets_to_options,
    norm_label,
    parse_instruction,
)
from .memory import TerminateMemory
from .models import Field, Page, Question

log = logging.getLogger("survey_flow.decider")

STRATEGIES = ("random", "first", "round-robin")

_BETWEEN_RE = re.compile(r"between\s+(\d+(?:\.\d+)?)\s+and\s+(\d+(?:\.\d+)?)", re.I)
_TO_RANGE_RE = re.compile(r"\b(\d+(?:\.\d+)?)\s*(?:-|–|to)\s*(\d+(?:\.\d+)?)\b", re.I)
_AT_LEAST_RE = re.compile(r"at\s+least\s+(\d+)", re.I)
_MIN_LEN_RE = re.compile(r"minimum\s+(?:length\s+)?(?:of\s+)?(\d+)", re.I)


# --------------------------------------------------------------------------- #
# persona
# --------------------------------------------------------------------------- #

class Persona:
    def __init__(self, data: Optional[dict] = None):
        data = data or {}
        reserved = {"keywords", "text"}
        self.text = str(data.get("text", "Test response"))
        kw = dict(data.get("keywords") or {})
        for k, v in data.items():
            if k not in reserved:
                kw[k] = v
        self.keywords: dict[str, object] = kw

    @classmethod
    def load(cls, path: str) -> "Persona":
        p = Path(path)
        if not p.exists():
            raise SystemExit(f"persona file not found: {path}")
        return cls(json.loads(p.read_text(encoding="utf-8")))

    def match_question(self, qtext: str):
        """Return (keyword, value) for the first keyword found in the question text."""
        ql = norm_label(qtext)
        for k, v in self.keywords.items():
            nk = norm_label(str(k))
            if nk and nk in ql:
                return k, v
        return None, None


# --------------------------------------------------------------------------- #
# validation-message constraints
# --------------------------------------------------------------------------- #

def constraints_from_validations(msgs: list[str]) -> dict:
    out: dict = {}
    for m in msgs:
        match = _AT_LEAST_RE.search(m)
        if match:
            out["min_select"] = max(out.get("min_select", 0), int(match.group(1)))
        match = _BETWEEN_RE.search(m)
        if match:
            out["range"] = (float(match.group(1)), float(match.group(2)))
        match = _MIN_LEN_RE.search(m)
        if match:
            out["min_length"] = max(out.get("min_length", 0), int(match.group(1)))
        if re.search(r"too\s+short", m, re.I):
            out["min_length"] = max(out.get("min_length", 30))
    return out


# --------------------------------------------------------------------------- #
# decider
# --------------------------------------------------------------------------- #

class Decider:
    def __init__(
        self,
        memory: TerminateMemory,
        persona: Optional[Persona] = None,
        strategy: str = "random",
        seed: Optional[int] = None,
    ):
        if strategy not in STRATEGIES:
            raise ValueError(f"strategy must be one of {STRATEGIES}")
        self.memory = memory
        self.persona = persona or Persona()
        self.strategy = strategy
        self.rng = random.Random(seed)
        self.rr: dict[str, int] = {}   # round-robin counters (process-local)

    # ------------------------------------------------------------------ #
    def decide_page(self, page: Page, attempt: int = 0, validation_msgs: list[str] | None = None) -> None:
        validation_msgs = validation_msgs or []
        vcons = constraints_from_validations(validation_msgs)
        for q in page.questions:
            try:
                self.decide_question(q, attempt=attempt, vcons=vcons)
            except Exception:  # keep going: one odd question must not kill the run
                log.exception("decision failed for question %s", q.sig)

    # ------------------------------------------------------------------ #
    def decide_question(self, q: Question, attempt: int = 0, vcons: dict | None = None) -> None:
        vcons = vcons or {}
        # reset any previous decision state
        for f in q.fields:
            f.selected = False
            f.new_value = None

        self.memory.note_seen(q.sig, q.text)

        # apply a min-selection learned from an earlier validation message
        learned_min = self.memory.learned(q.sig, "min_select")
        if learned_min:
            q.min_select = max(q.min_select, int(learned_min))

        if q.kind == "constant_sum":
            self._decide_constant_sum(q)
        else:
            self._decide_choices(q, attempt, vcons)
            self._decide_scalars(q, attempt, vcons)

        self._decide_consent(q)

    # ------------------------------------------------------------------ #
    # instructions
    # ------------------------------------------------------------------ #
    def _instruction_targets(self, q: Question) -> tuple[list[Field], Optional[Hint]]:
        """Parse all red instructions; return matched option fields + first text/number hint."""
        option_pool = self._option_pool(q)
        matched: list[Field] = []
        seen: set[int] = set()
        scalar_hint: Optional[Hint] = None
        for text in q.instructions:
            hint = parse_instruction(text)
            if hint.kind == "none":
                continue
            if hint.kind == "text" and scalar_hint is None:
                scalar_hint = hint
                continue
            hits = match_targets_to_options(hint.targets, hint.numbers, option_pool, q.kind)
            if hits:
                for f in hits:
                    if id(f) not in seen:
                        seen.add(id(f))
                        matched.append(f)
            elif hint.kind == "options" and not scalar_hint and (
                hint.numbers or _looks_scalar_hint(hint)
            ):
                scalar_hint = hint
        return matched, scalar_hint

    # ------------------------------------------------------------------ #
    def _option_pool(self, q: Question) -> list[Field]:
        pool: list[Field] = []
        for f in q.fields:
            if f.type in ("radio", "checkbox"):
                pool.append(f)
            elif f.type == "select-one":
                pool.extend(o for o in f.options if not o.is_placeholder_option)
        return pool

    @staticmethod
    def _apply_selection(q: Question, fields: list[Field]) -> None:
        for f in fields:
            if f.type in ("radio", "checkbox"):
                f.selected = True
            elif f.type == "select-one":
                owner = _select_owner(f, q.fields)
                if owner is not None:
                    owner.new_value = f.value

    # ------------------------------------------------------------------ #
    # choices
    # ------------------------------------------------------------------ #
    def _decide_choices(self, q: Question, attempt: int, vcons: dict) -> None:
        if not q.is_choice:
            return

        pool = self._option_pool(q)
        if not pool:
            return

        matched, _scalar_hint = self._instruction_targets(q)

        # ---- rows (grid / ranking) ------------------------------------ #
        if q.kind in ("grid_single", "grid_multi", "ranking"):
            rows = _rows_of(q)
            if q.kind == "ranking":
                self._decide_ranking(q, rows, matched)
                return
            k_row = q.min_select
            if "min_select" in vcons:
                k_row = max(k_row, vcons["min_select"])
                self.memory.set_learned(q.sig, "min_select", vcons["min_select"])
            if attempt:
                k_row = k_row + attempt
            for row_name, row_fields in rows:
                row_matched = [f for f in matched if f.name == row_name]
                if row_matched:
                    # keep at most one per row for SA grids
                    if q.kind == "grid_single":
                        row_matched = row_matched[:1]
                    self._apply_selection(q, row_matched)
                    continue
                k = min(k_row, len(row_fields)) if q.kind == "grid_multi" else 1
                chosen = self._pick(q, row_fields, k, attempt, vcons, row_key=row_name)
                self._apply_selection(q, chosen)
            return

        # ---- single select (SA, dropdown, NPS, rating) ----------------- #
        if q.kind in ("single", "dropdown", "nps", "rating"):
            if matched:
                pick = matched[0]
                self._apply_selection(q, [pick])
                return
            # persona: match a persona value against the options
            kw, val = self.persona.match_question(q.text)
            if val is not None:
                for f in pool:
                    if _value_matches_label(str(val), f.label or f.value):
                        self._apply_selection(q, [f])
                        return
            chosen = self._pick(q, pool, 1, attempt, vcons)
            self._apply_selection(q, chosen)
            return

        # ---- multi select --------------------------------------------- #
        if q.kind == "multi":
            if matched:
                self._apply_selection(q, matched)
                return
            k = q.min_select
            if "min_select" in vcons:
                k = max(k, vcons["min_select"])
                self.memory.set_learned(q.sig, "min_select", vcons["min_select"])
            if attempt and "min_select" not in vcons:
                k += attempt
            if q.max_select:
                k = min(k, q.max_select)
            chosen = self._pick(q, pool, k, attempt, vcons)
            self._apply_selection(q, chosen)
            return

    def _decide_ranking(self, q: Question, rows: list, matched: list[Field]) -> None:
        n = len(rows)
        if n == 0:
            return
        rot = self.rr.get(q.sig, 0) % n
        self.rr[q.sig] = self.rr.get(q.sig, 0) + 1
        for i, (row_name, row_fields) in enumerate(rows):
            rank = ((i + rot) % n) + 1
            want = str(rank)
            target = next((f for f in row_fields if f.value == want), None)
            if target is None:
                target = next((f for f in row_fields if norm_label(f.label) == norm_label(want)), None)
            if target is None:
                target = row_fields[(rank - 1) % len(row_fields)]
            target.selected = True

    # ------------------------------------------------------------------ #
    # the core picker: exclude remembered-terminate options
    # ------------------------------------------------------------------ #
    def _pick(
        self,
        q: Question,
        candidates: list[Field],
        k: int,
        attempt: int,
        vcons: dict,
        row_key: str = "",
    ) -> list[Field]:
        if not candidates:
            return []
        k = max(1, k)

        fatal = [f for f in candidates if self._is_fatal(q, f)]
        safe = [f for f in candidates if f not in fatal]
        pool = safe if safe else candidates          # if everything is fatal, must pick anyway
        if fatal and safe:
            log.debug("q=%s avoiding fatal options: %s", q.sig,
                      [f.label for f in fatal])

        # respect exclusive options ("None of these", "Don't know", ...)
        non_excl = [f for f in pool if not f.exclusive]
        chosen: list[Field] = []

        if k > 1 and non_excl:
            pool_use = non_excl            # never mix an exclusive option into multi-selects
        elif non_excl and len(non_excl) < len(pool) and k == 1 and self.strategy == "random":
            # prefer substantive answers but keep exclusive as (rare) fallback
            pool_use = non_excl
        else:
            pool_use = pool

        if self.strategy == "first":
            chosen = pool_use[:k]
        elif self.strategy == "round-robin":
            key = f"{q.sig}|{row_key}"
            start = self.rr.get(key, 0) % max(1, len(pool_use))
            self.rr[key] = self.rr.get(key, 0) + 1
            chosen = [pool_use[(start + i) % len(pool_use)] for i in range(min(k, len(pool_use)))]
            # de-dup (k may wrap)
            seen: set[int] = set()
            chosen = [f for f in chosen if not (id(f) in seen or seen.add(id(f)))]
        else:  # random
            non_excl_use = [f for f in pool_use if not f.exclusive] or pool_use
            if k >= len(non_excl_use):
                chosen = list(non_excl_use)
            else:
                chosen = self.rng.sample(non_excl_use, k)
                # tiny chance to include an exclusive answer for data variety
                if k == 1 and len(non_excl_use) < len(pool_use) and self.rng.random() < 0.05:
                    chosen = [self.rng.choice(pool_use)]

        # if a lone exclusive answer got chosen together with others, keep exclusive only
        excl = [f for f in chosen if f.exclusive]
        if excl and len(chosen) > 1:
            if any(f.type in ("radio", "select-one") for f in chosen):
                chosen = excl[:1]
        if q.max_select:
            chosen = chosen[: q.max_select]
        return chosen

    def _is_fatal(self, q: Question, f: Field) -> bool:
        return self.memory.is_fatal(q.sig, f.identity, f.label, f.value)

    # ------------------------------------------------------------------ #
    # scalars (text / number / date / slider ...)
    # ------------------------------------------------------------------ #
    def _decide_scalars(self, q: Question, attempt: int, vcons: dict) -> None:
        if not q.scalars:
            return

        # instruction / persona lookup
        _matched, scalar_hint = self._instruction_targets(q)
        kw, val = self.persona.match_question(q.text)

        other_chosen = any(
            (f.selected or f.new_value) and _is_other(f) for f in q.fields
        )

        for f in q.scalars:
            # "Other - specify" companion box: only filled when Other is chosen
            if _is_other_box(f) and not other_chosen:
                continue

            value: Optional[str] = None

            # 1) instruction
            if scalar_hint is not None and scalar_hint.targets:
                value = str(scalar_hint.targets[0])
                if f.type in ("number", "range") or _looks_number(value):
                    try:
                        num = float(value)
                        value = str(int(num)) if num.is_integer() else str(num)
                    except ValueError:
                        value = None
            # 2) persona (keyword matched this question, or name hints)
            if value is None and val is not None and kw is not None:
                if norm_label(str(kw)) in norm_label(f.name + " " + f.label) or _question_matches_kw(q, kw):
                    value = str(val)
            if value is None:
                value = self._generate_scalar(f, q, attempt, vcons)

            value = _clamp(f, value)
            if value is not None:
                f.new_value = value

    def _generate_scalar(self, f: Field, q: Question, attempt: int, vcons: dict) -> str:
        if f.type == "number":
            lo, hi = _number_bounds(f, q, vcons)
            if lo is None and hi is None:
                return str(30 + attempt * 3)
            lo = 0 if lo is None else lo
            hi = lo if hi is None else hi
            if hi < lo:
                lo, hi = hi, lo
            step = f.step or ""
            if "." in step:
                val = round(self.rng.uniform(float(lo), float(hi)), 2)
                return str(val)
            return str(self.rng.randint(int(lo), int(hi)))
        if f.type == "range":
            lo = float(f.min) if f.min else 0.0
            hi = float(f.max) if f.max else 100.0
            mid = (lo + hi) / 2
            return _snap(mid, f)
        if f.type == "date":
            target = "1985-06-15"
            if f.min and target < f.min:
                target = f.min
            if f.max and target > f.max:
                target = f.max
            return target
        if f.type in ("time", "datetime-local"):
            return "12:00" if f.type == "time" else "2024-06-15T12:00"
        # text-ish
        base = self.persona.text
        if "min_length" in vcons and vcons["min_length"] > len(base):
            base = (base + " ") * 2
            while len(base) < vcons["min_length"]:
                base += base
        need = f.minlength or 0
        while len(base) < need:
            base += " " + self.persona.text
        if f.maxlength:
            base = base[: f.maxlength]
        return base

    # ------------------------------------------------------------------ #
    def _decide_constant_sum(self, q: Question) -> None:
        fields = [f for f in q.scalars if f.type in ("number", "text")]
        if not fields:
            return
        total = q.total if q.total is not None else 100
        n = len(fields)
        base = total // n if n else total
        rem = total - base * n if n else 0
        for i, f in enumerate(fields):
            v = base + (rem if i == 0 else 0)
            f.new_value = str(max(0, v))

    def _decide_consent(self, q: Question) -> None:
        boxes = [f for f in q.fields if f.type == "checkbox" and f.consent]
        others = [f for f in q.fields if f.type in ("radio", "select-one")
                  or (f.type == "checkbox" and not f.consent)]
        if boxes and not others:
            for f in boxes:
                f.selected = True


# --------------------------------------------------------------------------- #
# helpers
# --------------------------------------------------------------------------- #

def _rows_of(q: Question) -> list[tuple[str, list[Field]]]:
    rows: dict[str, list[Field]] = {}
    order: list[str] = []
    for f in q.fields:
        if f.type in ("radio", "checkbox"):
            if f.name not in rows:
                rows[f.name] = []
                order.append(f.name)
            rows[f.name].append(f)
    return [(n, rows[n]) for n in order]


def _select_owner(option_field: Field, pool: list[Field]) -> Optional[Field]:
    for f in pool:
        if f.type == "select-one" and any(o is option_field for o in f.options):
            return f
    return None


def _value_matches_label(value: str, label: str) -> bool:
    from .instructions import _phrase_in
    nv, nl = norm_label(value), norm_label(label)
    if not nv or not nl:
        return False
    if nv == nl:
        return True
    return _phrase_in(nl, nv) or (len(nl) >= 3 and _phrase_in(nv, nl))


def _is_other(f: Field) -> bool:
    from .instructions import OTHER_RE
    return bool(OTHER_RE.match((f.label or "").strip()))


def _is_other_box(f: Field) -> bool:
    name = (f.name or "").lower()
    label = norm_label(f.label or "")
    if "other" in name and f.type in ("text", "textarea"):
        return True
    if f.type in ("text", "textarea") and ("specify" in label or label == "other"):
        return True
    return False


def _question_matches_kw(q: Question, kw) -> bool:
    return norm_label(str(kw)) in norm_label(q.text)


def _looks_number(s: str) -> bool:
    try:
        float(s)
        return True
    except ValueError:
        return False


def _looks_scalar_hint(hint: Hint) -> bool:
    if not hint.targets:
        return False
    t = hint.targets[0]
    if _looks_number(t):
        return True
    verb = re.match(r"^(enter|type|input|write|fill)", hint.raw, re.I)
    return bool(verb)


def _number_bounds(f: Field, q: Question, vcons: dict) -> tuple[Optional[float], Optional[float]]:
    lo: Optional[float] = None
    hi: Optional[float] = None
    if f.min not in (None, ""):
        try:
            lo = float(f.min)
        except ValueError:
            lo = None
    if f.max not in (None, ""):
        try:
            hi = float(f.max)
        except ValueError:
            hi = None
    if "range" in vcons and lo is None and hi is None:
        return vcons["range"]
    m = _BETWEEN_RE.search(q.text)
    if m:
        return float(m.group(1)), float(m.group(2))
    m = _TO_RANGE_RE.search(q.text)
    if m and float(m.group(2)) > float(m.group(1)):
        return float(m.group(1)), float(m.group(2))
    if "range" in vcons:
        return vcons["range"]
    return lo, hi


def _snap(val: float, f: Field) -> str:
    try:
        step = float(f.step) if f.step else 0.0
    except ValueError:
        step = 0.0
    if step > 0:
        lo = float(f.min) if f.min else 0.0
        val = lo + round((val - lo) / step) * step
    if val.is_integer():
        return str(int(val))
    return str(val)


def _clamp(f: Field, value: Optional[str]) -> Optional[str]:
    if value is None:
        return None
    if f.type == "number" and f.min not in (None, "") and f.max not in (None, ""):
        try:
            v = float(value)
            v = max(float(f.min), min(float(f.max), v))
            return str(int(v)) if v.is_integer() else str(v)
        except ValueError:
            return value
    if f.maxlength and len(value) > f.maxlength:
        value = value[: f.maxlength]
    return value
