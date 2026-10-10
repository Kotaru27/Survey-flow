"""Back-compat shim: the mock Confirmit-style survey now ships inside the
package (`survey_flow.mock_survey`) so the built tool can use it for selftests.
Tests keep importing from `tests.mock_survey` unchanged.
"""

from survey_flow.mock_survey import (  # noqa: F401  # pylint: disable=unused-import
    MockSurveyHandler,
    start_mock_server,
)
