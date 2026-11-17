#!/usr/bin/env python3
"""Reproduce the pinned access-token-compatible rclone client (Linux amd64)."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request


def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''): h.update(block)
    return h.hexdigest()


def download(url, path, expected):
    urllib.request.urlretrieve(url,path)
    if digest(path)!=expected: raise RuntimeError('Downloaded source checksum mismatch')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,default=Path(__file__).parent/'bin')
    parser.add_argument('--toolchain',type=Path)
    parser.add_argument('--cache',type=Path)
    parser.add_argument('--ensure',action='store_true',help='Verify and reuse an existing pinned binary')
    args=parser.parse_args()
    spec=json.loads(Path(__file__).with_name('rclone-build.json').read_text())
    if args.ensure and (args.output/'rclone').exists():
        if digest(args.output/'rclone')!=spec['binary_sha256']: raise RuntimeError('Existing backup client checksum mismatch')
        print('Pinned backup client checksum verified')
        return
    with tempfile.TemporaryDirectory(prefix='devcourse-rclone-') as directory:
        work=Path(directory)
        if args.toolchain:
            go=args.toolchain.resolve()
            version=subprocess.check_output([go,'version']).decode()
            if spec['go_version'] not in version: raise RuntimeError('Go toolchain version mismatch')
        else:
            archive=work/'go.tar.gz';download(spec['go_url'],archive,spec['go_sha256'])
            with tarfile.open(archive) as t:t.extractall(work,filter='data')
            go=work/'go/bin/go'
        archive=work/'source.tar.gz';download(spec['source_url'],archive,spec['source_sha256'])
        with tarfile.open(archive) as t:t.extractall(work/'source',filter='data')
        source=work/'source'/('rclone-'+spec['upstream_commit'])
        file=source/'backend/yandex/yandex.go';text=file.read_text()
        original='if token.RefreshToken == "" {'
        if text.count(original)!=1: raise RuntimeError('Unexpected upstream token validation implementation')
        file.write_text(text.replace(original,'if token.RefreshToken == "" && token.AccessToken == "" {'))
        cache=args.cache.resolve() if args.cache else work/'cache'
        env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',GOAMD64='v1',GOMAXPROCS='2',GOTOOLCHAIN='local',
                 GOCACHE=str(cache/'build'),GOMODCACHE=str(cache/'modules'))
        binary=work/'rclone'
        subprocess.run([go,'build','-buildvcs=false','-p','2','-trimpath','-ldflags',
                        '-s -w -X github.com/rclone/rclone/fs.Version='+spec['upstream_tag']+'-devcourse-access-token',
                        '-o',binary,'.'],cwd=source,env=env,check=True)
        if digest(binary)!=spec['binary_sha256']: raise RuntimeError('Built binary differs from the recorded reproducible build')
        args.output.mkdir(parents=True,exist_ok=True)
        shutil.copy2(binary,args.output/'rclone');(args.output/'rclone').chmod(0o755)
        shutil.copy2(source/'COPYING',args.output/'COPYING')
        shutil.copy2(Path(__file__).with_name('rclone-access-token.patch'),args.output/'access-token.patch')
        shutil.copy2(Path(__file__).with_name('rclone-build.json'),args.output/'provenance.json')
        print('Pinned backup rclone client built and checksum verified')


if __name__=='__main__':main()
