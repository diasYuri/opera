# ADR 006 — Credentials and local trust

Status: storage/CLI implemented in F0; Y credential resolver/OAuth wiring is F1.

Use the native system keyring by default via go-keyring v0.2.8 (macOS Keychain,
Linux Secret Service and Windows Credential Manager). Failure is explicit and
never switches to disk automatically. Native availability is checked only by an
explicit auth operation; doctor/config do not unlock or read keyrings. This also
keeps CI independent from user accounts and desktop services.

`--store file` or user `credentials.store='file'` explicitly selects an unencrypted
filesystem fallback. Records live under the application data directory, outside
the workspace, named by a SHA-256 of the reference. The data/credential directories
and each record must be private. Unix checks current ownership, 0700 directories,
0600 files and rejects symlinks; Windows sets owner to the current user and a
protected DACL with one user-only grant, rejecting broad/null ACLs or other owners.
Unsafe existing permissions fail closed and are not silently repaired. Directory
locations can be inspected with `config paths`. Choose user-controlled parents.

Writes use a restricted temporary file, sync/close and atomic rename, with one
record per reference. Independent reference writes do not overwrite one another.
Same-reference concurrent updates are last-writer-wins; there is no multi-record
transaction. Filesystem snapshots/backups can still contain these unencrypted
secrets. OS administrators and processes running as the same user are outside
this access-control boundary. Deletion is not guaranteed secure erasure.

The public Store boundary supports Set/Get/Delete. CLI set accepts non-TTY stdin
only, capped at 64 KiB; status displays presence and delete needs an explicit name.
There is no CLI get/reveal. Never pass raw secrets as command flags. Keyring native
error output is suppressed because it may contain sensitive subprocess data.
TOML accepts only a credential reference; unknown secret fields are rejected
without printing their values. Credentials must never reach event/log/snapshot
or session exports when execution is introduced.
