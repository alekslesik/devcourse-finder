import http.server
import threading
import unittest

import measure


class DriverTests(unittest.TestCase):
    def exercise(self, status, body):
        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'

            def do_GET(self):
                self.send_response(status)
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_):
                pass

        server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            return measure.run_phase(f'http://127.0.0.1:{server.server_port}', .1, 3)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_valid_search_responses_are_measured(self):
        result = self.exercise(200, b'{"items":[],"total":0}')
        self.assertGreater(result['successful_requests'], 0)
        self.assertEqual(result['failed_requests'], 0)
        self.assertLessEqual(result['p50_ms'], result['p95_ms'])
        self.assertLessEqual(result['p95_ms'], result['max_ms'])

    def test_http_failure_is_not_reported_as_success(self):
        result = self.exercise(503, b'{"error":"unavailable"}')
        self.assertEqual(result['successful_requests'], 0)
        self.assertGreater(result['failed_requests'], 0)
        self.assertIsNone(result['p95_ms'])

    def test_invalid_payload_is_not_reported_as_success(self):
        result = self.exercise(200, b'{"ok":true}')
        self.assertEqual(result['successful_requests'], 0)
        self.assertGreater(result['failed_requests'], 0)


if __name__ == '__main__':
    unittest.main()
