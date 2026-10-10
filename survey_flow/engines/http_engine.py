"""Plain-HTTP engine (requests): works for classic Confirmit/Forsta forms
without JavaScript.  Each session keeps its own cookie jar, so every run is a
fresh respondent."""

from __future__ import annotations

import logging
import re
from collections import defaultdict
from urllib.parse import urljoin

import requests

from ..models import Page, SubmitControl
from .base import Engine, NoSubmitError, Session

log = logging.getLogger("survey_flow.engine.http")

UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) "
      "Chrome/122.0.0.0 Safari/537.36")

_NEXT_RE = re.compile(r"^(next|continue|weiter|›|»|>|→|next\s*>)$", re.I)
_SUBMIT_RE = re.compile(r"^(submit|ok|done|finish|save|apply|go|start|begin|enter)$", re.I)

_SCALAR_TYPES = {"text", "textarea", "number", "range", "date", "time", "datetime-local"}


class HttpEngine(Engine):
    def new_session(self) -> Session:
        return HttpSession()

    def close(self) -> None:
        pass


class HttpSession(Session):
    def __init__(self, timeout: float = 60.0):
        self.s = requests.Session()
        self.s.headers.update({
            "User-Agent": UA,
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
        })
        self.timeout = timeout

    def fetch(self, url: str) -> tuple[str, str]:
        r = self.s.get(url, timeout=self.timeout, allow_redirects=True)
        r.raise_for_status()
        return r.text, r.url

    def submit(self, page: Page) -> tuple[str, str]:
        action = urljoin(page.url, page.form_action or page.url)
        data: dict = {}
        multi: dict[str, list] = defaultdict(list)

        def put(name: str, value: str) -> None:
            if not name:
                return
            multi[name].append(value)

        for name, value in page.hiddens:
            put(name, value)

        for q in page.questions:
            for f in q.fields:
                if f.type in ("radio", "checkbox"):
                    if f.selected:
                        put(f.name, f.value)
                elif f.type == "select-one":
                    v = f.new_value if f.new_value is not None else f.value
                    if f.name:
                        put(f.name, v if v is not None else "")
                elif f.type == "select-multiple":
                    if f.new_value is not None:
                        put(f.name, f.new_value)
                    else:
                        for o in f.options:
                            if o.checked:
                                put(f.name, o.value)
                elif f.type in _SCALAR_TYPES:
                    if f.name:
                        v = f.new_value if f.new_value is not None else (f.value or "")
                        put(f.name, v)

        for name, values in multi.items():
            data[name] = values[0] if len(values) == 1 else values

        sc = choose_submit(page)
        if sc is None:
            raise NoSubmitError(
                "no submit control found on page "
                f"({page.url}); try --engine browser for JS-driven surveys"
            )
        if sc.name:
            data[sc.name] = sc.value if sc.value else (sc.label or "")

        method = (page.form_method or "POST").upper()
        log.debug("submit %s %s via %r", method, action, sc.label)
        if method == "GET":
            r = self.s.get(action, params=data, timeout=self.timeout, allow_redirects=True)
        else:
            r = self.s.post(action, data=data, timeout=self.timeout, allow_redirects=True)
        r.raise_for_status()
        return r.text, r.url


def choose_submit(page: Page) -> SubmitControl | None:
    if not page.submits:
        return None
    for s in page.submits:
        if _NEXT_RE.match((s.label or s.value or "").strip()):
            return s
    for s in page.submits:
        if _SUBMIT_RE.match((s.label or s.value or "").strip()):
            return s
    # prefer an <input type=submit> over <button>
    for s in page.submits:
        if s.tag == "input":
            return s
    return page.submits[0]
