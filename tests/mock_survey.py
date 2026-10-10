"""A miniature Confirmit/Forsta-like survey used by the end-to-end test.

Flow:
  0 consent page (interstitial, required checkbox)
  1 SA screener: "No" terminates                 -> teaches memory
  2 SA with red bold instruction: Select "Yes"   -> instruction protection
  3 MA requiring >= 4 selections (validation)    -> validation learning
  4 dropdown: "Decline to answer" -> quota full  -> quota memory
  5 SA grid, 3 required rows + red instruction
  6 open text, min 5 chars
  -> complete

Options are ordered so that a round-robin strategy hits the terminate and
the quota-full ends first, then learns to avoid them.
"""

from __future__ import annotations

import threading
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs

PAGE_CONSENT = """<!doctype html><html><body>
<h2>Welcome to the survey</h2>
<form method="post" action="/survey">
  <p>Before you begin, please confirm your consent.</p>
  <label><input type="checkbox" name="consent" value="on"> I agree to participate</label>
  <input type="submit" name="Next" value="Start">
</form></body></html>"""

PAGE_Q1 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV1">
  <p class="QuestionText">Are you currently employed?</p>
  <label><input type="radio" name="q1" value="1"> No</label>
  <label><input type="radio" name="q1" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_Q2 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV2">
  <p class="QuestionText">Do you have a valid driving license?</p>
  <span style="color:#ff0000"><b>Select: "Yes"</b></span>
  <label><input type="radio" name="q2" value="1"> No</label>
  <label><input type="radio" name="q2" value="2"> Yes</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_Q3 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV3">
  <p class="QuestionText">Which of the following services do you use? (select all that apply)</p>
  <label><input type="checkbox" name="q3_1" value="1"> Email</label>
  <label><input type="checkbox" name="q3_2" value="2"> Search</label>
  <label><input type="checkbox" name="q3_3" value="3"> Video</label>
  <label><input type="checkbox" name="q3_4" value="4"> Shopping</label>
  <label><input type="checkbox" name="q3_5" value="5"> None of these</label>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_Q4 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV4">
  <p class="QuestionText">Which plan best describes you?</p>
  <select name="q4">
    <option value="">-- Select --</option>
    <option value="1">Decline to answer</option>
    <option value="2">Full access</option>
    <option value="3">Beta tester</option>
  </select>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_Q5 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV5">
  <p class="QuestionText">Rate the following statements</p>
  <span style="color:red"><b>Answer all three rows</b></span>
  <table>
    <tr><td>Delivery speed</td>
        <td><label><input type="radio" name="q5_1" value="1"> Poor</label></td>
        <td><label><input type="radio" name="q5_1" value="2"> Average</label></td>
        <td><label><input type="radio" name="q5_1" value="3"> Good</label></td></tr>
    <tr><td>Value for money</td>
        <td><label><input type="radio" name="q5_2" value="1"> Poor</label></td>
        <td><label><input type="radio" name="q5_2" value="2"> Average</label></td>
        <td><label><input type="radio" name="q5_2" value="3"> Good</label></td></tr>
    <tr><td>Customer support</td>
        <td><label><input type="radio" name="q5_3" value="1"> Poor</label></td>
        <td><label><input type="radio" name="q5_3" value="2"> Average</label></td>
        <td><label><input type="radio" name="q5_3" value="3"> Good</label></td></tr>
  </table>
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_Q6 = """<!doctype html><html><body>
<form method="post" action="/survey">
<div id="QuestionDIV6">
  <p class="QuestionText">What is the main reason you use our service?</p>
  <input type="text" name="q6" maxlength="100">
  <input type="submit" name="Next" value="Next">
</div></form></body></html>"""

PAGE_COMPLETE = """<!doctype html><html><body>
<h1>Thank you</h1>
<p>Your survey is now complete.</p>
</body></html>"""

PAGE_TERMINATE = """<!doctype html><html><body>
<h1>Survey terminated</h1>
<p>You do not qualify for this survey.</p>
</body></html>"""

PAGE_QUOTA = """<!doctype html><html><body>
<h1>Quota full</h1>
<p>Sorry, this quota has been reached.</p>
</body></html>"""


def _error_page(html: str, message: str) -> str:
    return html.replace('<form method="post"', f'<div class="error">{message}</div>\n<form method="post"', 1)


class _State:
    def __init__(self):
        self.page = 0


class MockSurveyHandler(BaseHTTPRequestHandler):
    states: dict[str, _State] = {}
    lock = threading.Lock()

    def log_message(self, *args):  # silence
        pass

    # ------------------------------------------------------------------ #
    def _cookie_sid(self) -> str | None:
        raw = self.headers.get("Cookie", "")
        for part in raw.split(";"):
            k, _, v = part.strip().partition("=")
            if k == "sid" and v:
                return v
        return None

    def _new_sid(self) -> str:
        sid = uuid.uuid4().hex
        with self.lock:
            self.states[sid] = _State()
        return sid

    def _state(self) -> tuple[str, _State]:
        sid = self._cookie_sid()
        if sid is None or sid not in self.states:
            sid = self._new_sid()
        return sid, self.states[sid]

    def _send(self, body: str, sid: str, new: bool) -> None:
        data = body.encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        if new:
            self.send_header("Set-Cookie", f"sid={sid}; Path=/")
        self.end_headers()
        self.wfile.write(data)

    # ------------------------------------------------------------------ #
    def do_GET(self):
        if self.path.split("?")[0] in ("/", "/survey"):
            sid = self._cookie_sid()
            new = sid is None or sid not in self.states
            if new:
                sid = self._new_sid()
            self._send(self._render(self.states[sid].page), sid, new)
        else:
            self.send_error(404)

    def do_POST(self):
        if self.path.split("?")[0] != "/survey":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length).decode("utf-8", "replace")
        data = parse_qs(body, keep_blank_values=True)
        sid, st = self._state()

        def val(key: str) -> str:
            return (data.get(key) or [""])[0]

        err = None
        page = st.page
        if page == 0:
            if val("consent") != "on":
                err = "Please agree to continue"
            else:
                st.page = 1
        elif page == 1:
            # q1 value 1 = "No" -> terminate
            st.page = 2 if val("q1") == "2" else 90
        elif page == 2:
            # q2 must be Yes (value 2); red instruction protects the run
            st.page = 3 if val("q2") == "2" else 90
        elif page == 3:
            chosen = [k for k in data if k.startswith("q3_") and data[k] and data[k][0]]
            if len(chosen) < 4:
                err = "Please select at least 4 options"
            else:
                st.page = 4
        elif page == 4:
            q4 = val("q4")
            if q4 == "":
                err = "Please select an option"
            elif q4 == "1":
                st.page = 91                      # Decline to answer -> quota full
            else:
                st.page = 5
        elif page == 5:
            if all(val(f"q5_{i}") for i in (1, 2, 3)):
                st.page = 6
            else:
                err = "Please answer each row"
        elif page == 6:
            if len(val("q6").strip()) >= 5:
                st.page = 99
            else:
                err = "Please enter at least 5 characters"
        elif page == 99:
            err = None
            st.page = 99

        # `err` renders the SAME page again with an error box
        if err:
            html = self._render(st.page).replace(
                '<form method="post"', f'<div class="error">{err}</div>\n<form method="post"', 1
            )
        else:
            html = self._render(st.page)
        self._send(html, sid, False)

    def _render(self, page: int) -> str:
        return {
            0: PAGE_CONSENT,
            1: PAGE_Q1,
            2: PAGE_Q2,
            3: PAGE_Q3,
            4: PAGE_Q4,
            5: PAGE_Q5,
            6: PAGE_Q6,
            90: PAGE_TERMINATE,
            91: PAGE_QUOTA,
            99: PAGE_COMPLETE,
        }[page]


def start_mock_server() -> tuple[ThreadingHTTPServer, str]:
    server = ThreadingHTTPServer(("127.0.0.1", 0), MockSurveyHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    host, port = server.server_address[:2]
    return server, f"http://{host}:{port}/survey"
