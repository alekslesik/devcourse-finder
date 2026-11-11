#!/usr/bin/env python3
"""Assert public TLS, exact HTTP redirects and metadata origin without bypassing trust."""
import argparse
import json
import re
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

HOST = 'devcoursefinder.ru'
ORIGIN = 'https://' + HOST


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check(host=None):
    host = host or HOST
    if not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}', host):
        raise ValueError('Expected a lowercase DNS hostname')
    origin = 'https://' + host
    paths = ['/', '/courses?language=python&goal=try', '/about', '/api/v1/courses', '/sitemap.xml']
    with urllib.request.urlopen(origin + '/api/v1/courses', timeout=25) as response:
        catalog = json.load(response)
    if not catalog.get('items'):
        raise RuntimeError('A live course is required to verify detail HTTPS')
    paths.append('/courses/' + catalog['items'][0]['course']['slug'])
    redirect = urllib.request.build_opener(NoRedirect)
    for path in paths:
        try:
            redirect.open('http://' + host + path, timeout=25)
        except urllib.error.HTTPError as response:
            if response.code not in [301, 308] or response.headers.get('Location') != origin + path:
                raise RuntimeError(f'Incorrect permanent redirect for {path}: {response.code}')
        else:
            raise RuntimeError(f'HTTP did not redirect: {path}')
        with urllib.request.urlopen(origin + path, timeout=25) as response:
            if response.status != 200 or not response.url.startswith(origin + '/'):
                raise RuntimeError(f'HTTPS failed: {path}')
            body = response.read().decode()
        if path == '/sitemap.xml':
            locations = [n.text for n in ET.fromstring(body).iter() if n.tag.endswith('}loc')]
            if not locations or any(loc != origin and not loc.startswith(origin + '/') for loc in locations):
                raise RuntimeError('Sitemap has an inconsistent origin')
        elif path == '/about':
            from html.parser import HTMLParser
            class Metadata(HTMLParser):
                def handle_starttag(self, tag, attrs):
                    attrs = dict(attrs)
                    if tag == 'link' and attrs.get('rel') == 'canonical':
                        self.canonical = attrs.get('href')
                    if tag == 'meta' and attrs.get('property') == 'og:image':
                        self.image = attrs.get('content')
            metadata = Metadata()
            metadata.feed(body)
            if getattr(metadata, 'canonical', None) != origin + '/about':
                raise RuntimeError('Canonical origin differs')
            image = getattr(metadata, 'image', '')
            if not image.startswith(origin + '/'):
                raise RuntimeError('Social image has an inconsistent origin')
            with urllib.request.urlopen(image, timeout=25) as response:
                if response.status != 200:
                    raise RuntimeError('Social image is unavailable')
        print('PASS', path, 'trusted HTTPS + exact permanent HTTP redirect')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', default=HOST)
    check(parser.parse_args().host)
