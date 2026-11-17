#!/usr/bin/env python3
"""Opt-in switch to native Yandex Disk API. Enter OAuth only in a root console."""
import configparser
import getpass
import io
import json
import os
from pathlib import Path
import secrets
import subprocess as sp
import time
import zipfile
import sys
import urllib.request
import urllib.error
import urllib.parse

C = Path('/etc/devcourse-backup')

def call(args, *, data=None, repository=None, config_path=None, phase='command'):
    env = dict(os.environ, RCLONE_CONFIG=str(config_path or C/'rclone.conf'), RESTIC_PASSWORD_FILE=str(C/'password'))
    env['PATH']=str(Path(__file__).parent/'bin')+os.pathsep+env.get('PATH','/usr/bin:/bin')
    if repository: env['RESTIC_REPOSITORY'] = repository
    try:
        p = sp.run(args, input=data, capture_output=True, env=env, timeout=120)
    except sp.TimeoutExpired:
        raise RuntimeError(phase + ': timed out after 120 seconds') from None
    if p.returncode:
        error=p.stderr.decode(errors='replace').lower()
        hint=''
        if 'refresh token' in error or 'refreshtoken' in error: hint='; rclone requires a usable OAuth refresh token'
        elif '401' in error or 'unauthorized' in error: hint='; authorization rejected (401)'
        elif '403' in error or 'forbidden' in error: hint='; permission denied (403)'
        elif '404' in error or 'not found' in error: hint='; repository path not found'
        elif '410' in error: hint='; Yandex API reports resource unavailable (410)'
        elif 'invalid character' in error: hint='; unexpected configuration or API response format'
        raise RuntimeError(phase + ': ' + Path(args[0]).name + ' exit ' + str(p.returncode) + hint)
    return p.stdout


def parse_token(raw):
    raw=raw.strip()
    if raw.startswith('{'):
        token=json.loads(raw)
    else:
        if raw.startswith(('https://','http://')):
            raw=urllib.parse.parse_qs(urllib.parse.urlsplit(raw).fragment).get('access_token',[''])[0]
        if raw.startswith(('OAuth ','Bearer ')): raw=raw.split(' ',1)[1]
        raw=raw.strip().strip('"').strip("'")
        token={'access_token':raw,'token_type':'OAuth'}
    if not isinstance(token,dict) or not isinstance(token.get('access_token'),str) or not token['access_token'].strip():
        raise RuntimeError('Input: empty or invalid OAuth token')
    if any(c.isspace() for c in token['access_token']):
        raise RuntimeError('Input: token contains whitespace; paste only a token, redirect URL or token JSON')
    return token


def check_api(token):
    path=urllib.parse.urlencode({'path':'disk:/backups/devcourse-finder/config'})
    request=urllib.request.Request('https://cloud-api.yandex.net/v1/disk/resources?'+path,
                                  headers={'Authorization':'OAuth '+token['access_token']})
    try:
        with urllib.request.urlopen(request,timeout=30) as response:
            if response.status!=200: raise RuntimeError('API metadata: unexpected HTTP status')
    except urllib.error.HTTPError as error:
        hints={401:'token rejected or expired',403:'token lacks access to this Disk path',404:'backup folder absent in the authorized account',410:'Yandex API reports resource unavailable'}
        raise RuntimeError('API metadata: HTTP '+str(error.code)+'; '+hints.get(error.code,'request rejected')) from None
    except (TimeoutError,urllib.error.URLError):
        raise RuntimeError('API metadata: network or TLS connection failed') from None


def main():
    if os.geteuid() != 0: raise RuntimeError('Run as root')
    os.umask(0o077)
    conf = C/'rclone.conf'
    candidate=C/'rclone-api-candidate.conf'
    previous=conf.read_bytes()
    config=json.loads((C/'config.json').read_text())
    oldrepo=config['repository']
    pinned_id=C/'repository-id'
    if pinned_id.exists():
        oldid=pinned_id.read_text().strip()
    else:
        print('Checking existing repository...',flush=True)
        oldid=json.loads(call(['restic','cat','config'],repository=oldrepo,phase='Existing repository'))['id']
        pinned_id.write_text(oldid+'\n');pinned_id.chmod(0o600)
    if '--retry' in sys.argv:
        if not candidate.exists(): raise RuntimeError('No saved candidate; run without --retry for hidden input')
        parser=configparser.ConfigParser(interpolation=None);parser.read(candidate)
        token=json.loads(parser['yandex-api']['token'])
    else:
        token=parse_token(getpass.getpass('Yandex Disk OAuth token or rclone token JSON (hidden): '))
        parser=configparser.ConfigParser(interpolation=None);parser.read_string(previous.decode())
        parser['yandex-api']={'type':'yandex','token':json.dumps(token)}
        buf=io.StringIO();parser.write(buf)
        pending=candidate.with_suffix('.pending');pending.write_text(buf.getvalue());pending.chmod(0o600);pending.replace(candidate)
    repo='rclone:yandex-api:backups/devcourse-finder'
    committed=False
    active_swapped=False
    try:
        print('Checking API access to the existing backup folder...',flush=True)
        check_api(token)
        print('Checking encrypted repository through rclone API backend...',flush=True)
        newid=json.loads(call(['restic','cat','config'],repository=repo,config_path=candidate,phase='API repository'))['id']
        if newid!=oldid: raise RuntimeError('Repository identity mismatch')
        path='yandex-api:backups/devcourse-api-check-'+secrets.token_hex(8)+'.tmp'
        start=time.monotonic()
        call(['rclone','rcat',path,'--timeout','60s','--retries','1','--low-level-retries','1'],data=os.urandom(1048576),config_path=candidate,phase='API upload')
        elapsed=round(time.monotonic()-start,1)
        call(['rclone','deletefile',path],config_path=candidate,phase='API test cleanup')
        print('Native API repository identity verified; 1MiB upload seconds:',elapsed,flush=True)
        # Do not change a live job's repository selection until validation is complete.
        sp.run(['systemctl','stop','devcourse-initial-backup.service'],check=True)
        conf.write_bytes(candidate.read_bytes());conf.chmod(0o600)
        active_swapped=True
        call(['restic','unlock'],repository=repo,config_path=candidate,phase='Remove stale repository locks')
        config['repository']=repo
        pending=C/'config.json.pending';pending.write_text(json.dumps(config)+'\n');pending.chmod(0o600);pending.replace(C/'config.json')
        committed=True
        candidate.unlink(missing_ok=True)
        kit=Path('/var/lib/devcourse-backup/recovery-kit.zip')
        replacement=kit.with_suffix('.pending.zip')
        with zipfile.ZipFile(replacement,'w',zipfile.ZIP_DEFLATED) as z:
            for name in ['config.json','rclone.conf','password','email.json','repository-id']:
                if (C/name).is_file(): z.write(C/name,'etc/devcourse-backup/'+name)
            for p in Path('/opt/devcourse-backup').rglob('*'):
                if p.is_file() and '__pycache__' not in p.parts:
                    z.write(p,'tools/'+str(p.relative_to('/opt/devcourse-backup')))
            z.writestr('READ-ME-FIRST.txt','CONFIDENTIAL: encryption key and cloud access credentials. Store independently in your password manager. Never upload this archive into the backup repository. Follow tools/README.md.\n')
        replacement.chmod(0o600);replacement.replace(kit)
        unit=Path('/etc/systemd/system/devcourse-initial-backup.service')
        unit.write_text('[Unit]\nDescription=Initial encrypted backup and remote recovery validation\nAfter=docker.service network-online.target\n[Service]\nType=oneshot\nExecStart=/opt/devcourse-backup/finish-first-backup.sh\nUMask=0077\nNice=10\nMemoryMax=700M\nTimeoutStartSec=8h\n')
        sp.run(['systemctl','reset-failed','devcourse-initial-backup.service'],check=False,stdout=sp.DEVNULL,stderr=sp.DEVNULL)
        sp.run(['systemctl','daemon-reload'],check=True)
        sp.run(['systemctl','start','--no-block','devcourse-initial-backup.service'],check=True)
        print('Native API enabled; initial backup/drill resumed. Download the updated root-only recovery kit to your password manager.',flush=True)
    finally:
        if active_swapped and not committed:
            conf.write_bytes(previous);conf.chmod(0o600)

if __name__=='__main__':
    try: main()
    except Exception as e:
        print('Native API setup failed:',type(e).__name__,str(e) if isinstance(e,RuntimeError) else '(values hidden)')
        raise SystemExit(1)
