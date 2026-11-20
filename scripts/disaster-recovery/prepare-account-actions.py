#!/usr/bin/env python3
"""Prepare private account configuration. Never enables routes or sends email."""
import base64
import fcntl
import getpass
import json
import os
from pathlib import Path
import re
import secrets
import sys
import tempfile
from urllib.parse import urlsplit
import recovery as r
from smtp_endpoint import endpoint_from_environment


def main():
    os.umask(0o077)
    if os.geteuid()!=0:
        raise RuntimeError('Run as root')
    if (r.BASE/'.account-actions-enabled').exists() or (r.BASE/'.account-actions-enabled').is_symlink():
        raise RuntimeError('Preparation requires inactive account actions')
    current=(r.BASE/'current').resolve(strict=True)
    if not (current/'compose.account-actions.yaml').is_file():
        raise RuntimeError('Deploy the account-action release before preparing')
    with (r.STATE/'operation.lock').open('a') as operation:
        fcntl.flock(operation,fcntl.LOCK_EX)
        with (r.BASE/'.deploy.lock').open('a') as deploy:
            fcntl.flock(deploy,fcntl.LOCK_EX)
            paths=r.password_secret_paths()
            if not paths:
                raise RuntimeError('Canonical password keyring/probe required')
            envfile=r.BASE/'.env'
            info=envfile.lstat()
            if envfile.is_symlink() or info.st_uid!=0 or info.st_mode&0o077:
                raise RuntimeError('Production environment must be root-private')
            original=envfile.read_text()
            def value(name):
                found=re.findall(r'^(?:export\s+)?'+re.escape(name)+r'\s*=\s*(.*?)\s*$',original,re.M)
                if len(found)>1:
                    raise RuntimeError('Duplicate account environment fields')
                if not found:return None
                raw=found[0]
                if len(raw)>=2 and raw[0]==raw[-1] and raw[0] in ('"',"'"):raw=raw[1:-1]
                return raw
            encoded=value('AUTH_GATEWAY_SECRET')
            if encoded is None:encoded=base64.b64encode(secrets.token_bytes(32)).decode()
            if len(base64.b64decode(encoded,validate=True))!=32 or base64.b64encode(base64.b64decode(encoded)).decode()!=encoded:
                raise RuntimeError('Invalid existing gateway key')
            canonical=r.account_mail_secret_path()
            if canonical is not None:
                config=json.loads(canonical.read_text())
            else:
                sender=os.environ.get('AUTH_SMTP_USER')
                password=os.environ.get('AUTH_SMTP_PASSWORD')
                if not sender and sys.stdin.isatty():sender=input('SMTP sender email: ').strip()
                if not password and sys.stdin.isatty():password=getpass.getpass('SMTP password (hidden): ')
                if not sender or not password:
                    raise RuntimeError('Add mail-scoped SMTP credentials through protected environment or hidden terminal input')
                host,port=endpoint_from_environment('AUTH_SMTP')
                config={'version':1,'host':host,'port':port,'username':sender,'from':sender,'password':password,'public_origin':os.environ.get('AUTH_PUBLIC_ORIGIN','https://devcoursefinder.ru')}
            api=r.run(['docker','ps','-q','--filter','label=com.docker.compose.project=devcourse-finder','--filter','label=com.docker.compose.service=api']).decode().strip()
            if not api or '\n' in api:raise RuntimeError('Expected one API container')
            image=r.run(['docker','inspect','--format','{{.Image}}',api]).decode().strip()
            with tempfile.TemporaryDirectory(prefix='account-prepare-',dir=r.STATE) as directory:
                bundle=Path(directory)
                for path in paths:r.copy_path(path,bundle/'rootfs')
                candidate=bundle/'rootfs/srv/devcourse-finder/secrets/auth-smtp.json'
                r.write(candidate,config)
                r.verify_password_secrets(bundle,image)
                r.verify_account_mail_secret(bundle,image)  # offline only, no SMTP connection
                origin=urlsplit(config['public_origin']);public_origin='https://'+origin.hostname
                update={'AUTH_GATEWAY_SECRET':encoded,'AUTH_PUBLIC_ORIGIN':public_origin}
                kept=[line for line in original.splitlines() if not re.match(r'^(?:export\s+)?(?:AUTH_GATEWAY_SECRET|AUTH_PUBLIC_ORIGIN)\s*=',line)]
                replacement='\n'.join(kept+[key+'='+val for key,val in update.items()])+'\n'
                descriptor,temporary=tempfile.mkstemp(prefix='.account-env-',dir=r.BASE)
                try:
                    with os.fdopen(descriptor,'w') as output:output.write(replacement);output.flush();os.fsync(output.fileno())
                    Path(temporary).replace(envfile)
                finally:Path(temporary).unlink(missing_ok=True)
                if canonical is None:r.write(r.BASE/'secrets/auth-smtp.json',config)
                for filename in ('password-keyring.json','auth-smtp.json'):
                    r.password_runtime_file(bundle,image,r.BASE/'secrets/runtime'/filename,filename)
            r.refresh_recovery_kit()
    print('Private configuration prepared; routes and mail remain disabled. Complete encrypted backup, restore, delivery and independent-recovery checks before activation.',flush=True)


if __name__=='__main__':
    try:main()
    except Exception:
        print('Account preparation failed; values hidden. Existing public deployment remains disabled.',file=sys.stderr)
        sys.exit(1)
