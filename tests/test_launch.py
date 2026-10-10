"""End-to-end test of the Testlink.bat entry point (`survey_flow launch`):
creates <folder>/<PID>/ with memory file, traces, run log and status.txt,
learns to avoid screen-outs, and reaches the requested completes.
"""

import json
from pathlib import Path

from survey_flow import cli
from survey_flow.pid import extract_pid


def test_launch_nopop_creates_pid_folder_and_memory(mock_url, tmp_path):
    url = mock_url + "?pid=QA42"
    folder = tmp_path / "results"

    code = cli.main([
        "launch", url,
        "--completes", "2",
        "--mode", "nopop",
        "--folder", str(folder),
    ])
    assert code == 0

    outdir = folder / extract_pid(url)
    assert outdir.is_dir()
    assert (outdir / "memory.json").is_file()
    assert (outdir / "traces.jsonl").is_file()
    assert (outdir / "status.txt").is_file()
    assert list(outdir.glob("run_*.log"))

    memory = json.loads((outdir / "memory.json").read_text())
    assert memory["stats"]["completes"] >= 2
    # memory exists to avoid screen-outs / validation errors on later runs
    assert memory["questions"], "expected learned question memory"

    status = (outdir / "status.txt").read_text()
    assert "completes" in status and "memory.json" in status


def test_launch_memory_file_is_reused_across_runs(mock_url, tmp_path):
    url = mock_url + "?pid=QA77"
    folder = tmp_path / "results"

    args = ["launch", url, "--completes", "1", "--mode", "nopop",
            "--folder", str(folder)]
    assert cli.main(args) == 0
    outdir = folder / extract_pid(url)
    first = json.loads((outdir / "memory.json").read_text())
    learned_before = set(first["questions"])

    assert cli.main(args) == 0
    second = json.loads((outdir / "memory.json").read_text())
    assert set(second["questions"]) >= learned_before, \
        "memory must persist and grow between runs"
    assert second["stats"]["runs"] > first["stats"]["runs"]


def test_launch_refuses_non_test_link_without_override(tmp_path):
    code = cli.main([
        "launch", "https://example.com/live-survey?pid=X1",
        "--completes", "1", "--mode", "nopop",
        "--folder", str(tmp_path),
    ])
    assert code == 3


def test_launch_unreachable_link_logs_errors_cleanly(tmp_path):
    """Connection problems must be logged (regression: the ERROR log line
    used to crash on a bad %d format) and recorded in status.txt."""
    url = "https://127.0.0.1:9/testing/x?pid=DEAD1"
    code = cli.main([
        "launch", url,
        "--completes", "1", "--mode", "nopop",
        "--folder", str(tmp_path), "--allow-live",
    ])
    assert code != 0
    outdir = tmp_path / "DEAD1"
    status = (outdir / "status.txt").read_text()
    assert "completes" in status
    logs = "".join(p.read_text() for p in outdir.glob("run_*.log"))
    assert "attempt 1 -> ERROR" in logs
    assert "Logging error" not in logs
