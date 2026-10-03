#!/usr/bin/env python3
"""Closed-loop HTTP search load: 20 clients, complete-body latency, nearest-rank p95."""
import argparse
import concurrent.futures
import datetime
import http.client
import json
import math
import pathlib
import threading
import time
import urllib.parse

QUERIES = [
    '', 'language=go', 'language=python&sort=price_asc',
    'language=java&sort=price_desc', 'language=javascript&sort=duration',
    'budget=free', 'budget=paid&include_free=true',
    'support=review&hours=10', 'schedule=flexible&include_closed=true',
    'language=go&direction=backend&goal=job&experience=basics&min=100000&max=10000000',
    'page=20&page_size=48&sort=price_asc', 'language=go&max=1',
]


def run_phase(base, seconds, clients):
    parsed = urllib.parse.urlsplit(base)
    if parsed.scheme != 'http':
        raise ValueError('Use the isolated local HTTP load endpoint')
    barrier = threading.Barrier(clients + 1)
    deadline = [0.0]

    def worker(index):
        conn = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=10)
        latencies, errors = [], {}
        query_index = index
        barrier.wait()
        while time.perf_counter() < deadline[0]:
            query = QUERIES[query_index % len(QUERIES)]
            query_index += 1
            started = time.perf_counter()
            try:
                conn.request('GET', '/api/v1/courses?' + query)
                response = conn.getresponse()
                body = response.read()
                latency = (time.perf_counter() - started) * 1000
                if response.status != 200:
                    raise ValueError(f'HTTP {response.status}')
                payload = json.loads(body)
                if not isinstance(payload.get('items'), list) or not isinstance(payload.get('total'), int):
                    raise ValueError('invalid search response')
                latencies.append(latency)
            except Exception as error:
                label = f'{type(error).__name__}: {error}'
                errors[label] = errors.get(label, 0) + 1
                conn.close()
                conn = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=10)
        conn.close()
        return latencies, errors

    with concurrent.futures.ThreadPoolExecutor(max_workers=clients) as pool:
        futures = [pool.submit(worker, i) for i in range(clients)]
        started = time.perf_counter()
        deadline[0] = started + seconds
        barrier.wait()
        results = [future.result() for future in futures]
        elapsed = time.perf_counter() - started
    samples = sorted(v for latencies, _ in results for v in latencies)
    errors = {}
    for _, failures in results:
        for label, count in failures.items():
            errors[label] = errors.get(label, 0) + count
    percentile = lambda fraction: samples[max(0, math.ceil(len(samples) * fraction) - 1)] if samples else None
    return {'duration_seconds': seconds, 'elapsed_seconds': round(elapsed, 3),
            'successful_requests': len(samples), 'failed_requests': sum(errors.values()),
            'errors': errors, 'requests_per_second': round(len(samples) / elapsed, 2),
            'p50_ms': percentile(.5), 'p95_ms': percentile(.95),
            'p99_ms': percentile(.99), 'max_ms': samples[-1] if samples else None}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--url', default='http://127.0.0.1:18080')
    parser.add_argument('--warmup', type=int, default=60)
    parser.add_argument('--duration', type=int, default=300)
    parser.add_argument('--clients', type=int, default=20)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    if min(args.warmup, args.duration, args.clients) < 1:
        parser.error('phase durations and clients must be positive')
    print(f'Warmup: {args.warmup}s, {args.clients} clients', flush=True)
    warmup = run_phase(args.url, args.warmup, args.clients)
    print(f'Measurement: {args.duration}s; warmup p95={warmup["p95_ms"]} ms', flush=True)
    measurement = run_phase(args.url, args.duration, args.clients)
    passed = measurement['p95_ms'] is not None and measurement['p95_ms'] <= 500 and measurement['failed_requests'] == 0
    report = {'timestamp_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'clients': args.clients, 'published_offers': 10000, 'queries': QUERIES,
              'warmup': warmup, 'measurement': measurement, 'target_p95_ms': 500,
              'passed': passed, 'method': 'closed-loop, keep-alive, full response body; nearest-rank percentiles'}
    pathlib.Path(args.output).write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(measurement, indent=2), flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
