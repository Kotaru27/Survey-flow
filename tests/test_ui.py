"""Tests for the graphical window's local web UI (no real browser needed)."""

import json
import threading
import time
import urllib.request
from http.server import ThreadingHTTPServer

import pytest

import survey_flow.ui as ui
from survey_flow.ui import JobManager, _Handler


@pytest.fixture()
def server(tmp_path, monkeypatch):
    monkeypatch.setattr(ui, "RESULTS_DIR", tmp_path / "results")
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
    httpd.daemon_threads = True
    httpd.manager = JobManager()
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    base = f"http://127.0.0.1:{httpd.server_address[1]}"
    yield base, httpd.manager
    httpd.shutdown()
    httpd.server_close()


def _get(url):
    with urllib.request.urlopen(url, timeout=10) as r:
        return r.status, r.read().decode("utf-8")


def _post(url, payload):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.status, json.loads(r.read().decode("utf-8"))


def _wait_done(base, job_id, timeout=30.0):
    after = 0
    lines = []
    deadline = time.time() + timeout
    while time.time() < deadline:
        status, body = _get(f"{base}/api/runs/{job_id}?after={after}")
        assert status == 200
        d = json.loads(body)
        after = d["next"]
        lines.extend(d["lines"])
        if d["status"] == "done":
            return d, lines
        time.sleep(0.05)
    raise AssertionError("job did not finish in time")


def test_main_page_has_the_form(server):
    base, _ = server
    status, body = _get(base + "/")
    assert status == 200
    assert "Survey test link" in body
    assert "Number of completes" in body
    assert "Open live run window (popup)" in body
    assert "Check tool is working" in body


def test_run_page_unknown_job_is_404(server):
    base, _ = server
    with pytest.raises(urllib.error.HTTPError) as e:
        _get(base + "/run/doesnotexist")
    assert e.value.code == 404


def test_background_run_against_mock_survey(server, mock_url):
    base, _ = server
    status, d = _post(base + "/api/runs",
                      {"url": mock_url, "completes": 2, "allow_live": False})
    assert status == 200 and d.get("id")

    final, lines = _wait_done(base, d["id"])
    assert final["code"] == 0, "\n".join(lines)
    assert final["stats"].get("completes") == 2
    assert final["completes_target"] == 2
    assert any("COMPLETE" in ln or "finished" in ln for ln in lines)

    # run window page exists while/after the job is known
    status, body = _get(f"{base}/run/{d['id']}")
    assert status == 200 and "Live test run" in body

    # incremental log polling returns only new lines
    status, body = _get(f"{base}/api/runs/{d['id']}?after={final['next']}")
    d2 = json.loads(body)
    assert d2["lines"] == []


def test_non_test_link_is_refused(server):
    base, _ = server
    status, d = _post(base + "/api/runs",
                      {"url": "https://example.com/live-survey",
                       "completes": 1, "allow_live": False})
    assert status == 200
    final, lines = _wait_done(base, d["id"])
    assert final["code"] == 3
    assert any("TEST link" in ln for ln in lines)


def test_selftest_endpoint(server):
    base, _ = server
    status, d = _post(base + "/api/selftest", {})
    assert status == 200
    assert d["ok"] is True
    assert len(d["checks"]) >= 5


def test_results_listing(server, mock_url):
    base, _ = server
    status, d = _post(base + "/api/runs", {"url": mock_url, "completes": 1})
    _wait_done(base, d["id"])
    status, body = _get(base + "/api/results")
    d = json.loads(body)
    names = [f["name"] for f in d["files"]]
    assert "memory.json" in names
    assert "traces.jsonl" in names
    assert "status.txt" in names
    assert any(n.startswith("run_") for n in names)


def test_only_one_run_at_a_time(tmp_path, monkeypatch):
    monkeypatch.setattr(ui, "RESULTS_DIR", tmp_path / "results")

    def slow(job):
        time.sleep(0.6)
        return 0, {}

    monkeypatch.setattr(ui, "_execute_run", slow)
    mgr = JobManager()
    mgr.start({"url": "http://x", "completes": 1})
    with pytest.raises(RuntimeError):
        mgr.start({"url": "http://y", "completes": 1})
    time.sleep(0.8)
    job = mgr.start({"url": "http://z", "completes": 1})
    assert job.status == "running"


def test_quit_endpoint(server):
    base, _ = server
    status, d = _post(base + "/api/quit", {})
    assert status == 200 and d["ok"] is True
    time.sleep(0.5)
    with pytest.raises((urllib.error.URLError, ConnectionError, OSError)):
        _get(base + "/")
