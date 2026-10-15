#!/usr/bin/env python3
"""Assert public TLS, exact HTTP redirects and metadata origin without bypassing trust."""
import json
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

HOST = 'alekslesik.fvds.ru'
ORIGIN = 'https://' + HOST


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check():
    paths = ['/', '/courses?language=python&goal=try', '/about', '/api/v1/courses', '/sitemap.xml']
    with urllib.request.urlopen(ORIGIN + '/api/v1/courses', timeout=25) as response:
        catalog = json.load(response)
    if not catalog.get('items'):
        raise RuntimeError('A live course is required to verify detail HTTPS')
    paths.append('/courses/' + catalog['items'][0]['course']['slug'])
    redirect = urllib.request.build_opener(NoRedirect)
    for path in paths:
        try:
            redirect.open('http://' + HOST + path, timeout=25)
        except urllib.error.HTTPError as response:
            if response.code not in [301, 308] or response.headers.get('Location') != ORIGIN + path:
                raise RuntimeError(f'Incorrect permanent redirect for {path}: {response.code}')
        else:
            raise RuntimeError(f'HTTP did not redirect: {path}')
        with urllib.request.urlopen(ORIGIN + path, timeout=25) as response:
            if response.status != 200 or not response.url.startswith(ORIGIN + '/'):
                raise RuntimeError(f'HTTPS failed: {path}')
            body = response.read().decode()
        if path == '/sitemap.xml':
            locations = [n.text for n in ET.fromstring(body).iter() if n.tag.endswith('}loc')]
            if not locations or any(not loc.startswith(ORIGIN + '/') for loc in locations):
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
            if getattr(metadata, 'canonical', None) != ORIGIN + '/about':
                raise RuntimeError('Canonical origin differs')
            image = getattr(metadata, 'image', '')
            if not image.startswith(ORIGIN + '/'):
                raise RuntimeError('Social image has an inconsistent origin')
            with urllib.request.urlopen(image, timeout=25) as response:
                if response.status != 200:
                    raise RuntimeError('Social image is unavailable')
        print('PASS', path, 'trusted HTTPS + exact permanent HTTP redirect')


if __name__ == '__main__':
    check()
