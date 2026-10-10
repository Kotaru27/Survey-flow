from survey_flow.outcomes import classify_text, find_validation


def test_complete_detected():
    status, ev = classify_text("Thank you. Your survey is now complete.")
    assert status == "complete"
    assert ev


def test_terminate_detected():
    status, _ = classify_text("Survey terminated. You do not qualify.")
    assert status == "terminate"


def test_terminate_url_signal():
    status, _ = classify_text("", "https://survey.example.com/terminate/sv/abc")
    assert status == "terminate"


def test_quota_detected():
    status, _ = classify_text("Sorry, this quota has been reached.")
    assert status == "quota_full"


def test_quota_wins_over_thanks():
    # quota pages often still say thank you
    status, _ = classify_text("Thank you. Unfortunately the quota is full.")
    assert status == "quota_full"


def test_not_an_end_page():
    status, _ = classify_text("Which of these brands have you heard of?")
    assert status == ""


def test_find_validation():
    msgs = find_validation("Please select at least 4 options")
    assert msgs


def test_question_text_not_validation():
    # a question mentioning termination must not classify as an end status
    status, _ = classify_text("Have you ever been terminated from a job?")
    assert status == ""
