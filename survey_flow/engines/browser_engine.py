"""Optional Playwright browser engine for JS-driven surveys.

Requires (on the machine where the tool runs):
    pip install -r requirements-browser.txt
    playwright install chromium
"""

from __future__ import annotations

import json
import logging
import re

from ..models import Page
from .base import Engine, EngineUnavailable, NoSubmitError, Session

log = logging.getLogger("survey_flow.engine.browser")

_NEXT_RE = re.compile(r"^(next|continue|weiter|›|»|>|→|next\s*>)$", re.I)
_SUBMIT_RE = re.compile(r"^(submit|ok|done|finish|save|apply|go|start|begin|enter)$", re.I)

_APPLY_JS = """
([fields, submits]) => {
  const all = document.querySelectorAll('input, select, textarea');
  for (const spec of fields) {
    const el = all[spec.i];
    if (!el) continue;
    if (el.type === 'radio' || el.type === 'checkbox') {
      el.checked = !!spec.checked;
    } else if (el.tagName === 'SELECT') {
      if (spec.value !== null && spec.value !== undefined) el.value = spec.value;
    } else {
      if (spec.value !== null && spec.value !== undefined) el.value = spec.value;
    }
    el.dispatchEvent(new Event('input', {bubbles: true}));
    el.dispatchEvent(new Event('change', {bubbles: true}));
  }
  return submits;
}
"""


class BrowserEngine(Engine):
    def __init__(self, headed: bool = False, timeout_ms: int = 30000):
        try:
            from playwright.sync_api import sync_playwright
        except ImportError as e:
            raise EngineUnavailable(
                "playwright is not installed. Install the browser engine with:\n"
                "  pip install -r requirements-browser.txt && playwright install chromium"
            ) from e
        self._pw = sync_playwright().start()
        try:
            self.browser = self._pw.chromium.launch(headless=not headed)
        except Exception as e:
            self._pw.stop()
            raise EngineUnavailable(
                "chromium is not available. Run: playwright install chromium\n"
                f"(original error: {e})"
            ) from e
        self.timeout_ms = timeout_ms

    def new_session(self) -> Session:
        return BrowserSession(self, self.browser)

    def close(self) -> None:
        try:
            self.browser.close()
        finally:
            self._pw.stop()


class BrowserSession(Session):
    def __init__(self, engine: BrowserEngine, browser):
        self.engine = engine
        self.ctx = browser.new_context()
        self.page = self.ctx.new_page()

    def fetch(self, url: str) -> tuple[str, str]:
        self.page.goto(url, wait_until="domcontentloaded", timeout=self.engine.timeout_ms)
        self._settle()
        return self.page.content(), self.page.url

    def submit(self, page: Page) -> tuple[str, str]:
        fields = []
        for q in page.questions:
            for f in q.fields:
                if f.type in ("radio", "checkbox"):
                    fields.append({"i": f.dom_index, "checked": bool(f.selected)})
                elif f.type in ("select-one", "select-multiple", "text", "textarea",
                                "number", "range", "date", "time", "datetime-local"):
                    if f.new_value is not None:
                        fields.append({"i": f.dom_index, "value": f.new_value})
        # hidden inputs keep their DOM values (already present)
        submits = [{"i": s.dom_index, "name": s.name, "value": s.value, "label": s.label}
                   for s in page.submits]
        try:
            self.page.evaluate(_APPLY_JS, [fields, submits])
        except Exception as e:
            raise NoSubmitError(f"failed to fill page via browser engine: {e}") from e

        target = self._pick_submit(page)
        if target is None:
            raise NoSubmitError(f"no submit control found on {page.url}")
        try:
            with self.page.expect_navigation(timeout=8000):
                target.click(force=True, timeout=8000)
        except Exception:
            # some pages update in place (AJAX) - just give them a moment
            self.page.wait_for_timeout(1200)
        self._settle()
        return self.page.content(), self.page.url

    def _pick_submit(self, page: Page):
        if not page.submits:
            return None
        chosen = None
        for s in page.submits:
            if _NEXT_RE.match((s.label or s.value or "").strip()):
                chosen = s
                break
        if chosen is None:
            for s in page.submits:
                if _SUBMIT_RE.match((s.label or s.value or "").strip()):
                    chosen = s
                    break
        if chosen is None:
            chosen = page.submits[0]
        if chosen.name:
            loc = self.page.locator(
                f'input[name={json.dumps(chosen.name)}], button[name={json.dumps(chosen.name)}]'
            )
            if loc.count():
                return loc.first
        if chosen.value:
            loc = self.page.locator(
                f'input[type="submit"][value={json.dumps(chosen.value)}], '
                f'button:has-text({json.dumps(chosen.value)})'
            )
            if loc.count():
                return loc.first
        if chosen.label:
            loc = self.page.locator(
                f'button:has-text({json.dumps(chosen.label)}), '
                f'input[type="submit"][value={json.dumps(chosen.label)}]'
            )
            if loc.count():
                return loc.first
        return None

    def _settle(self) -> None:
        try:
            self.page.wait_for_load_state("networkidle", timeout=5000)
        except Exception:
            pass

    def close(self) -> None:
        try:
            self.ctx.close()
        except Exception:
            pass
