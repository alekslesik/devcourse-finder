# Password storage foundation

Tracking: [epic #207](https://github.com/alekslesik/devcourse-finder/issues/207),
[foundation #208](https://github.com/alekslesik/devcourse-finder/issues/208).

The initial foundation adds `backend/auth/password` as a library. Phase 2 below
adds protected key files and recovery tools. Neither phase creates users,
database tables, registration routes or login routes. Public
authentication must remain disabled until the integration and recovery requirements
below are implemented.

## Credential format and verification

Store `Credential.Scheme`, `PepperKeyID` and `PHC` in private database columns.
The scheme is `argon2id-hmac-sha256-v1`:

1. HMAC-SHA256 with a separate pepper over
   `devcourse-finder/password/v1\x00` followed by the exact UTF-8 password bytes.
2. Argon2id v19 over the 32-byte HMAC output, with a fresh `crypto/rand` salt
   of 16 bytes and a 32-byte derived hash.
3. PHC stores the Argon2 version, costs, salt and derived hash. The non-secret
   pepper key ID is stored separately; pepper bytes never belong in the database.

`Hash` requires at least 15 Unicode code points and at most 1024 UTF-8 bytes.
Spaces are significant; no trimming, truncation or Unicode normalization occurs.
`Verify` accepts shorter historical passwords, but still rejects invalid UTF-8
and oversized input. The future registration/reset service must additionally
reject common and compromised passwords before hashing, without sending raw
passwords to external services.

Verification uses a constant-time derived-hash comparison. A wrong password
returns `Match=false` and never triggers replacement. A correct password with an
older pepper or weaker memory/time cost returns a fresh `Replacement` credential.
Persist that replacement with an atomic compare-and-swap against the original
credential, so a concurrent password reset cannot be overwritten. Memory and
iteration costs never decrease during an upgrade; parallelism follows current
configuration. A missing historical key fails closed.

## Resource limits and measured defaults

Defaults: 65536 KiB (64 MiB), three iterations, one lane. The parser accepts only
Argon2id v19, at most 256 PHC bytes, 16–32 salt bytes, a 32-byte derived hash,
canonical decimal parameters and costs within these bounds:

| Setting | Accepted range |
| --- | --- |
| Memory | 19456–131072 KiB |
| Iterations | 2–6 |
| Parallelism | 1–4 |
| Concurrent operations per shared hasher | 1–2 |

Use **one shared hasher per process, concurrency 1 on the current VDS**. Hashing,
verification and upgrades share the gate. Overload returns `ErrBusy` immediately,
without an unbounded queue. Apply request-body limits and rate limits before
calling the library; choose an explicit temporary-overload HTTP response during
API integration. Multiple processes each have their own gate and memory budget.

Cancellation is checked before and after work; an already-running Argon2
calculation cannot be interrupted and retains its slot until completion. An
upgrade may run two KDF calculations sequentially. Limits bound individual KDF
cost and concurrent operations, not total process RSS: Go garbage collection may
retain previous allocations. Budget container memory and measure the integrated
service before activation.

On 2026-11-17, a standalone Linux/amd64 test binary built with Go 1.27.4 was
measured on the VDS (Intel Xeon E5-2680 v2), `GOMAXPROCS=1`, `nice -n 10`, five
iterations per benchmark, synthetic password/key only:

| Operation | Mean time | Allocated bytes/operation |
| --- | --- | --- |
| Hash | 423 ms | 67112355 |
| Verify, no upgrade | 396 ms | 67112048 |

This is a baseline for the library, not endpoint throughput or an RSS measurement.
Retain the defaults initially and repeat under realistic concurrent application
load before exposing authentication. Reproduce with `go test -c` for the package,
then `GOMAXPROCS=1 ./password.test -test.run='^$'
-test.bench='Benchmark(Hash|Verify)Default$' -test.benchtime=5x -test.count=1`.

## Pepper lifecycle

`New` requires an explicit active key ID and a keyring with 1–8 keys, each containing
32–64 cryptographically random bytes. Use a CSPRNG; repeated test bytes are test
fixtures only. Key IDs contain 1–32 ASCII letters, digits, underscores or hyphens.
The hasher copies the supplied keys and uses immutable state concurrently.

Production integration must load the keyring from a root/service-readable secret
file outside Git and the database, with restrictive permissions. Use distinct
keys for pepper, backup encryption, sessions and mail. Do not generate a new key
on application startup or silently fall back to an unpeppered scheme.

Rotation adds a new active key and retains old verification keys. Successful
logins migrate credentials incrementally. Retire a key only after all affected
credentials have migrated or been reset. Also retain protected historical keys
needed by historical database backups; zero live references alone is insufficient.
Loss of a required key means affected credentials cannot be verified and require
account recovery. Pepper does not protect against full application-server compromise.

The encrypted backup must include the keyring/configuration and document their
relationship with database snapshots. Keep an independent protected copy outside
the VDS. Verify a restore with both old and new credentials after rotation before
considering it complete. This phase intentionally creates no production pepper.

## Integration and activation requirements

- Add private credential persistence and atomic replacement; never expose hashes
  or key IDs through user-facing account serializers.
- Add verified-email registration, common/compromised-password rejection and
  bounded password-reset flows with hashed, expiring, atomically consumed tokens.
- Add account-enumeration protection, including bounded dummy verification for
  unknown accounts, uniform public errors, rate limits and abuse controls.
  Internal library errors must not be returned directly to clients.
- Add secure sessions, CSRF protection, session revocation on reset, and admin
  2FA with protected recovery codes.
- Configure backup email notifications, confirm independent storage of the
  recovery kit, include real pepper keys in encrypted recovery, and test isolated
  recovery before enabling public registration.

Credential/hasher formatting and credential JSON serialization redact sensitive
fields as a guard against accidental output. Explicitly logging `PHC`, key bytes,
request bodies or raw passwords still leaks them and is forbidden. Clearing
temporary byte slices is best effort; Go strings and garbage collection do not
provide a guarantee of secret erasure from memory.

Checks: `go test -race ./backend/auth/password` and
`go vet ./backend/auth/password`. Tests cover exact input bytes, independent salts,
wrong passwords/keys, key rotation, cost preservation, unsafe PHC rejection,
overload, cancellation, concurrent access, input policy and redaction.

## Administrative keyring files (phase 2)

Commands below use the release binary `devcourse-finder` on a trusted Linux host,
as root for the production path. They execute without database access and print
only success/failure, never keys or credential contents. Use a private directory
outside Git and Docker build contexts:

```sh
install -d -m 0700 /srv/devcourse-finder/secrets
devcourse-finder password-keyring init /srv/devcourse-finder/secrets/password-keyring.json pepper-2026-01
devcourse-finder password-keyring check /srv/devcourse-finder/secrets/password-keyring.json
devcourse-finder password-keyring probe /srv/devcourse-finder/secrets/password-keyring.json /srv/devcourse-finder/secrets/password-recovery.json
devcourse-finder password-keyring verify /srv/devcourse-finder/secrets/password-keyring.json /srv/devcourse-finder/secrets/password-recovery.json
```

Keyring JSON has exactly `version` (1), `active_key_id`, and `keys` (ID to canonical
standard-base64 key bytes). Duplicate/unknown fields, oversized input, symlinks,
non-regular files, permissions for group/others and unexpected file owners are
rejected. Files must be owned by root or the invoking user, mode 0600 or 0400.
Parent directories must be trusted. Generation publishes a completed, fsynced file
without replacing any existing path; startup never generates keys. A failed
post-publication directory fsync can leave a complete file: inspect with `check`
before retrying, rather than deleting it.

`PASSWORD_STORAGE_ENABLED` accepts only absent/`false` or `true`.
When false no key is required. When true, API startup validates
`PASSWORD_KEYRING_FILE` before opening the database and fails closed on an error.
This is a configuration preflight, **not registration/login activation**. No
public authentication routes exist yet. Future API integration must retain the
shared hasher and configure its non-root production mount. Phase 3 recovery tools
prepare a private projection and remap the mount to restored snapshot files in
the isolated application drill.
Do not enable the production flag before that integration is accepted.

### Rotation

`password-keyring rotate SOURCE NEW_PATH NEW_ID` generates a new 32-byte key,
retains all historical keys and writes a new candidate file without changing the
active source. It refuses duplicate IDs, overwrite or more than eight keys.
Generate a new probe for the candidate; it includes a synthetic credential for
**every** retained key. Test verification and historical credential upgrades
before activation. Probe passwords are public synthetic fixtures, never user
passwords. Probe hashes still belong in protected files and encrypted snapshots.

Serialize activation with `/srv/devcourse-finder/.deploy.lock`, the same lock used
by deployment and full backups. Move the validated candidate keyring and probe
into the two canonical paths while holding that lock. Update the service only
after both files are installed. Retain a protected copy of the previous pair.
Make an encrypted checkpoint and test restore before retiring anything. Never
remove a historical key merely because live credentials have migrated: older
database snapshots can still require it. Compromise of a pepper also requires an
incident response and possibly password resets; rotation alone does not repair
already-stolen data.

### Encrypted recovery

The full backup explicitly captures the canonical keyring/probe pair, validates
it using the pinned API image in a network-disabled, read-only container, and
includes both files in the same checksummed manifest as the database dump.
Partial, unsafe or unverified pairs fail the backup. Isolated and clean-server
recovery validate restored keys/probes before starting the application. Existing
snapshots without password secrets remain compatible.

For a small independent recovery proof, run as root:

```sh
python3 /opt/devcourse-backup/recovery.py password-checkpoint
```

This uploads the keyring, all-key probe, compatible verifier binary and a
checksummed deployment manifest to the existing encrypted restic repository
under `devcourse-password-keys-v1`, then downloads that exact snapshot and verifies
every key. It refreshes the private `/var/lib/devcourse-backup/recovery-kit.zip` with current
pepper files and records `/var/lib/devcourse-backup/last-password-checkpoint.json`.
It **does not contain a database dump**, replace the full backup, advance its
freshness status, or apply full-snapshot retention. Keep checkpoints needed by
historical database backups. Temporary restored plaintext is removed afterwards.

To recover such a checkpoint independently, use the recovery kit's compatible
restic/rclone setup and backup encryption password. List snapshots with tag
`devcourse-password-keys-v1`, restore the chosen ID into a private directory,
validate the manifest checksums, then run the included verifier with
`password-keyring verify KEYRING PROBE` against the restored pair. Preserve mode
0600 and root ownership when installing production files. The full application's
database must come from a matching full snapshot and compatible release.

The password-manager/off-VDS copy must be updated after key creation/rotation;
the old recovery kit alone does not contain newly created pepper keys. Access to
Yandex, restic encryption credentials and this instruction must remain available
independently of the VDS. SMTP notification setup and confirmation of independent
secret storage are still requirements before public registration is activated.

## Private user persistence (phase 3)

`auth_users` stores a random UUID, canonical email, creation time and nullable
email-confirmation time. `auth_credentials` has one row per user: scheme, PHC,
pepper ID, credential `revision`, `password_version` and `password_changed_at`.
Deleting a user cascades to credentials. The additive/idempotent migration does
not enable registration, create production accounts or alter catalog data.

Account identifiers accept bare ASCII dot-atom email addresses, at most 254
bytes total and 64 bytes before `@`, with DNS-style domain labels up to 63 bytes.
Surrounding whitespace is trimmed and ASCII case is folded for the whole address.
Display names, quoted local parts and Unicode addresses are rejected; international
domains must already be IDNA ASCII. Plus tags and dots are preserved, without
provider-specific alias collapsing. PostgreSQL enforces the canonical format and
unique address. This is an account-identity policy, not a claim that all mail
providers consider local parts case-insensitive. Ownership still requires a
confirmation email; new rows are always unconfirmed.

`DB.Users()` exposes only private repository operations. `Create` inserts the user
and validated credential together, with no partial user on failure. `ByEmail` and
`ByID` return private records whose default formatting and JSON omit sensitive
fields. Database errors are mapped to fixed errors; SQL details/arguments, PHC,
email and keyring values must not be logged. Request handlers must translate
internal not-found/conflict errors to enumeration-resistant public behaviour.
No new HTTP endpoints or public account DTOs are provided by this phase.

After a correct password check, persist `Verification.Replacement` using
`Upgrade(observed, replacement)`. Its compare-and-swap matches the user ID,
revision, password version and original scheme/PHC/key ID. A stale observation
returns false and cannot overwrite a concurrent reset. Upgrade increases only
the credential revision; it preserves password age and password version.
`ChangePassword` also uses compare-and-swap, increases both counters and advances
the password-change time. It does not itself authorize a reset or revoke sessions:
those flows belong to the future authentication service.

Use `DB.WithUsers` to combine future token consumption and password replacement in
one transaction. Nested creation uses a savepoint and remains subject to outer
rollback. Never issue a session based only on a stale successful hash check:
recheck the password version atomically when creating it, and handle concurrent
credential changes explicitly. Session validation/revocation must use the password
version, not the hash revision, so harmless rehashing does not invalidate sessions.

### Database and secret recovery checks

`./scripts/test-user-recovery.sh` runs against the disposable integration-test
PostgreSQL service. It creates synthetic accounts with old/current pepper keys,
produces an actual custom-format `pg_dump`, restores into a second empty database,
copies the protected keyring/probe, verifies both passwords and upgrades the
historical credential. It is part of backend CI and never uses real accounts.

The isolated VDS drill now creates a separate projection of the restored keyring,
owned by the pinned image's actual non-root UID/GID, mode 0400. The canonical
root-owned 0600 pair stays unchanged. The API receives only that temporary file
through a read-only bind at `/run/secrets/password-keyring.json`, with storage
preflight enabled. The drill never mounts production keys or publishes ports.
This checks the real non-root container's ability to read the recovered keyring.
Temporary projections are deleted during drill cleanup.

Before application startup, restored credential key IDs must all exist in the
restored keyring. The all-key probe verifies the key material itself. Legacy
snapshots without the credential tables remain supported. Database-only restore
checks the current canonical keyring and leaves writers stopped on a missing-key
failure, retaining the pre-restore safety dump.

For future production activation, the only reviewed runtime bind source is
`/srv/devcourse-finder/secrets/runtime/password-keyring.json`, with the fixed target
above, read-only, and the corresponding `PASSWORD_KEYRING_FILE`. Runtime projections
are regenerated from canonical restored keys for the image UID/GID; they are not
copied as root-owned files from a backup. Registration, mail confirmation, reset
tokens, rate limits, sessions and 2FA remain subsequent epic tasks. SMTP backup
notifications and independent recovery-kit storage remain activation gates.

## Account actions and mail intents (phase 4 backend)

This phase provides private backend APIs and a testable delivery step. It creates
no public registration/reset endpoints, mail worker service, UI forms or production
accounts. SMTP credentials are not provisioned and no real emails are sent during
validation. Public activation still requires the remaining epic gates.

`auth_action_tokens` stores only SHA-256 digests of CSPRNG-generated 32-byte bearer
secrets, with a versioned purpose-specific digest domain. Tokens are bound to user,
canonical email and password version. Confirmation lasts 24 hours; reset lasts
15 minutes, using the database clock. Only confirmed addresses can request a
reset. A token is consumed atomically with email confirmation or credential change.
Failures roll back both changes. Concurrent/repeated/expired actions fail with a
fixed internal error. Rehashing changes only the credential revision and leaves a
valid password-version-bound token usable; a real password change invalidates it.

All account actions and credential updates lock the user first, then credentials
and associated jobs/tokens. This prevents lock-order inversion between delivery,
reset, confirmation and pepper upgrades. `WithUsers` remains available for wider
transactions. The future session implementation must enforce password versions;
there are no live sessions to revoke in this phase.

### Outbox without persisted bearer secrets

`auth_mail_jobs` contains delivery intent, recipient, purpose, version, deadlines
and lease metadata; never an email body, raw token or full link. Requests return
the same success for unknown, ineligible, queued and rate-limited addresses.
Per-account/purpose requests have a 60-second cooldown, at most five per hour and
one active intent. Global/IP limits and public response timing remain requirements
for HTTP integration; these repository semantics alone are not enumeration-proof.

`accountmail.Service.SendOne` is an explicitly invoked worker step. It claims a
two-minute lease, generates a fresh token in memory and stores its digest. Sending
has a shorter deadline; completion is fenced by lease ID and digest. There are at
most five attempts with bounded exponential backoff. Expired intents and crashed
final attempts release their reservation. Unknown/changed user state supersedes
the job without sending. Email-confirmation/reset requests expire if not delivered
within their respective request lifetime.

A retry replaces the token: the old link becomes unusable. SMTP is at-least-once:
if DATA was accepted but the process died before database completion, a retry can
send another email and invalidate the older link. If a send reports failure, its
token is invalidated. DATA acceptance counts as success even if QUIT subsequently
fails. Raw tokens are never persisted merely to make SMTP retries identical.
Operators must monitor failed jobs and prune completed/expired intent history
under an explicit retention policy when activating the worker.

Links use a configured HTTPS origin and fixed paths with `#token=...`. Request
Host headers never select the origin. SMTP revalidates the exact origin, path,
purpose and token encoding. The future page must scrub the fragment before
analytics, set `Referrer-Policy: no-referrer`, and require an explicit CSRF-protected
POST. GET/prefetch must never confirm or reset an account. The opt-in routes below
implement this boundary and remain disabled in ordinary deployments.

### Password policy and SMTP

`accountmail.Service.Reset` checks the token privately before the bounded KDF and
applies the length/UTF-8 policy plus the embedded offline common-password blocklist.
The list is a pinned MIT-licensed public SecLists baseline (10001 unique digests),
not a complete breached-password corpus. Comparison case-folds/trims for rejection;
accepted values are still hashed exactly. No user password or hash leaves the
process for a breach-check service. License/provenance are included in the image;
`devcourse-finder licenses` also prints the notice from standalone binaries.

The optional canonical config is `/srv/devcourse-finder/secrets/auth-smtp.json`,
owned by root, mode 0600, outside Git/build contexts. It has exactly these JSON
fields (the password is added through secrets/local hidden input, never chat):

- `version`: 1; `host`: the operator-selected canonical lowercase SMTP DNS name;
  `port`: 465 or 587. Yandex remains supported as `smtp.yandex.ru`.
- `username` and `from`: the same canonical mailbox email, e.g. `noreply@devcoursefinder.ru`.
- `password`: the mailbox SMTP credential (for Yandex, a mail-scoped application
  password), separate from the Disk password.
- `public_origin`: the trusted HTTPS site origin, normally `https://devcoursefinder.ru`.

`devcourse-finder auth-mail check-config PATH` validates offline and sends nothing.
The SMTP adapter requires verified TLS 1.2+, implicit TLS on 465 or mandatory
STARTTLS on 587 before AUTH. Certificates/hostnames are checked; errors are fixed
and omit provider responses, recipients, credentials and links. Tests use local
TLS mail doubles only. A non-root production worker needs a reviewed private
projection of this config; the opt-in profile below supplies it. Live activation
is still pending.

### Recovery and replay prevention

The optional SMTP config is included only in the encrypted full snapshot and
private independent recovery kit. Snapshot validation uses the pinned image's
`auth-mail check-config` in a network-disabled container, without SMTP login.
Older snapshots without SMTP config or action tables remain supported.

After every database restore, including isolated drills and database-only restore,
all restored action tokens are consumed and pending/sending mail intents are
superseded **before** application startup. Recovery must not revive a previously
used link or send stale emails from an older database state. Users must request
new links after recovery. The actual pg_dump/pg_restore CI test verifies that a
saved synthetic bearer is rejected after this recovery step and that queued mail
cannot replay. Password/keyring verification still succeeds for old/current keys.
Future sessions must receive equivalent recovery invalidation when implemented.

### Opt-in HTTP forms and mail worker (v0.48)

The implementation now includes `/account/confirm-email` and
`/account/reset-password`, and five `/api/v1/account/*` endpoints. They remain
404 in the ordinary production stack. There is no registration, login or session
implementation in this release. `GET` only displays a form or obtains a CSRF
challenge; changing an account requires an explicit POST.

The document head removes the query/fragment before hydration. A strictly parsed
bearer stays in memory only and is cleared after completion/expiry. Account pages
exclude analytics (including the no-JavaScript pixel), disallow indexing and
external connections, and send no-store/no-referrer headers. Passwords support
paste, password managers and visibility controls. Request-email responses are
uniform; this reduces enumeration but does not promise constant-time database or
SMTP behavior.

The Node route signs method, canonical path, timestamp, nonce, client IP, origin,
CSRF header/cookie and body with a separate random 32-byte gateway secret. The Go
handler verifies the signature and rejects replay, unknown fields, oversized
bodies, cross-origin actions and invalid CSRF. Direct calls through the generic
API rewrite cannot bypass authentication. In-memory limits are global 60/min,
per-IP 12/min (IPv6 /64), and email-request 20/min; they reset on restart and must
be supplemented at ingress for a multi-instance deployment. The ingress must
append/replace the real client IP; the Node proxy uses the rightmost forwarded
address only with `AUTH_TRUSTED_INGRESS=true`. Keep frontend bound to loopback and
API/DB unexposed. Never enable this flag behind an unreviewed proxy.

`compose.account-actions.yaml` supplies the explicitly enabled API/frontend and a
non-root, network-capable mail worker with read-only private projections. The
worker has no public ports, persists retries in the existing outbox and exposes
only a local heartbeat/database health command. Delivery failures emit sanitized
codes, never provider responses or recipients. Monitor outbox failures separately
from process health. The normal release excludes this file.

Preparation after deploying a supporting release:

```sh
sudo python3 /opt/devcourse-backup/prepare-account-actions.py
```

Provide the mailbox address and SMTP credential through protected environment
`AUTH_SMTP_USER` / `AUTH_SMTP_PASSWORD`, or the script's hidden terminal prompt.
For a hosting provider set `AUTH_SMTP_HOST` and `AUTH_SMTP_PORT` to its documented
SMTP endpoint; omitted values retain `smtp.yandex.ru:465`. Existing canonical
configuration takes precedence over these preparation variables.
Do not put it in command arguments or chat. Preparation validates the SMTP config
and pepper offline, creates the gateway key in root-private `.env`, projects
secrets as mode 0400 for the pinned image UID, and refreshes the independent
recovery kit. It does not log in to SMTP, send mail, enable routes or change the
running deployment. An error may leave prepared private configuration; rerunning
is safe while actions are disabled.

Before activation, verify real SMTP delivery with an operator-controlled recipient,
complete a fresh encrypted full backup and isolated restore, export the refreshed
recovery kit independently of the VDS, verify ingress client-IP handling, and
accept the account/session activation scope. Backup-failure email is a separate
configuration and must also be tested. No marker is created automatically.

Only after those gates are met, root can write the literal `enabled` followed by a
newline to `/srv/devcourse-finder/.account-actions-enabled` (regular root-owned
0600 file). Dispatch the published release through **Deploy release**; the deploy
script then includes the optional Compose file and rejects malformed markers or
unsupported releases. To disable, remove the marker and redeploy through the same
workflow; orphan removal stops the mail worker. Do not roll back to a release
without the profile while the marker exists.

Full backups include the profile, canonical secrets and activation marker when
present. Full recovery recreates private worker/API projections. Isolated drills
always exclude the worker, disable HTTP actions and remove the gateway secret
from runtime configuration; they cannot deliver mail. Restored action tokens and
pending mail are invalidated before startup as described above.


### Domain mailbox SMTP (v0.49)

A hosting-provider mailbox can use the same TLS adapter as Yandex. The selected
SMTP hostname is trusted operator configuration, loaded exclusively from the
private file; public HTTP requests cannot override the server, sender or origin.
The canonical DNS hostname pins SNI and certificate verification. URLs, IP
literals, whitespace, uppercase/trailing-dot names and plaintext ports are
rejected. There is no option to disable TLS verification. The username must be
the full mailbox address and must match From; providers requiring a separate
opaque authentication ID need a separately reviewed integration.

Choose the actual managed mailbox service before changing DNS. Obtain its MX,
SPF, DKIM and DMARC instructions and authenticated SMTP endpoint. Keep the site A
record; create the provider's exact records, maintaining a single SPF policy.
DNS hosting alone does not provide a mailbox. Do not invent MX targets or DKIM
keys. DNS/mailbox credentials and recovery access belong in independent protected
storage, with the existing backup recovery material.

The VDS must reach the provider's port 465 or 587. Check provider network policy
when HTTPS works but SMTP connections time out; configuring a mailbox or installing
a local MTA does not remove an upstream SMTP block. For HOSTKEY see
https://hostkey.ru/documentation/faq/network_settings/ and request the documented
SMTP access through the account's support channel as needed.

After mailbox/DNS/network configuration, prepare credentials, verify one technical
message and its SPF/DKIM/DMARC results at the operator's inbox, configure backup
alerts, then make and restore a fresh encrypted copy containing the new mail
configuration. Preparation and release deployment alone do not enable account
routes or send messages. All existing activation gates still apply.
