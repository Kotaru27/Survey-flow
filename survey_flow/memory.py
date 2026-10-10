"""Persistent memory of selections that ended a run (terminate / quota-full).

Stored as JSON so learning survives restarts.  Questions are identified by a
stable signature, options by their normalized label (option order can be
randomized between sessions, labels cannot).
"""

from __future__ import annotations

import json
import os
import tempfile
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

from .instructions import norm_label

DEFAULT_MEMORY_PATH = "survey_memory.json"
# statuses whose selections should be avoided on future runs
AVOIDABLE = ("terminate", "quota_full")


@dataclass
class QuestionMemory:
    text: str = ""
    seen: int = 0
    ends: dict = field(default_factory=dict)   # option identity -> record


class TerminateMemory:
    def __init__(self, path: str = DEFAULT_MEMORY_PATH, avoid: tuple[str, ...] = AVOIDABLE):
        self.path = Path(path)
        self.avoid = tuple(avoid)
        self.data: dict = {"version": 1, "questions": {}, "stats": self._new_stats()}
        self.load()

    # ------------------------------------------------------------------ #
    @staticmethod
    def _new_stats() -> dict:
        return {
            "runs": 0,
            "completes": 0,
            "terminates": 0,
            "quota_full": 0,
            "validations": 0,
            "stuck": 0,
            "last_run": None,
        }

    def load(self) -> None:
        if self.path.exists():
            try:
                loaded = json.loads(self.path.read_text(encoding="utf-8"))
                if isinstance(loaded, dict) and "questions" in loaded:
                    self.data = loaded
                    self.data.setdefault("stats", self._new_stats())
            except (json.JSONDecodeError, OSError):
                # keep a corrupt file aside instead of losing it
                try:
                    self.path.replace(self.path.with_suffix(".corrupt.json"))
                except OSError:
                    pass
                self.data = {"version": 1, "questions": {}, "stats": self._new_stats()}

    def save(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        payload = json.dumps(self.data, indent=2, ensure_ascii=False)
        # atomic write
        fd, tmp = tempfile.mkstemp(dir=str(self.path.parent), suffix=".tmp")
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                fh.write(payload)
            os.replace(tmp, self.path)
        finally:
            if os.path.exists(tmp):
                try:
                    os.remove(tmp)
                except OSError:
                    pass

    # ------------------------------------------------------------------ #
    def note_seen(self, sig: str, text: str = "") -> None:
        q = self.data["questions"].setdefault(sig, {"text": text, "seen": 0, "ends": {}})
        if text and not q.get("text"):
            q["text"] = text
        q["seen"] = int(q.get("seen", 0)) + 1

    def mark_end(
        self,
        sig: str,
        text: str,
        chosen: list[dict],
        status: str,
        url: str = "",
        message: str = "",
    ) -> list[str]:
        """Record the choices that coincided with a run-ending status.

        `chosen` is a list of {"label": ..., "value": ..., "identity": ...}.
        Returns the identities recorded.
        """
        q = self.data["questions"].setdefault(sig, {"text": text, "seen": 0, "ends": {}})
        if text:
            q["text"] = text
        recorded = []
        for c in chosen:
            ident = c.get("identity")
            if not ident:
                if c.get("label"):
                    ident = "l:" + norm_label(c["label"])
                elif c.get("value"):
                    ident = "v:" + c["value"]
                else:
                    continue
            rec = q["ends"].setdefault(ident, {
                "label": c.get("label", ""),
                "value": c.get("value", ""),
                "count": 0,
                "statuses": {},
            })
            rec["count"] = int(rec.get("count", 0)) + 1
            sts = rec.setdefault("statuses", {})
            sts[status] = int(sts.get(status, 0)) + 1
            rec["last_status"] = status
            rec["last_url"] = url
            rec["last_message"] = message[:300]
            rec["last_ts"] = time.strftime("%Y-%m-%dT%H:%M:%S")
            recorded.append(ident)
        stat_key = "terminates" if status == "terminate" else status
        stats = self.data.setdefault("stats", self._new_stats())
        stats[stat_key] = int(stats.get(stat_key, 0)) + 1
        return recorded

    # ------------------------------------------------------------------ #
    def is_fatal(self, sig: str, identity: str, label: str = "", value: str = "") -> bool:
        """Should future runs avoid this option for this question?"""
        q = self.data["questions"].get(sig)
        if not q:
            return False
        ends = q.get("ends", {})
        keys = [identity]
        if label:
            keys.append("l:" + norm_label(label))
        if value:
            keys.append("v:" + value)
        for k in keys:
            if k in ends:
                rec = ends[k]
                statuses = rec.get("statuses") or {}
                status = rec.get("last_status")
                if any(s in self.avoid for s in statuses) or (status in self.avoid):
                    return True
        return False

    def fatal_options(self, sig: str) -> dict:
        """identity -> record, restricted to avoid-worthy statuses."""
        q = self.data["questions"].get(sig)
        if not q:
            return {}
        out = {}
        for ident, rec in q.get("ends", {}).items():
            statuses = rec.get("statuses") or {}
            status = rec.get("last_status")
            if any(s in self.avoid for s in statuses) or (status in self.avoid):
                out[ident] = rec
        return out

    def all_fatal(self, sig: str) -> bool:
        """True when at least one avoidable end exists (per-question flag)."""
        return bool(self.fatal_options(sig))

    # ------------------------------------------------------------------ #
    def set_learned(self, sig: str, key: str, value) -> None:
        """Persist a lesson from a validation message (e.g. min_select=4)."""
        q = self.data["questions"].setdefault(sig, {"text": "", "seen": 0, "ends": {}})
        learned = q.setdefault("learned", {})
        learned[key] = value

    def learned(self, sig: str, key: str, default=None):
        q = self.data["questions"].get(sig) or {}
        return (q.get("learned") or {}).get(key, default)

    # ------------------------------------------------------------------ #
    def bump(self, key: str, n: int = 1) -> None:
        stats = self.data.setdefault("stats", self._new_stats())
        stats[key] = int(stats.get(key, 0)) + n

    @property
    def stats(self) -> dict:
        return self.data.setdefault("stats", self._new_stats())

    def summary_lines(self) -> list[str]:
        lines = []
        s = self.stats
        lines.append(
            f"runs={s.get('runs', 0)} completes={s.get('completes', 0)} "
            f"terminates={s.get('terminates', 0)} quota_full={s.get('quota_full', 0)} "
            f"validations={s.get('validations', 0)} stuck={s.get('stuck', 0)}"
        )
        qs = self.data.get("questions", {})
        if not qs:
            lines.append("no remembered terminate selections yet")
            return lines
        lines.append(f"{len(qs)} question(s) with remembered end selections:")
        for sig, q in qs.items():
            fatal = self.fatal_options(sig)
            if not fatal:
                continue
            text = (q.get("text") or sig)[:70]
            labels = ", ".join(
                (r.get("label") or r.get("value") or ident)
                for ident, r in fatal.items()
            )
            lines.append(f"  [{sig}] {text!r} -> avoid: {labels}")
        return lines

    def clear(self) -> None:
        self.data = {"version": 1, "questions": {}, "stats": self._new_stats()}
        self.save()
