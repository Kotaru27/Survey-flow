"""Tests for PID extraction (results-folder naming from the survey link)."""

from survey_flow.pid import extract_pid


def test_pid_from_query_param():
    assert extract_pid("https://x.com/CF/run?__pid=ABC_123&q=1") == "ABC_123"
    assert extract_pid("https://x.com/run?pid=55512") == "55512"
    assert extract_pid("https://x.com/run?PID=study77") == "study77"
    assert extract_pid("https://x.com/run?projectid=P9&s=x") == "P9"


def test_pid_query_param_wins_over_numbers():
    assert extract_pid("https://x.com/a/999?pid=QA42") == "QA42"


def test_pid_from_decipher_style_path():
    assert extract_pid("https://surveys.example.com/survey/12345/test?u=1") == "12345"
    assert extract_pid("https://x.com/survey/abc-9/testing") == "abc-9"


def test_pid_from_loose_param_or_digits():
    assert extract_pid("https://x.com/wss/?s=QX99") == "QX99"
    assert extract_pid("https://x.com/storage/554433/page.html") == "554433"


def test_pid_fallback_is_stable_and_safe():
    a = extract_pid("https://x.com/nothing/here")
    b = extract_pid("https://x.com/nothing/here")
    assert a == b
    assert a.startswith("PID-")
    assert all(c.isalnum() or c in "-_" for c in extract_pid("https://x.com/?pid=a/b\\c"))


def test_pid_never_empty():
    assert extract_pid("")
    assert extract_pid(None)
