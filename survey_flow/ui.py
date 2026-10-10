"""Graphical window for survey-flow: paste a link, set the number of completes,
run with a live popup window - or in the background.

Serves a small local page (stdlib http.server only, no extra packages) and
auto-opens it in the default browser. The page talks to this process over
relative URLs on the same local server.

* "Open live run window (popup)" ON  -> a popup window streams the run log.
* OFF                                 -> the run happens in the background and
                                          a message pops up when it finishes.

Everything (memory, traces, run logs) is written to the results folder shown
at the bottom of the window (default: ~/SurveyFlow-results).
"""

from __future__ import annotations

import json
import logging
import os
import platform
import subprocess
import sys
import threading
import time
import uuid
import webbrowser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from . import __version__
from .cli import looks_like_test_url

log = logging.getLogger("survey_flow.ui")

RESULTS_DIR = Path(
    os.environ.get("SURVEY_FLOW_HOME") or (Path.home() / "SurveyFlow-results")
)

# --------------------------------------------------------------------------- #
# run jobs
# --------------------------------------------------------------------------- #


class Job:
    """One survey test run executing in a background thread."""

    def __init__(self, params: dict):
        self.id = uuid.uuid4().hex[:12]
        self.params = params
        self.status = "running"          # running | done
        self.code: int | None = None
        self.error: str | None = None
        self.stats: dict = {}
        self.log_path: Path | None = None
        self._lines: list[str] = []
        self._lock = threading.Lock()

    def append(self, line: str) -> None:
        with self._lock:
            self._lines.append(line)

    def lines_after(self, index: int) -> tuple[list[str], int]:
        with self._lock:
            return list(self._lines[index:]), len(self._lines)


class _JobLogHandler(logging.Handler):
    def __init__(self, job: Job):
        super().__init__()
        self.job = job
        self.setFormatter(logging.Formatter("%(levelname)-7s %(message)s"))

    def emit(self, record: logging.LogRecord) -> None:
        try:
            self.job.append(self.format(record))
        except Exception:
            pass


class JobManager:
    def __init__(self) -> None:
        self._jobs: dict[str, Job] = {}
        self._active: Job | None = None
        self._lock = threading.Lock()

    @property
    def active(self) -> Job | None:
        with self._lock:
            if self._active and self._active.status == "running":
                return self._active
            return None

    def start(self, params: dict) -> Job:
        with self._lock:
            if self._active and self._active.status == "running":
                raise RuntimeError("a run is already in progress")
            job = Job(params)
            self._jobs[job.id] = job
            self._active = job
        thread = threading.Thread(target=self._run, args=(job,), daemon=True)
        thread.start()
        return job

    def run_blocking(self, params: dict) -> Job:
        """Run one job in the current thread (used by `launch --mode nopop`)."""
        with self._lock:
            job = Job(params)
            self._jobs[job.id] = job
            self._active = job
        self._run(job)
        return job

    def get(self, job_id: str) -> Job | None:
        return self._jobs.get(job_id)

    # ------------------------------------------------------------------ #
    def _run(self, job: Job) -> None:
        root = logging.getLogger()
        old_handlers = list(root.handlers)
        old_level = root.level
        for h in old_handlers:
            root.removeHandler(h)
        root.setLevel(logging.INFO)

        results_dir = Path(
            job.params.get("results_dir") or RESULTS_DIR)
        results_dir.mkdir(parents=True, exist_ok=True)
        stamp = time.strftime("%Y%m%d-%H%M%S")
        job.log_path = results_dir / f"run_{stamp}.log"
        fh = logging.FileHandler(job.log_path, encoding="utf-8")
        fh.setFormatter(logging.Formatter(
            "%(asctime)s %(levelname)-7s %(name)s: %(message)s"))
        root.addHandler(fh)
        jh = _JobLogHandler(job)
        root.addHandler(jh)

        try:
            code, stats = _execute_run(job)
            job.code = code
            job.stats = stats
            summary = (f"finished: {stats.get('completes', 0)}"
                       f"/{job.params.get('completes')} completes "
                       f"(exit code {code})")
            job.append(f"--- {summary} ---")
            log.info("job %s %s", job.id, summary)
        except Exception as e:  # never let the thread die silently
            job.error = f"{type(e).__name__}: {e}"
            job.code = 3
            job.append(f"ERROR: {job.error}")
            log.exception("job %s crashed", job.id)
        finally:
            _write_status(results_dir, job)
            job.status = "done"
            for h in list(root.handlers):
                root.removeHandler(h)
            for h in old_handlers:
                root.addHandler(h)
            root.setLevel(old_level)


def _write_status(results_dir: Path, job: Job) -> None:
    """Write a human-readable status.txt next to the memory/trace files."""
    p = job.params
    stats = job.stats or {}
    done = job.code == 0 and not job.error
    lines = [
        f"survey link      : {p.get('url', '')}",
        f"target           : {p.get('completes')} completes",
        f"mode             : {p.get('mode', '') or 'background'}",
        f"result           : {'DONE - target reached' if done else (job.error or f'gave up (exit code {job.code})')}",
        f"memory totals    : runs={stats.get('runs', 0)} "
        f"completes={stats.get('completes', 0)} "
        f"terminates={stats.get('terminates', 0)} "
        f"quota_full={stats.get('quota_full', 0)} "
        f"validations={stats.get('validations', 0)}",
        f"memory file      : {results_dir / 'memory.json'}",
        f"traces           : {results_dir / 'traces.jsonl'}",
        f"run log          : {job.log_path or ''}",
        f"updated          : {time.strftime('%Y-%m-%d %H:%M:%S')}",
    ]
    try:
        (results_dir / "status.txt").write_text("\n".join(lines) + "\n",
                                                encoding="utf-8")
    except Exception as e:
        log.warning("could not write status.txt: %s", e)


def _execute_run(job: Job):
    """Run the standard pipeline; returns (exit_code, memory_stats)."""
    from .decider import Decider, Persona
    from .engines import create_engine
    from .memory import TerminateMemory
    from .runner import Runner, RunConfig

    p = job.params
    url = (p.get("url") or "").strip()
    completes = int(p.get("completes") or 1)
    allow_live = bool(p.get("allow_live"))

    if not url:
        return 3, {}
    if not url.lower().startswith(("http://", "https://")):
        return 3, {}
    if not looks_like_test_url(url) and not allow_live:
        job.append(
            "ERROR: this does not look like a TEST link. Tick "
            "'allow live link' only for approved QA use.")
        return 3, {}

    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    results_dir = Path(p.get("results_dir") or RESULTS_DIR)
    results_dir.mkdir(parents=True, exist_ok=True)
    memory = TerminateMemory(str(results_dir / "memory.json"))
    persona = Persona.load(p["persona"]) if p.get("persona") else Persona()
    decider = Decider(memory, persona=persona,
                      strategy=p.get("strategy") or "random",
                      seed=p.get("seed"))
    engine = create_engine("http")
    cfg = RunConfig(
        url=url,
        completes=completes,
        max_attempts=int(p.get("max_attempts") or 60),
        max_pages=int(p.get("max_pages") or 80),
        delay=float(p.get("delay") or 0.3),
        trace_path=str(results_dir / "traces.jsonl"),
    )
    job.append(f"target: {completes} completes | url: {url}")
    runner = Runner(cfg, memory, decider, engine)
    try:
        code = runner.run()
    finally:
        engine.close()
    return code, dict(memory.stats)


# --------------------------------------------------------------------------- #
# HTTP layer
# --------------------------------------------------------------------------- #

class _Handler(BaseHTTPRequestHandler):
    server_version = "SurveyFlowUI"

    # quiet by default
    def log_message(self, fmt, *args):
        log.debug("http %s", fmt % args)

    # ---- helpers ------------------------------------------------------ #
    def _send(self, body: bytes, ctype: str, status: int = 200) -> None:
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def _json(self, obj, status: int = 200) -> None:
        self._send(json.dumps(obj).encode("utf-8"),
                   "application/json; charset=utf-8", status)

    def _read_json(self) -> dict:
        length = int(self.headers.get("Content-Length") or 0)
        if length <= 0 or length > 1_000_000:
            return {}
        try:
            return json.loads(self.rfile.read(length).decode("utf-8", "replace"))
        except Exception:
            return {}

    # ---- routes ------------------------------------------------------- #
    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        path = parsed.path
        query = parse_qs(parsed.query)

        if path == "/":
            self._send(PAGE_HTML.encode("utf-8"),
                       "text/html; charset=utf-8")
        elif path.startswith("/run/"):
            job_id = path.split("/run/", 1)[1]
            if self.server.manager.get(job_id):
                self._send(RUN_PAGE_HTML.encode("utf-8"),
                           "text/html; charset=utf-8")
            else:
                self._send(b"unknown run", "text/plain; charset=utf-8", 404)
        elif path.startswith("/api/runs/"):
            job_id = path.split("/api/runs/", 1)[1]
            job = self.server.manager.get(job_id)
            if not job:
                self._json({"error": "unknown run"}, 404)
                return
            try:
                after = int((query.get("after") or ["0"])[0])
            except ValueError:
                after = 0
            lines, nxt = job.lines_after(after)
            self._json({
                "id": job.id,
                "status": job.status,
                "code": job.code,
                "error": job.error,
                "stats": job.stats,
                "completes_target": job.params.get("completes"),
                "lines": lines,
                "next": nxt,
            })
        elif path == "/api/results":
            files = []
            if RESULTS_DIR.is_dir():
                for f in sorted(RESULTS_DIR.iterdir()):
                    if f.is_file():
                        st = f.stat()
                        files.append({"name": f.name, "size": st.st_size,
                                      "mtime": int(st.st_mtime)})
            self._json({"dir": str(RESULTS_DIR), "files": files})
        else:
            self._send(b"not found", "text/plain; charset=utf-8", 404)

    def do_POST(self) -> None:
        path = urlparse(self.path).path

        if path == "/api/runs":
            params = self._read_json()
            try:
                job = self.server.manager.start(params)
            except RuntimeError as e:
                self._json({"error": str(e)}, 409)
                return
            self._json({"id": job.id})
        elif path == "/api/selftest":
            from .selftest import report_to_dict, run_selftest

            self._json(report_to_dict(run_selftest()))
        elif path == "/api/open-folder":
            ok = _open_in_file_manager(RESULTS_DIR)
            self._json({"ok": ok, "dir": str(RESULTS_DIR)})
        elif path == "/api/quit":
            self._json({"ok": True})

            def _stop():
                time.sleep(0.2)
                self.server.shutdown()
            threading.Thread(target=_stop, daemon=True).start()
        else:
            self._send(b"not found", "text/plain; charset=utf-8", 404)


def _open_in_file_manager(path: Path) -> bool:
    try:
        path.mkdir(parents=True, exist_ok=True)
        system = platform.system()
        if system == "Windows":
            os.startfile(str(path))  # type: ignore[attr-defined]  # noqa: S606
        elif system == "Darwin":
            subprocess.Popen(["open", str(path)])
        else:
            subprocess.Popen(["xdg-open", str(path)])
        return True
    except Exception as e:
        log.warning("could not open folder %s: %s", path, e)
        return False


# --------------------------------------------------------------------------- #
# entry point
# --------------------------------------------------------------------------- #


def run_ui(host: str = "127.0.0.1", port: int = 8765,
           open_browser: bool = True,
           autorun: dict | None = None,
           quit_after_done: float | None = None) -> int:
    """Serve the window. With `autorun` a run starts immediately and the
    browser opens directly on its live-run popup (`launch --mode pop`).
    `quit_after_done` shuts the tool down that many seconds after the run ends.
    """
    server = ThreadingHTTPServer((host, port), _Handler)
    server.daemon_threads = True
    server.manager = JobManager()  # type: ignore[attr-defined]
    actual_port = server.server_address[1]
    display_host = "127.0.0.1" if host in ("0.0.0.0", "::") else host
    url = f"http://{display_host}:{actual_port}/"

    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    job = None
    if autorun:
        job = server.manager.start(autorun)  # type: ignore[attr-defined]
        run_url = f"{url}run/{job.id}?standalone=1"
        print(f"Survey Flow {__version__} - live run at {run_url}", flush=True)
    else:
        print(f"Survey Flow {__version__} - window ready at {url}", flush=True)
    print(f"Results folder: {RESULTS_DIR}", flush=True)
    if open_browser:
        try:
            webbrowser.open(f"{url}run/{job.id}?standalone=1" if job else url)
        except Exception as e:
            log.warning("could not open the browser automatically: %s", e)
            print(f"Please open {url} in your browser.", flush=True)
    else:
        print(f"Open {url} in your browser.", flush=True)

    if job and quit_after_done:
        def _auto_quit():
            while job.status == "running":
                time.sleep(0.5)
            time.sleep(quit_after_done)
            log.info("run finished - closing the tool")
            server.shutdown()
        threading.Thread(target=_auto_quit, daemon=True).start()

    try:
        while thread.is_alive():
            time.sleep(0.3)
    except KeyboardInterrupt:
        pass
    finally:
        try:
            server.shutdown()
            server.server_close()
        except Exception:
            pass
    return 0


# --------------------------------------------------------------------------- #
# the window pages (self-contained: no CDN / internet needed)
# --------------------------------------------------------------------------- #

_PAGE_CSS = """
:root {
  --bg: #f4f6fa; --card: #ffffff; --ink: #1c2733; --muted: #5b6b7c;
  --accent: #2f6fed; --accent-ink: #ffffff; --ok: #1e8e3e; --bad: #c5221f;
  --line: #e3e8ef; --mono: ui-monospace, Consolas, Menlo, monospace;
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--ink);
  font: 15px/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
}
.wrap { max-width: 880px; margin: 0 auto; padding: 28px 20px 60px; }
header h1 { margin: 0 0 2px; font-size: 24px; }
header p { margin: 0 0 22px; color: var(--muted); }
.card {
  background: var(--card); border: 1px solid var(--line); border-radius: 12px;
  padding: 20px 22px; margin-bottom: 18px; box-shadow: 0 1px 2px rgba(16,24,40,.04);
}
label { display: block; font-weight: 600; margin: 14px 0 6px; }
label:first-child { margin-top: 0; }
input[type=text], input[type=number] {
  width: 100%; padding: 10px 12px; border: 1px solid var(--line);
  border-radius: 8px; font-size: 15px; background: #fff; color: var(--ink);
}
input[type=text]:focus, input[type=number]:focus {
  outline: 2px solid var(--accent); outline-offset: -1px;
}
.row { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; }
.row .grow { flex: 1 1 220px; }
.check { display: flex; gap: 8px; align-items: center; font-weight: 500;
         margin: 12px 0 0; cursor: pointer; }
.check input { width: 16px; height: 16px; }
.check .hint { color: var(--muted); font-weight: 400; font-size: 13px; }
button {
  border: 0; border-radius: 8px; padding: 11px 18px; font-size: 15px;
  font-weight: 600; cursor: pointer;
}
button.primary { background: var(--accent); color: var(--accent-ink); }
button.primary:hover { background: #2458c4; }
button.secondary { background: #e8eefb; color: #2458c4; }
button.secondary:hover { background: #d9e4fa; }
button.ghost { background: transparent; color: var(--muted); border: 1px solid var(--line); }
button:disabled { opacity: .55; cursor: not-allowed; }
.actions { margin-top: 18px; display: flex; gap: 10px; flex-wrap: wrap; }
.status { margin-top: 14px; font-weight: 600; }
.status.ok { color: var(--ok); } .status.bad { color: var(--bad); }
.status .sub { display: block; font-weight: 400; color: var(--muted); font-size: 13px; }
.files { margin: 0; padding: 0; list-style: none; }
.files li {
  display: flex; justify-content: space-between; gap: 12px;
  padding: 7px 0; border-bottom: 1px dashed var(--line); font-size: 14px;
}
.files .size { color: var(--muted); font-variant-numeric: tabular-nums; }
.footer { color: var(--muted); font-size: 13px; display: flex;
          justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.mono { font-family: var(--mono); }
#logbox, .logbox {
  background: #101820; color: #d5e2f0; border-radius: 10px; padding: 14px;
  font-family: var(--mono); font-size: 12.5px; line-height: 1.55;
  min-height: 220px; max-height: 55vh; overflow-y: auto; white-space: pre-wrap;
  word-break: break-word;
}
.banner {
  padding: 10px 14px; border-radius: 8px; margin-bottom: 12px;
  font-weight: 600; display: none;
}
.banner.ok { display: block; background: #e6f4ea; color: var(--ok); }
.banner.bad { display: block; background: #fce8e6; color: var(--bad); }
/* modal */
.overlay {
  position: fixed; inset: 0; background: rgba(16,24,40,.45);
  display: none; align-items: center; justify-content: center; padding: 20px;
  z-index: 50;
}
.overlay.show { display: flex; }
.modal {
  background: #fff; border-radius: 14px; max-width: 560px; width: 100%;
  padding: 22px 24px; box-shadow: 0 12px 40px rgba(16,24,40,.25);
  max-height: 85vh; overflow-y: auto;
}
.modal h2 { margin: 0 0 10px; font-size: 19px; }
.modal .close { margin-top: 16px; }
.pass { color: var(--ok); font-weight: 700; }
.fail { color: var(--bad); font-weight: 700; }
ul.checks { list-style: none; margin: 10px 0 0; padding: 0; }
ul.checks li { padding: 8px 0; border-bottom: 1px dashed var(--line); }
ul.checks .detail { display: block; color: var(--muted); font-size: 13px; }
"""

_PAGE_JS = """
const $ = (sel) => document.querySelector(sel);
const fmtSize = (n) => n > 1048576 ? (n/1048576).toFixed(1)+" MB"
                    : n > 1024 ? (n/1024).toFixed(1)+" KB" : n + " B";

function showModal(title, bodyHtml) {
  $("#modal-title").textContent = title;
  $("#modal-body").innerHTML = bodyHtml;
  $("#overlay").classList.add("show");
}
function closeModal() { $("#overlay").classList.remove("show"); }

function setStatus(el, text, cls, sub) {
  el.className = "status " + (cls || "");
  el.innerHTML = text + (sub ? '<span class="sub">'+sub+"</span>" : "");
}

async function api(path, body) {
  const opts = body === undefined ? {} : {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body),
  };
  const r = await fetch(path, opts);
  if (!r.ok) {
    let msg = "request failed (" + r.status + ")";
    try { msg = (await r.json()).error || msg; } catch (e) {}
    throw new Error(msg);
  }
  return r.json();
}

/* ---- live run tracking (used by the popup and the background panel) ---- */
function trackRun(jobId, els) {
  let after = 0;
  const timer = setInterval(async () => {
    let d;
    try { d = await api("/api/runs/" + jobId + "?after=" + after); }
    catch (e) { return; }
    after = d.next;
    if (els.log && d.lines.length) {
      els.log.textContent += d.lines.join("\\n") + "\\n";
      els.log.scrollTop = els.log.scrollHeight;
    }
    if (d.status === "running") {
      if (els.banner) { els.banner.className = "banner"; els.banner.textContent = ""; }
      if (els.state) els.state.textContent = "running...";
    } else {
      clearInterval(timer);
      const ok = d.code === 0;
      const target = d.completes_target || "?";
      const got = (d.stats && d.stats.completes) || 0;
      const summary = ok
        ? "FINISHED - " + got + "/" + target + " completes reached"
        : (d.error ? "FAILED - " + d.error
                   : "FINISHED with problems - " + got + "/" + target
                     + " completes (exit code " + d.code + ")");
      if (els.banner) {
        els.banner.className = "banner " + (ok ? "ok" : "bad");
        els.banner.textContent = summary;
      }
      if (els.onDone) els.onDone(ok, summary, d);
    }
  }, 400);
  return timer;
}

/* ---- main page ---- */
function initMain() {
  const startBtn = $("#btn-start"), checkBtn = $("#btn-check");
  const statusEl = $("#run-status"), bgLog = $("#bg-log"), bgWrap = $("#bg-wrap");

  async function startRun() {
    const url = $("#url").value.trim();
    const completes = parseInt($("#completes").value || "1", 10);
    const allowLive = $("#allow-live").checked;
    const showPopup = $("#show-popup").checked;
    if (!url) { setStatus(statusEl, "Please paste the survey test link first.", "bad"); return; }
    if (!(completes > 0)) { setStatus(statusEl, "Number of completes must be at least 1.", "bad"); return; }

    startBtn.disabled = true;
    setStatus(statusEl, "Starting the test run...", "");
    let d;
    try { d = await api("/api/runs", {url, completes, allow_live: allowLive}); }
    catch (e) {
      setStatus(statusEl, "Could not start: " + e.message, "bad");
      startBtn.disabled = false;
      return;
    }

    const onDone = (ok, summary) => {
      startBtn.disabled = false;
      setStatus(statusEl, summary, ok ? "ok" : "bad",
        "Full log: " + (window.__logPath || "results folder"));
      if (!showPopup) {
        showModal(ok ? "Test run finished" : "Test run finished with problems",
          "<p>" + summary + "</p><p class='mono' style='font-size:13px'>"
          + "The log was saved to the results folder (see below).</p>");
      }
    };

    if (showPopup) {
      const w = window.open("/run/" + d.id, "sf_run_" + d.id,
        "width=900,height=640,menubar=no,toolbar=no");
      if (w) {
        setStatus(statusEl, "Running - see the popup window for the live log.", "");
        trackRun(d.id, {onDone});
      } else {
        /* popup blocked -> show the live log inline instead */
        bgWrap.style.display = "block";
        bgLog.textContent = "";
        setStatus(statusEl, "Running - popup was blocked, showing the log here.", "");
        trackRun(d.id, {log: bgLog, onDone});
      }
    } else {
      bgWrap.style.display = "block";
      bgLog.textContent = "";
      setStatus(statusEl, "Running in the background...", "",
        "You will get a message here when it finishes.");
      trackRun(d.id, {log: bgLog, onDone});
    }
    loadResults();
  }

  async function runSelftest() {
    checkBtn.disabled = true;
    showModal("Checking the tool...", "<p>Running the offline self-test, please wait...</p>");
    let d;
    try { d = await api("/api/selftest", {}); }
    catch (e) {
      showModal("Self-test", "<p class='fail'>Could not run the self-test: "
        + e.message + "</p>");
      checkBtn.disabled = false;
      return;
    }
    const items = d.checks.map(c =>
      "<li><span class='" + (c.ok ? "pass" : "fail") + "'>"
      + (c.ok ? "PASS" : "FAIL") + "</span> &nbsp;" + c.name
      + (c.detail ? "<span class='detail'>" + c.detail + "</span>" : "") + "</li>"
    ).join("");
    showModal(d.ok ? "SELF-TEST PASSED - the tool is working"
                  : "SELF-TEST FAILED",
      "<p>" + (d.ok
        ? "Every part of the tool answered correctly, including a full test run "
          + "against the built-in demo survey."
        : "Something is not working - see the failing checks below.") + "</p>"
      + "<ul class='checks'>" + items + "</ul>");
    checkBtn.disabled = false;
  }

  async function loadResults() {
    let d;
    try { d = await api("/api/results"); } catch (e) { return; }
    window.__logPath = d.dir;
    $("#results-dir").textContent = d.dir;
    const ul = $("#files");
    ul.innerHTML = d.files.slice(-8).reverse().map(f =>
      "<li><span class='mono'>" + f.name + "</span>"
      + "<span class='size'>" + fmtSize(f.size) + "</span></li>"
    ).join("") || "<li><span class='size'>no files yet</span></li>";
  }

  $("#btn-start").addEventListener("click", startRun);
  $("#btn-check").addEventListener("click", runSelftest);
  $("#btn-folder").addEventListener("click", () => api("/api/open-folder", {}));
  $("#btn-quit").addEventListener("click", async () => {
    try { await api("/api/quit", {}); } catch (e) {}
    document.body.innerHTML =
      "<div class='wrap'><div class='card'><h2>Survey Flow closed</h2>"
      + "<p>You can close this window now.</p></div></div>";
  });
  $("#modal-close").addEventListener("click", closeModal);
  $("#overlay").addEventListener("click", (e) => {
    if (e.target && e.target.id === "overlay") closeModal();
  });
  loadResults();
}

/* ---- popup run window ---- */
function initRun() {
  const jobId = location.pathname.split("/run/")[1];
  const standalone = location.search.indexOf("standalone=1") >= 0;
  const logEl = document.getElementById("logbox");
  trackRun(jobId, {
    log: logEl,
    banner: document.getElementById("banner"),
    onDone: (ok, summary) => {
      document.getElementById("btn-close").style.display = "inline-block";
      document.title = (ok ? "DONE - " : "PROBLEM - ") + document.title;
    },
  });
  const btn = document.getElementById("btn-close");
  if (standalone) {
    btn.textContent = "Finish and close";
    btn.addEventListener("click", async () => {
      try { await api("/api/quit", {}); } catch (e) {}
      window.close();
    });
  } else {
    btn.addEventListener("click", () => window.close());
  }
}

document.addEventListener("DOMContentLoaded", () => {
  if (location.pathname.startsWith("/run/")) initRun();
  else initMain();
});
"""

_PAGE_HTML = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Survey Flow - survey test runner</title>
<style>""" + _PAGE_CSS + """</style>
</head>
<body>
<div class="wrap">
  <header>
    <h1>Survey Flow</h1>
    <p>Paste your survey <b>test link</b>, say how many completes you need, press Start.</p>
  </header>

  <div class="card">
    <label for="url">Survey test link</label>
    <input type="text" id="url" placeholder="https://... (paste the TEST link here)"
           autocomplete="off" spellcheck="false">

    <div class="row">
      <div class="grow">
        <label for="completes">Number of completes</label>
        <input type="number" id="completes" value="3" min="1" max="999">
      </div>
    </div>

    <label class="check">
      <input type="checkbox" id="show-popup" checked>
      Open live run window (popup) &mdash; show the test run while it happens
      <span class="hint">(untick to run in the background)</span>
    </label>
    <label class="check">
      <input type="checkbox" id="allow-live">
      Allow a link that is not a test link
      <span class="hint">(QA only &mdash; never run this on live fielding)</span>
    </label>

    <div class="actions">
      <button class="primary" id="btn-start">Start test run</button>
      <button class="secondary" id="btn-check">Check tool is working</button>
    </div>
    <div class="status" id="run-status"></div>

    <div id="bg-wrap" style="display:none;margin-top:14px">
      <div class="logbox" id="bg-log"></div>
    </div>
  </div>

  <div class="card">
    <label>Results folder</label>
    <div class="mono" id="results-dir" style="font-size:13px">...</div>
    <ul class="files" id="files"></ul>
    <div class="actions">
      <button class="ghost" id="btn-folder">Open results folder</button>
    </div>
  </div>

  <div class="footer">
    <span>Survey Flow __VERSION__ &middot; rule-based survey QA tool (no AI)</span>
    <button class="ghost" id="btn-quit">Close tool</button>
  </div>
</div>

<div class="overlay" id="overlay">
  <div class="modal">
    <h2 id="modal-title"></h2>
    <div id="modal-body"></div>
    <button class="secondary close" id="modal-close">Close</button>
  </div>
</div>

<script>""" + _PAGE_JS + """</script>
</body>
</html>
"""

_RUN_PAGE_HTML = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Survey Flow - live test run</title>
<style>""" + _PAGE_CSS + """</style>
</head>
<body>
<div class="wrap">
  <header>
    <h1>Live test run</h1>
    <p>Every page the tool sees and answer it picks appears below.</p>
  </header>
  <div class="banner" id="banner"></div>
  <div class="logbox" id="logbox">starting...</div>
  <div class="actions">
    <button class="secondary" id="btn-close" style="display:none">Close window</button>
  </div>
</div>
<script>""" + _PAGE_JS + """</script>
</body>
</html>
"""


def _with_version(html: str) -> str:
    return html.replace("__VERSION__", __version__)


PAGE_HTML = _with_version(_PAGE_HTML)
RUN_PAGE_HTML = _with_version(_RUN_PAGE_HTML)
