#!/usr/bin/env python3
"""Reproduce the embedded public blocklist from its pinned MIT-licensed input."""
import argparse,hashlib,pathlib
SOURCE_SHA256='68782d6a4a19a4768d5f15dd66bd534e7a33055cc755411e33f16d18c50fdcce'
p=argparse.ArgumentParser(description=__doc__);p.add_argument('source');a=p.parse_args()
data=pathlib.Path(a.source).read_bytes()
if hashlib.sha256(data).hexdigest()!=SOURCE_SHA256 or not data.isascii():raise SystemExit('Pinned source checksum/encoding mismatch')
entries=sorted({hashlib.sha256(line.strip().lower()).digest() for line in data.splitlines() if line.strip()})
root=pathlib.Path(__file__).resolve().parents[1]
output=root/'backend/auth/password/blocklist/common.sha256'
output.write_bytes(b''.join(entries))
print('Public blocklist entries:',len(entries),'artifact SHA256:',hashlib.sha256(output.read_bytes()).hexdigest())
