"""Operator-configured SMTP DNS endpoint; all supported modes require TLS."""
import ipaddress
import os
import re


def endpoint_from_environment(prefix):
    host = os.environ.get(prefix + '_HOST', 'smtp.yandex.ru')
    port = os.environ.get(prefix + '_PORT', '465')
    labels = host.split('.')
    if len(host) > 253 or len(labels) < 2 or any(
        not re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label)
        for label in labels
    ):
        raise RuntimeError('Invalid SMTP DNS hostname; values hidden')
    try:
        ipaddress.ip_address(host)
    except ValueError:
        pass
    else:
        raise RuntimeError('SMTP requires a DNS hostname; values hidden')
    if port not in ('465', '587'):
        raise RuntimeError('SMTP requires verified TLS on port 465 or 587')
    return host, int(port)
