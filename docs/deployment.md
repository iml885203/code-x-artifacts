# Self-hosting CodeXArtifacts

## Accounts

`codexartifacts init [directory]` creates `users.json` and a random `admin-password`
in a directory readable only by its owner. Both files are created exclusively:
existing files are never overwritten. The service defaults to `.local/users.json`.
Set `PICTOR_USERS_FILE` to use another private file. Restart after editing accounts.

Each JSON entry has `username`, `displayName`, `passwordHash` and `groups`.
Usernames use 1–64 lowercase ASCII letters, numbers, dots, hyphens or underscores.
Use a unique username and bcrypt cost 10–14; plaintext passwords are refused.
To hash a new password without placing it in shell history or process arguments:

```sh
read -rs PASSWORD
printf '%s' "$PASSWORD" | ./bin/codexartifacts hash-password
unset PASSWORD
```

Paste the resulting hash into the private JSON file. Passwords must be 12–72 bytes.
Accounts in the file can sign in; group membership is shared with the existing
artifact authorization model. There is no signup or email password reset.
Removing an account blocks new sign-ins but does not immediately revoke existing
sessions or personal tokens. Local logout revokes only the current browser session.
OIDC deployments can use [subject-wide back-channel logout](oidc.md#subject-wide-back-channel-logout);
PATs and MCP grants remain independently revoked. For a compromise, stop the
service and plan session/credential invalidation before exposing it again.

For centrally managed identities, configure [OIDC browser sign-in](oidc.md), for
example with Pocket ID. OIDC is an alternative authentication mode and does not
automatically migrate existing accounts.

## Runtime settings

| Variable | Default | Purpose |
|---|---|---|
| `PICTOR_ADDR` | `127.0.0.1:8080` | Listen address |
| `PICTOR_DB` | `.local/codexartifacts.db` | SQLite database |
| `PICTOR_ASSETS_DIR` | `assets` beside the database | Uploaded files |
| `PICTOR_USERS_FILE` | `.local/users.json` | Private local account hashes |
| `PICTOR_AUTH` | `local` | `local`, explicitly configured `ldap`, or [OIDC](oidc.md) |
| `PICTOR_OAUTH_ORIGIN` | empty (off) | Fixed public origin enabling [MCP OAuth](mcp-oauth.md), e.g. `https://artifacts.example.org` |
| `PICTOR_PUBLIC_LINKS` | `false` | Allow authors to publish public links |
| `PICTOR_USAGE_READERS` | empty | Comma-separated accounts allowed to read usage |

The `PICTOR_` setting names and `window.pictor` artifact API are retained for
compatibility. Product branding is CodeXArtifacts.

LDAP is optional: set `PICTOR_AUTH=ldap` and supply `LDAP_URL`, `LDAP_DOMAIN`,
`LDAP_BASE_DN`, plus optional comma-separated `LDAP_ALLOWED_GROUPS`.
No company directory, internal host or organization credentials are built in.
Simple bind is used only over LDAPS; other LDAP connections use NTLM.

## Docker

The published image is `ghcr.io/iml885203/code-x-artifacts:v0.1.0`, for Linux
AMD64 and ARM64. Download [the release Compose file](https://github.com/iml885203/code-x-artifacts/releases/download/v0.1.0/compose.release.yaml)
as `compose.yaml` in a new directory. It pulls the image without a source checkout
or build tools, runs as uid/gid 65532, binds only `127.0.0.1:8080`, and keeps all
application data in one named `data` volume:

```sh
docker compose pull app
docker compose run --rm app init /data
docker compose run --rm password cat /data/admin-password
docker compose run --rm password rm /data/admin-password
docker compose up -d
```

Save the generated password privately before deleting its plaintext bootstrap
file; the bcrypt account file remains. The `password` helper has no listener and
uses the same non-root identity. The server image contains no runtime Node.js or
shell; `/licenses` carries the project and third-party notices. The image's
`codexartifacts healthcheck` command probes the configured local listener within
three seconds without opening SQLite or loading sign-in configuration. Container
health is liveness, not proof that login or persistence works.

The original `compose.yaml` in a source checkout remains a build-based option:
run `docker compose build` before the same initialization/start steps above.
Do not use a source-build Compose file as a replacement for a deployed release
file without preserving its project name and data volume.

### Updates

Use a concrete version, never an automatically moving `latest` tag. The matching
GitHub release's `image.json` records the source revision, multi-platform manifest
digest and platforms. To pin the exact published content, replace the app's image
with `ghcr.io/iml885203/code-x-artifacts@sha256:<recorded-digest>`; Docker selects
the matching platform. Keep the recorded previous image and compatible data backup.

Before updating, stop this app and back up its full volume as described below.
Edit only the image reference in the existing release Compose file, then pull and
recreate the app with that same Compose project and volume. Verify sign-in,
representative artifacts, uploads and editing. Never run `down -v` for an update.
An older image is not a database rollback; restore a matching backup into a
separate volume if schema compatibility requires it. In particular, do not
restore revoked browser sessions or roll back to stateless sessions without
following [OIDC session recovery](oidc.md#upgrade-and-rollback).

For a remote deployment, put a trusted HTTPS reverse proxy in front of the
loopback port, configure login rate limiting and request-size/time limits there,
and preserve WebSocket upgrades. Secure cookies remain enabled; plain HTTP is
only practical through browsers' `localhost` exception. Do not expose the local
password/bootstrap files through a web server or commit them.

## Persistence and recovery

Run one replica: live room coordination is in memory and its accepted updates
are logged to SQLite before broadcasting. Restart recovery uses those logs.
Stop the application, copy the full data directory/volume to a private backup,
then restart. Retain SQLite WAL/SHM files if present and all assets, account hashes
and session-key database state. Restore to a separate data directory first and
verify representative artifacts, permissions, files and continued editing before
switching users to it. Never mount the same SQLite volume into two active instances.

The `/healthz` endpoint is a liveness check; it does not prove login, publishing
or persistence. Deployment acceptance should verify those application behaviors.

## Bundled font fallback

The legacy Board fallback slot uses the OFL-licensed Arimo font in this distribution.
Its metrics and appearance are not claimed identical to the legacy Liberation binary.
Other bundled Board families retain their upstream files and copyright notices.
The build keeps all font requests on the same origin.
