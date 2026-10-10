from .base import Engine, EngineError, EngineUnavailable, NoSubmitError, Session

__all__ = ["Engine", "EngineError", "EngineUnavailable", "NoSubmitError", "Session"]


def create_engine(name: str, headed: bool = False) -> Engine:
    if name == "http":
        from .http_engine import HttpEngine
        return HttpEngine()
    if name == "browser":
        from .browser_engine import BrowserEngine
        return BrowserEngine(headed=headed)
    raise ValueError(f"unknown engine: {name}")
