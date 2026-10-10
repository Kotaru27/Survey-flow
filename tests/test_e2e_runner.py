"""End-to-end: full runner against the mock Confirmit-style survey.

Round-robin strategy makes the sequence deterministic:
  run 1: hits "No" on the screener      -> TERMINATE  (memory learns)
  run 2: avoids "No", instruction saves Q2, MA validation is corrected,
         hits "Decline to answer"       -> QUOTA FULL (memory learns)
  run 3+: avoids both                   -> COMPLETE
"""

import json

from survey_flow.decider import Decider
from survey_flow.engines import create_engine
from survey_flow.memory import TerminateMemory
from survey_flow.runner import Runner, RunConfig


def test_reaches_completes_and_learns(mock_url, tmp_path):
    mem_path = tmp_path / "memory.json"
    trace_path = tmp_path / "traces.jsonl"

    memory = TerminateMemory(str(mem_path))
    decider = Decider(memory, strategy="round-robin", seed=7)
    engine = create_engine("http")
    cfg = RunConfig(
        url=mock_url,
        completes=3,
        max_attempts=20,
        delay=0.0,
        trace_path=str(trace_path),
    )
    try:
        code = Runner(cfg, memory, decider, engine).run()
    finally:
        engine.close()

    assert code == 0, "target completes not reached"
    stats = memory.stats
    assert stats["completes"] == 3
    assert stats["terminates"] >= 1, "expected to hit the screener terminate once"
    assert stats["quota_full"] >= 1, "expected to hit the quota-full end once"

    # --- learned to avoid the terminating / over-quota selections ------- #
    assert memory.is_fatal("QuestionDIV1", "l:no", label="No"), "screener 'No' not remembered"
    assert memory.is_fatal("QuestionDIV4", "l:decline to answer",
                           label="Decline to answer"), "quota selection not remembered"

    # --- red instruction was followed: Q2 never terminated -------------- #
    q2 = memory.data["questions"].get("QuestionDIV2")
    assert not (q2 and q2.get("ends")), "Q2 should never end a run (instruction followed)"

    # --- memory file persisted on disk ---------------------------------- #
    on_disk = json.loads(mem_path.read_text())
    assert "QuestionDIV1" in on_disk["questions"]

    # --- traces written -------------------------------------------------- #
    traces = [json.loads(line) for line in trace_path.read_text().splitlines() if line.strip()]
    statuses = [t["status"] for t in traces]
    assert "complete" in statuses
    assert statuses.count("complete") == 3


def test_second_process_avoids_learned_terminates(mock_url, tmp_path):
    """Memory persists: a fresh process (fresh memory counters) avoids known ends."""
    mem_path = tmp_path / "memory.json"

    # process 1: learn
    memory = TerminateMemory(str(mem_path))
    decider = Decider(memory, strategy="round-robin", seed=1)
    engine = create_engine("http")
    cfg = RunConfig(url=mock_url, completes=1, max_attempts=10, delay=0.0, trace_path=None)
    try:
        assert Runner(cfg, memory, decider, engine).run() == 0
    finally:
        engine.close()

    # process 2: fresh runner instance, same memory file
    memory2 = TerminateMemory(str(mem_path))
    decider2 = Decider(memory2, strategy="round-robin", seed=2)
    engine2 = create_engine("http")
    try:
        code = Runner(cfg, memory2, decider2, engine2).run()
    finally:
        engine2.close()

    assert code == 0
    # the fresh process should avoid 'No' from the very first run
    assert memory2.is_fatal("QuestionDIV1", "l:no", label="No")


def test_random_strategy_with_seed_reaches_complete(mock_url, tmp_path):
    memory = TerminateMemory(str(tmp_path / "m.json"))
    decider = Decider(memory, strategy="random", seed=42)
    engine = create_engine("http")
    cfg = RunConfig(url=mock_url, completes=2, max_attempts=40, delay=0.0, trace_path=None)
    try:
        code = Runner(cfg, memory, decider, engine).run()
    finally:
        engine.close()
    assert code == 0
    assert memory.stats["completes"] == 2
