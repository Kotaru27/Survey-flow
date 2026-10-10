"""The run loop: start a fresh session, walk pages, answer, remember ends,
until the requested number of completes is reached."""

from __future__ import annotations

import json
import logging
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

from .analyzer import analyze
from .decider import Decider, Persona
from .engines import Engine, EngineError, NoSubmitError
from .memory import TerminateMemory
from .models import Outcome, Page, Question

log = logging.getLogger("survey_flow.runner")


@dataclass
class RunConfig:
    url: str
    completes: int = 1
    max_attempts: int = 60        # whole runs (sessions)
    max_pages: int = 80           # pages per run
    max_same_page: int = 5        # repeats of one page before declaring stuck
    delay: float = 0.3            # pause between runs
    trace_path: Optional[str] = "traces.jsonl"
    consecutive_error_limit: int = 3


@dataclass
class RunResult:
    status: str                   # complete | terminate | quota_full | stuck | error
    message: str = ""
    answers: list = field(default_factory=list)
    pages: int = 0


class Runner:
    def __init__(
        self,
        cfg: RunConfig,
        memory: TerminateMemory,
        decider: Decider,
        engine: Engine,
    ):
        self.cfg = cfg
        self.memory = memory
        self.decider = decider
        self.engine = engine

    # ------------------------------------------------------------------ #
    def run(self) -> int:
        cfg = self.cfg
        completes = 0
        attempts = 0
        errors = 0
        t0 = time.time()

        while completes < cfg.completes and attempts < cfg.max_attempts:
            attempts += 1
            self.memory.bump("runs")
            try:
                result = self._run_once(attempts)
            except (EngineError, NoSubmitError) as e:
                errors += 1
                log.error("attempt %d failed: %s", attempts, e)
                result = RunResult(status="error", message=str(e))
                self.memory.bump("stuck")
            except Exception as e:  # network hiccup, parse bug, ...
                errors += 1
                log.error("attempt %d crashed: %s", attempts, e, exc_info=True)
                result = RunResult(status="error", message=str(e))
                self.memory.bump("stuck")

            if result.status == "complete":
                completes += 1
                errors = 0
                self.memory.bump("completes")
                log.info("attempt %d -> COMPLETE (%d/%d) %s",
                         attempts, completes, cfg.completes, _short(result.answers))
            elif result.status in ("terminate", "quota_full"):
                errors = 0
                verb = "TERMINATE" if result.status == "terminate" else "QUOTA FULL"
                log.warning("attempt %d -> %s %s %s",
                            attempts, verb, _short(result.answers),
                            f"[{result.message}]" if result.message else "")
            elif result.status == "stuck":
                log.error("attempt %d -> STUCK %s", attempts, _short(result.answers))
            else:
                log.error("attempt %d -> ERROR %s", result.message)

            self.memory.save()
            if errors >= cfg.consecutive_error_limit:
                log.error("aborting after %d consecutive errors", errors)
                break
            if completes < cfg.completes and attempts < cfg.max_attempts:
                if cfg.delay:
                    time.sleep(cfg.delay)

        elapsed = time.time() - t0
        self._summary(completes, attempts, elapsed)
        return 0 if completes >= cfg.completes else 2

    # ------------------------------------------------------------------ #
    def _run_once(self, attempt_no: int) -> RunResult:
        cfg = self.cfg
        session = self.engine.new_session()
        try:
            html, url = session.fetch(cfg.url)
            prev_sig: Optional[str] = None
            same_count = 0
            just_submitted = False
            last_answered: list[tuple[Question, list]] = []
            unknown_clicks = 0
            answers: list[str] = []
            pages = 0

            for _ in range(cfg.max_pages):
                pages += 1
                page = analyze(html, url)

                outcome = page.outcome
                if outcome == Outcome.VALIDATION and not just_submitted:
                    outcome = Outcome.QUESTION   # stale/first-load message: keep going

                sig = page.signature
                if sig == prev_sig:
                    same_count += 1
                else:
                    same_count = 0
                    prev_sig = sig

                # ---------------- end states ---------------- #
                if outcome in (Outcome.COMPLETE, Outcome.TERMINATE, Outcome.QUOTA_FULL):
                    status = outcome.value
                    message = page.messages[0] if page.messages else ""
                    if status in ("terminate", "quota_full") and last_answered:
                        self._remember(last_answered, status, url, message)
                    self._trace(attempt_no, status, message, answers, pages)
                    return RunResult(status=status, message=message,
                                     answers=answers, pages=pages)

                # ---------------- interstitial ---------------- #
                if outcome == Outcome.INTERSTITIAL:
                    if same_count > cfg.max_same_page:
                        self._trace(attempt_no, "stuck", "interstitial loop", answers, pages)
                        return RunResult(status="stuck", message="interstitial loop",
                                         answers=answers, pages=pages)
                    html, url = session.submit(page)
                    just_submitted = True
                    continue

                # ---------------- unknown ---------------- #
                if outcome == Outcome.UNKNOWN:
                    unknown_clicks += 1
                    if unknown_clicks > 8 or not page.submits:
                        self._trace(attempt_no, "stuck", "unrecognized page", answers, pages)
                        return RunResult(status="stuck", message="unrecognized page",
                                         answers=answers, pages=pages)
                    html, url = session.submit(page)
                    just_submitted = True
                    continue

                # ---------------- question / validation ---------------- #
                if same_count > cfg.max_same_page:
                    msg = "; ".join(page.messages[:3])
                    self._trace(attempt_no, "stuck", msg or "same page repeated",
                                answers, pages)
                    if just_submitted:
                        self.memory.bump("stuck")
                    return RunResult(status="stuck", message=msg or "same page repeated",
                                     answers=answers, pages=pages)

                if outcome == Outcome.VALIDATION and just_submitted:
                    self.memory.bump("validations")
                    log.info("validation on page %d: %s", pages,
                             "; ".join(page.messages[:3]))

                vmsgs = page.messages if just_submitted else page.messages
                self.decider.decide_page(page, attempt=same_count, validation_msgs=vmsgs)

                answered = []
                for q in page.questions:
                    entries = _chosen_entries(q)
                    if entries:
                        answered.append((q, entries))
                        label = ", ".join(t for t in q.value_texts)
                        log.debug("page %d q[%s] -> %s", pages, q.sig, label)
                if answered:
                    last_answered = answered
                    for _q, entries in answered:
                        answers.append(
                            f"{_q.sig}=" + "/".join(e["label"] or e["value"] for e in entries)
                        )

                html, url = session.submit(page)
                just_submitted = True

            self._trace(attempt_no, "stuck", "max pages reached", answers, pages)
            self.memory.bump("stuck")
            return RunResult(status="stuck", message="max pages reached",
                             answers=answers, pages=pages)
        finally:
            session.close()

    # ------------------------------------------------------------------ #
    def _remember(self, last_answered, status: str, url: str, message: str) -> None:
        for q, entries in last_answered:
            recorded = self.memory.mark_end(q.sig, q.text, entries, status, url, message)
            if recorded:
                log.info("memory: %s on %r -> avoid %s", status, q.text[:60],
                         [e.get("label") or e.get("value") for e in entries])

    def _trace(self, attempt: int, status: str, message: str, answers: list, pages: int) -> None:
        if not self.cfg.trace_path:
            return
        rec = {
            "ts": time.strftime("%Y-%m-%dT%H:%M:%S"),
            "attempt": attempt,
            "status": status,
            "message": message[:300],
            "pages": pages,
            "answers": answers,
        }
        try:
            with open(self.cfg.trace_path, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(rec, ensure_ascii=False) + "\n")
        except OSError:
            pass

    def _summary(self, completes: int, attempts: int, elapsed: float) -> None:
        s = self.memory.stats
        log.info("-" * 60)
        log.info("finished: %d/%d completes in %d attempts (%.1fs)",
                 completes, self.cfg.completes, attempts, elapsed)
        for line in self.memory.summary_lines():
            log.info("memory: %s", line)
        if self.memory.path.exists():
            log.info("memory file: %s", self.memory.path)


def _chosen_entries(q: Question) -> list[dict]:
    out = []
    for f in q.chosen_options:
        out.append({"label": f.label, "value": f.value, "identity": f.identity})
    return out


def _short(answers: list) -> str:
    if not answers:
        return ""
    joined = " | ".join(answers[-6:])
    return ("path: " + joined) if len(joined) <= 200 else ("path: " + joined[:197] + "...")
