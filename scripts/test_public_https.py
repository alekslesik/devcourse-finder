import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.error

spec = importlib.util.spec_from_file_location('https_check', Path(__file__).with_name('check-public-https.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class Response(io.BytesIO):
    status = 200

    def __init__(self, url, body):
        super().__init__(body.encode())
        self.url = url


class PublicHTTPSTests(unittest.TestCase):
    def run_check(self, *, redirect_status=301, location=None, canonical=None, sitemap=None, tls_error=False):
        def open_https(url, **kwargs):
            if tls_error:
                raise urllib.error.URLError('certificate verify failed')
            if '/api/' in url:
                body = json.dumps({'items': [{'course': {'slug': 'example'}}]})
            elif '/about' in url:
                body = f'<link rel="canonical" href="{canonical or check.ORIGIN + "/about"}"><meta property="og:image" content="{check.ORIGIN}/social.svg">'
            elif '/sitemap' in url:
                body = f'<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>{sitemap or check.ORIGIN + "/courses"}</loc></url></urlset>'
            else:
                body = 'content'
            return Response(url, body)

        class Redirect:
            def open(self, url, **kwargs):
                raise urllib.error.HTTPError(url, redirect_status, 'redirect',
                                             {'Location': location or url.replace('http:', 'https:', 1)}, None)

        with patch.object(check.urllib.request, 'urlopen', side_effect=open_https), \
             patch.object(check.urllib.request, 'build_opener', return_value=Redirect()), \
             patch('sys.stdout', new=io.StringIO()):
            check.check()

    def test_valid_tls_permanent_redirect_metadata_and_live_detail(self):
        self.run_check()
        self.run_check(redirect_status=308)

    def test_temporary_redirect_and_lost_query_are_rejected(self):
        for options in [{'redirect_status': 302}, {'location': check.ORIGIN + '/'},
                        {'location': 'http://' + check.HOST + '/'}]:
            with self.subTest(options=options), self.assertRaises(RuntimeError):
                self.run_check(**options)

    def test_wrong_metadata_origin_and_untrusted_tls_fail(self):
        for options in [{'canonical': 'http://' + check.HOST + '/about'},
                        {'sitemap': 'https://unrelated.example/courses'}]:
            with self.subTest(options=options), self.assertRaises(RuntimeError):
                self.run_check(**options)
        with self.assertRaises(urllib.error.URLError):
            self.run_check(tls_error=True)


if __name__ == '__main__':
    unittest.main()
