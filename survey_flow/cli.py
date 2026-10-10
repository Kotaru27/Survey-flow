"""Command-line interface for survey-flow."""

from __future__ import annotations

import argparse
import logging
import re
import sys

from .decider import STRATEGIES, Decider, Persona
from .engines import EngineUnavailable, create_engine
from .memory import DEFAULT_MEMORY_PATH, TerminateMemory
from .runner import Runner, RunConfig

log = logging.getLogger("survey_flow")

_TEST_URL_RE = re.compile(r"(^|[/._?&=\-])test(s|ing)?([/._?&=\-]|$)", re.I)


def looks_like_test_url(url: str) -> bool:
    return bool(_TEST_URL_RE.search(url or ""))


def _setup_logging(level: str, log_file: str | None) -> None:
    fmt = "%(levelname)-7s %(name)s: %(message)s"
    root = logging.getLogger()
    for h in list(root.handlers):          # avoid duplicates on repeated calls
        root.removeHandler(h)
    root.setLevel(getattr(logging, level.upper(), logging.INFO))
    sh = logging.StreamHandler()
    sh.setFormatter(logging.Formatter(fmt))
    root.addHandler(sh)
    if log_file:
        fh = logging.FileHandler(log_file, encoding="utf-8")
        fh.setFormatter(logging.Formatter("%(asctime)s %(levelname)-7s %(name)s: %(message)s"))
        root.addHandler(fh)


def _avoid_tuple(spec: str) -> tuple[str, ...]:
    parts = tuple(p.strip() for p in spec.split(",") if p.strip())
    for p in parts:
        if p not in ("terminate", "quota_full"):
            raise SystemExit(f"--avoid: unknown status {p!r} (use terminate and/or quota_full)")
    if not parts:
        raise SystemExit("--avoid: at least one status required")
    return parts


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="survey-flow",
        description="Rule-based (no AI) Confirmit/Forsta TEST-survey runner: "
                    "answers questions by analyzing the page, follows red bold "
                    "instructions, remembers selections that terminate and avoids them.",
    )
    p.add_argument("--log-level", default="INFO",
                   choices=["DEBUG", "INFO", "WARNING", "ERROR"])
    p.add_argument("--log-file", default=None, help="also log to this file")
    # allow the same flags AFTER the subcommand as well
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--log-level", default=argparse.SUPPRESS,
                        choices=["DEBUG", "INFO", "WARNING", "ERROR"])
    common.add_argument("--log-file", default=argparse.SUPPRESS, help="also log to this file")
    sub = p.add_subparsers(dest="command", required=True)

    run = sub.add_parser("run", parents=[common], help="run the survey until N completes")
    run.add_argument("url", help="Forsta/Confirmit test-link URL")
    run.add_argument("--completes", type=int, default=1, required=True,
                     help="how many completes you need")
    run.add_argument("--memory", default=DEFAULT_MEMORY_PATH,
                     help="terminate-memory file (default: %(default)s)")
    run.add_argument("--persona", default=None,
                     help="persona JSON with consistent test answers")
    run.add_argument("--strategy", default="random", choices=list(STRATEGIES),
                     help="how to answer when there is no red instruction "
                          "(options already known to terminate are always excluded)")
    run.add_argument("--engine", default="http", choices=["http", "browser"],
                     help="http = plain forms (default); browser = Playwright for JS pages")
    run.add_argument("--headed", action="store_true", help="browser engine: show the window")
    run.add_argument("--delay", type=float, default=0.3, help="pause between runs (seconds)")
    run.add_argument("--max-attempts", type=int, default=60,
                     help="max whole runs before giving up")
    run.add_argument("--max-pages", type=int, default=80, help="max pages per run")
    run.add_argument("--seed", type=int, default=None, help="seed the random strategy")
    run.add_argument("--avoid", default="terminate,quota_full",
                     help="comma statuses to remember & avoid (default: %(default)s)")
    run.add_argument("--trace", default="traces.jsonl",
                     help="JSONL file recording every run path ('' to disable)")
    run.add_argument("--allow-live", action="store_true",
                     help="permit a URL that does not look like a test link "
                          "(QA only - never field live surveys with this tool)")
    run.set_defaults(func=cmd_run)

    ana = sub.add_parser("analyze", parents=[common], help="fetch one page and print its structure")
    ana.add_argument("url")
    ana.add_argument("--engine", default="http", choices=["http", "browser"])
    ana.add_argument("--headed", action="store_true")
    ana.set_defaults(func=cmd_analyze)

    mem = sub.add_parser("memory", parents=[common], help="inspect or clear the terminate memory")
    mem.add_argument("action", nargs="?", default="show", choices=["show", "clear"])
    mem.add_argument("--memory", default=DEFAULT_MEMORY_PATH)
    mem.add_argument("--avoid", default="terminate,quota_full")
    mem.set_defaults(func=cmd_memory)
    return p


# --------------------------------------------------------------------------- #

def cmd_run(args) -> int:
    if not looks_like_test_url(args.url) and not args.allow_live:
        log.error(
            "refusing: %r does not look like a TEST link. This tool is for "
            "survey QA on test links only. If you are sure, re-run with --allow-live.",
            args.url,
        )
        return 3

    avoid = _avoid_tuple(args.avoid)
    memory = TerminateMemory(args.memory, avoid=avoid)
    persona = Persona.load(args.persona) if args.persona else Persona()
    decider = Decider(memory, persona=persona, strategy=args.strategy, seed=args.seed)

    try:
        engine = create_engine(args.engine, headed=args.headed)
    except EngineUnavailable as e:
        log.error("%s", e)
        return 3

    cfg = RunConfig(
        url=args.url,
        completes=args.completes,
        max_attempts=args.max_attempts,
        max_pages=args.max_pages,
        delay=args.delay,
        trace_path=args.trace or None,
    )
    runner = Runner(cfg, memory, decider, engine)
    log.info("target: %d completes | strategy=%s | engine=%s | memory=%s",
             args.completes, args.strategy, args.engine, args.memory)
    try:
        return runner.run()
    finally:
        engine.close()


def cmd_analyze(args) -> int:
    from .analyzer import page_report

    try:
        engine = create_engine(args.engine, headed=args.headed)
    except EngineUnavailable as e:
        log.error("%s", e)
        return 3
    try:
        session = engine.new_session()
        html, url = session.fetch(args.url)
        from .analyzer import analyze
        page = analyze(html, url)
        print(page_report(page))
        session.close()
        return 0
    except Exception as e:
        log.error("analyze failed: %s", e)
        return 2
    finally:
        engine.close()


def cmd_memory(args) -> int:
    avoid = _avoid_tuple(args.avoid)
    memory = TerminateMemory(args.memory, avoid=avoid)
    if args.action == "clear":
        memory.clear()
        print(f"cleared {args.memory}")
        return 0
    print(f"file: {args.memory}")
    for line in memory.summary_lines():
        print(line)
    return 0


def main(argv=None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    _setup_logging(args.log_level, args.log_file)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
