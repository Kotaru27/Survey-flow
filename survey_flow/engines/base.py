"""Engine interfaces: something that can fetch pages and submit answers."""

from __future__ import annotations

from ..models import Page


class EngineError(Exception):
    pass


class NoSubmitError(EngineError):
    pass


class EngineUnavailable(EngineError):
    pass


class Session:
    """One survey run (fresh cookies/context per run)."""

    def fetch(self, url: str) -> tuple[str, str]:
        """Return (html, final_url) for the initial page."""
        raise NotImplementedError

    def submit(self, page: Page) -> tuple[str, str]:
        """Apply the decisions already written onto `page` and submit.

        Returns (html, final_url) of the resulting page.
        """
        raise NotImplementedError

    def close(self) -> None:
        pass


class Engine:
    def new_session(self) -> Session:
        raise NotImplementedError

    def close(self) -> None:
        pass
