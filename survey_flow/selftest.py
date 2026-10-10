"""Offline self-test: proves the tool works without needing a real survey link.

Run from the CLI (`selftest`) or from the UI's "Check tool is working" button.
Each layer of the pipeline is verified, finishing with a full end-to-end run
against the embedded mock Confirmit-style survey (consent -> screener terminate
-> red-instruction question -> validation -> quota-full -> complete).
"""

from __future__ import annotations

import platform
import sys
import tempfile
import traceback
from dataclasses import dataclass, field


@dataclass
class Check:
    name: str
    ok: bool
    detail: str = ""


@dataclass
class SelfTestReport:
    checks: list = field(default_factory=list)

    @property
    def passed(self) -> bool:
        return bool(self.checks) and all(c.ok for c in self.checks)

    def add(self, name: str, ok: bool, detail: str = "") -> None:
        self.checks.append(Check(name=name, ok=bool(ok), detail=detail))


# --------------------------------------------------------------------------- #

def _check_runtime(report: SelfTestReport) -> None:
    report.add(
        "Python runtime",
        sys.version_info >= (3, 8),
        f"Python {platform.python_version()} on {platform.system()} {platform.machine()}",
    )


def _check_parser(report: SelfTestReport) -> None:
    from .analyzer import html_parser

    name = html_parser()
    report.add("HTML parser", name in ("lxml", "html.parser"), f"using {name!r}")


def _check_analyzer(report: SelfTestReport) -> None:
    from .analyzer import analyze
    from .mock_survey import PAGE_Q2

    page = analyze(PAGE_Q2, "http://127.0.0.1/survey")
    ok = len(page.questions) == 1 and bool(page.questions[0].choices)
    detail = f"{len(page.questions)} question(s) detected on the sample page"
    if ok:
        q = page.questions[0]
        opts = [f.label for f in q.choices]
        detail += f"; options: {opts}"
        ok = any((lab or "").strip().lower() == "yes" for lab in opts)
    report.add("Page analyzer", ok, detail)


def _check_instructions(report: SelfTestReport) -> None:
    from .instructions import parse_instruction

    hint = parse_instruction('Select: "Yes"')
    ok = hint.kind == "options" and any(t.lower() == "yes" for t in hint.targets)
    report.add(
        "Red-instruction rules",
        ok,
        f'`Select: "Yes"` -> kind={hint.kind}, targets={hint.targets}',
    )


def _check_memory(report: SelfTestReport) -> None:
    from .memory import TerminateMemory

    with tempfile.TemporaryDirectory() as td:
        path = f"{td}/mem.json"
        mem = TerminateMemory(path)
        mem.mark_end("QuestionDIV1", "Are you currently employed?",
                     [{"label": "No", "value": "1", "identity": "l:no"}],
                     status="terminate", message="no")
        mem.save()
        mem2 = TerminateMemory(path)
        ok = mem2.is_fatal("QuestionDIV1", "l:no", label="No")
        ok = ok and not mem2.is_fatal("QuestionDIV1", "l:yes", label="Yes")
        report.add("Terminate memory", ok,
                   "saved and re-loaded; fatal option remembered, safe option not")


def _check_end_to_end(report: SelfTestReport) -> None:
    """Full pipeline against the embedded mock survey - the real proof."""
    from .decider import Decider
    from .engines import create_engine
    from .memory import TerminateMemory
    from .mock_survey import start_mock_server
    from .runner import Runner, RunConfig

    server, url = start_mock_server()
    try:
        with tempfile.TemporaryDirectory() as td:
            mem_path = f"{td}/memory.json"
            trace_path = f"{td}/traces.jsonl"
            memory = TerminateMemory(mem_path)
            decider = Decider(memory, strategy="round-robin", seed=7)
            engine = create_engine("http")
            cfg = RunConfig(url=url, completes=2, max_attempts=20, delay=0.0,
                            trace_path=trace_path)
            try:
                code = Runner(cfg, memory, decider, engine).run()
            finally:
                engine.close()

            stats = memory.stats
            ok = (
                code == 0
                and stats.get("completes", 0) >= 2
                and memory.is_fatal("QuestionDIV1", "l:no", label="No")
                and memory.is_fatal("QuestionDIV4", "l:decline to answer",
                                    label="Decline to answer")
            )
            report.add(
                "End-to-end run (embedded mock survey)",
                ok,
                f"exit={code}; completes={stats.get('completes', 0)}/2, "
                f"terminates={stats.get('terminates', 0)}, "
                f"quota_full={stats.get('quota_full', 0)}; "
                f"screener terminate and quota-full selection were learned & avoided",
            )
    finally:
        server.shutdown()


# --------------------------------------------------------------------------- #

def run_selftest() -> SelfTestReport:
    import logging

    report = SelfTestReport()
    steps = (
        _check_runtime,
        _check_parser,
        _check_analyzer,
        _check_instructions,
        _check_memory,
        _check_end_to_end,
    )
    # keep the report readable: the end-to-end run logs a lot at INFO
    sf_log = logging.getLogger("survey_flow")
    old_level = sf_log.level
    sf_log.setLevel(logging.WARNING)
    try:
        for step in steps:
            try:
                step(report)
            except Exception as e:  # a crashed check is a failed check
                report.add(step.__name__.replace("_check_", "").replace("_", " "),
                           False, f"{type(e).__name__}: {e}")
                traceback.print_exc()
    finally:
        sf_log.setLevel(old_level)
    return report


def format_report(report: SelfTestReport) -> str:
    lines = []
    for c in report.checks:
        mark = "PASS" if c.ok else "FAIL"
        lines.append(f"[{mark}] {c.name}" + (f" - {c.detail}" if c.detail else ""))
    lines.append("")
    lines.append("SELF-TEST PASSED - the tool is working." if report.passed
                 else "SELF-TEST FAILED - see the failing checks above.")
    return "\n".join(lines)


def report_to_dict(report: SelfTestReport) -> dict:
    return {
        "ok": report.passed,
        "checks": [{"name": c.name, "ok": c.ok, "detail": c.detail}
                   for c in report.checks],
    }
