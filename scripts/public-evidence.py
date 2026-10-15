#!/usr/bin/env python3
"""Bounded, anonymous reads for #123/#124; failures are evidence, not CI failures."""
import datetime
import hashlib
import json
from pathlib import Path
import ssl
import urllib.error
import urllib.request

DEST = Path('public-evidence')
ORIGIN = 'http://alekslesik.fvds.ru'
QUERY = '/api/v1/courses?language=python&experience=none&goal=try&include_unknown_profiles=true&page_size=48'


def read(url, filename):
    try:
        req = urllib.request.Request(url, headers={'User-Agent': 'DevCourseFinder-public-acceptance/1.0'})
        with urllib.request.urlopen(req, timeout=25) as response:
            body = response.read(4 * 1024 * 1024 + 1)
            if len(body) > 4 * 1024 * 1024:
                raise ValueError('response exceeds 4 MiB')
            (DEST / filename).write_bytes(body)
            return {'url': url, 'final_url': response.url, 'status': response.status,
                    'sha256': hashlib.sha256(body).hexdigest(), 'file': filename}
    except (OSError, ValueError, urllib.error.URLError) as error:
        return {'url': url, 'error': str(error)}


def main():
    DEST.mkdir(exist_ok=True)
    report = {'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'tls_verification': 'default trust store; never disabled', 'reads': []}
    for scheme in ['http', 'https']:
        for index, path in enumerate(['/courses?language=python', '/about', '/sitemap.xml', QUERY]):
            report['reads'].append(read(scheme + '://alekslesik.fvds.ru' + path, f'{scheme}-{index}.txt'))
    # Save both pages: counts must be interpreted against the same frozen snapshot.
    for page in [1, 2]:
        report['reads'].append(read(ORIGIN + QUERY + f'&page={page}', f'catalog-{page}.json'))
    # Dedicated official fields, not search snippets or course-title inference.
    for course_id in [58852, 63085, 67, 245274, 245606, 244062, 243826, 243175]:
        report['reads'].append(read(f'https://stepik.org/api/courses/{course_id}', f'stepik-{course_id}.json'))
    (DEST / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
