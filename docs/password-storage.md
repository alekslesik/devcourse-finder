# Password storage foundation

Tracking: [epic #207](https://github.com/alekslesik/devcourse-finder/issues/207),
[foundation #208](https://github.com/alekslesik/devcourse-finder/issues/208).

This phase adds `backend/auth/password` as a library. It does not create users,
database tables, production keys, registration routes or login routes. Public
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
