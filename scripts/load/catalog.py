#!/usr/bin/env python3
"""Generate synthetic published courses; never use this catalog in production."""
import copy
import datetime
import json
import pathlib
import sys

root = pathlib.Path(__file__).resolve().parents[2]
source = json.loads((root / 'data/demo-catalog.json').read_text())
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
courses = []
for i in range(10000):
    course = copy.deepcopy(source['courses'][i % len(source['courses'])])
    course.update(id=f'load-{i:05d}', slug=f'load-{i:05d}', checked_at=now,
                  status='published', demo=True)
    offer = course['offers'][0]
    offer.update(id=f'load-{i:05d}-tariff', price_checked_at=now, valid_until=None)
    course['offers'] = [offer]
    courses.append(course)
pathlib.Path(sys.argv[1]).write_text(json.dumps(
    {'approved_domains': source['approved_domains'], 'courses': courses}, ensure_ascii=False))
