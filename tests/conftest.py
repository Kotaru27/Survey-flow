import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tests.mock_survey import start_mock_server  # noqa: E402


@pytest.fixture(scope="session")
def mock_url():
    server, url = start_mock_server()
    yield url
    server.shutdown()
