"""Tests for the offline self-test (`survey_flow selftest` / UI button)."""

from survey_flow import cli
from survey_flow.selftest import format_report, report_to_dict, run_selftest


def test_selftest_passes_overall():
    report = run_selftest()
    assert report.passed, format_report(report)


def test_selftest_covers_every_layer():
    report = run_selftest()
    names = [c.name for c in report.checks]
    assert any("runtime" in n.lower() for n in names)
    assert any("parser" in n.lower() for n in names)
    assert any("analy" in n.lower() for n in names)
    assert any("instruction" in n.lower() for n in names)
    assert any("memory" in n.lower() for n in names)
    assert any("end-to-end" in n.lower() for n in names)


def test_report_helpers():
    report = run_selftest()
    text = format_report(report)
    assert "SELF-TEST PASSED" in text
    assert "[PASS]" in text
    d = report_to_dict(report)
    assert d["ok"] is True
    assert isinstance(d["checks"], list) and d["checks"]
    for c in d["checks"]:
        assert set(c) == {"name", "ok", "detail"}


def test_cli_selftest_exit_code():
    assert cli.main(["selftest"]) == 0
