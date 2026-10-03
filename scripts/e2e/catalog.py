#!/usr/bin/env python3
"""Temporary, current-dated demo catalog for real-stack browser acceptance."""
import copy
import datetime
import json
import pathlib
import sys

root = pathlib.Path(__file__).resolve().parents[2]
data = json.loads((root / 'data/demo-catalog.json').read_text())
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
for course in data['courses']:
    course['checked_at'] = now
    for offer in course['offers']:
        offer.update(price_checked_at=now, valid_until=None)
for name, status in [('closed', 'published'), ('draft', 'draft'), ('archived', 'archived')]:
    course = copy.deepcopy(data['courses'][0])
    course.update(id=f'{name}-course', slug=f'{name}-course', status=status)
    course['offers'][0].update(id=f'{name}-tariff', enrollment='closed' if name == 'closed' else 'open')
    data['courses'].append(course)
pathlib.Path(sys.argv[1]).write_text(json.dumps(data, ensure_ascii=False))
