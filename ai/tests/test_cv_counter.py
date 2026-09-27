import pytest

cv2 = pytest.importorskip("cv2")  # opencv is only in requirements-cv.txt

from cv.crowd_counter import ZoneCounter, parse_polygon  # noqa: E402


def test_zone_count_and_line_crossing():
    zc = ZoneCounter(zone=parse_polygon("0,0.5 1,0.5 1,1 0,1"), line=((0, 0.5), (1, 0.5)))
    assert zc.update([(0.5, 0.2), (0.5, 0.8)], [1, 2]) == 1   # one person in the lower zone
    zc.update([(0.5, 0.7), (0.5, 0.8)], [1, 2])               # track 1 crosses the line
    assert zc.entries + zc.exits == 1
    assert zc.smoothed() in (1.0, 1.5, 2.0)
