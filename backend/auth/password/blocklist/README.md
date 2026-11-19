# Offline common-password blocklist

The binary contains sorted, unique SHA-256 digests of ASCII-lowercased,
whitespace-trimmed entries from SecLists `10k-most-common.txt`, under the included
MIT license. These are public common-password hashes, not user credentials.

- Source: https://github.com/danielmiessler/SecLists/blob/abe37a24be7dd50d676840c9f63dca87de5117f8/Passwords/Common-Credentials/10k-most-common.txt
- Source SHA256: `68782d6a4a19a4768d5f15dd66bd534e7a33055cc755411e33f16d18c50fdcce`
- Artifact: 10001 unique digests; SHA256 `2028d1275f90841faedccfb337ffca4c4971b787f0b0514475633d3a468de05e`
- Regenerate: `python3 scripts/build-password-blocklist.py DOWNLOADED_SOURCE`

Password length/UTF-8 validation runs first. Blocklist comparison normalizes only
for rejection; accepted password bytes are still hashed exactly as entered. This
is a bounded common-password baseline, not a complete breached-password corpus.
No raw password, prefix or hash is sent to an external service.

`known-common.txt` is the public long-password test vector copied from the pinned
source above, under the same MIT license. It is not an account credential.
