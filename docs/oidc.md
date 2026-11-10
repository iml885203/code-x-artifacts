# Sign in with an OIDC provider

CodeXArtifacts can delegate browser sign-in to an operator-configured OpenID
Connect provider, such as Pocket ID. Users authenticate there and return to a
normal application session. Passwords/passkeys stay with the provider.

Choose one authentication mode: `local` (default), `ldap` or `oidc`. There is no
combined login chooser. Switching modes does not delete accounts or artifacts,
and does not automatically merge identities.

## Configure

Register an OIDC application at your provider with an exact callback:

```text
https://artifacts.example.org/auth/oidc/callback
```

Use Authorization Code with PKCE S256 and restrict who may use this application.
Set `PICTOR_AUTH=oidc` and configure:

| Variable | Purpose |
|---|---|
| `OIDC_ISSUER` | Exact trusted issuer for discovery |
| `OIDC_CLIENT_ID` | Registered client ID |
| `OIDC_CLIENT_SECRET_FILE` | Private client-secret file for a confidential client |
| `OIDC_ORIGIN` | Canonical application origin, without a path |
| `OIDC_PROVIDER_NAME` | Login label; default `OpenID Connect` |
| `OIDC_SCOPES` | Comma-separated scopes, default `profile,email`; `openid` is always included, no offline access requested |
| `OIDC_ALLOWED_SUBJECTS` | Comma-separated admitted subject IDs |
| `OIDC_ALLOWED_GROUPS` | Comma-separated admitted provider groups |
| `OIDC_TRUST_APP_ASSIGNMENT` | Explicitly trust IdP client assignment when no local admission lists apply |
| `OIDC_NAME_CLAIM` | Display-name claim, default `name` |
| `OIDC_GROUPS_CLAIM` | Provider group claim, default `groups`; must be a list of strings |
| `OIDC_GROUP_MAP` | JSON map of provider groups to stored profile groups; empty by default |

Admission must be explicit. Trust application assignment only if your provider
restricts who may use this client. Admission and profile group mapping are
separate: permission to log in does not grant an existing account's ownership.
Missing groups cannot bypass a configured group restriction. If both subject and
group lists are configured, the subject must match and at least one group must
match. Application-assignment trust never bypasses these lists.

For example, `OIDC_GROUP_MAP='{"designers":"Design"}'` maps the provider
group `designers` to stored profile group `Design`; it does not admit users by
itself. Stored groups are profile metadata in the current permission model: they
grant no artifact or administrator access. Artifact access comes from ownership,
sharing, named editors and organization editing rules. Ask the provider for its required group scope when configuring group claims.

Use HTTPS for deployed application/provider URLs and retain TLS verification.
The browser and backend must both reach the provider. Set the reverse proxy's
host/scheme to match the canonical origin. Loopback development exceptions do not
permit insecure remote providers. Login requests cannot supply an arbitrary issuer.
Discovery and signing-key requests are bounded and use operator-configured trust.

For example, after saving the secret in a private readable file:

```sh
PICTOR_AUTH=oidc \
OIDC_ISSUER=https://identity.example.org \
OIDC_CLIENT_ID=codexartifacts \
OIDC_CLIENT_SECRET_FILE=.local/oidc-client-secret \
OIDC_ORIGIN=https://artifacts.example.org \
OIDC_TRUST_APP_ASSIGNMENT=true \
./bin/codexartifacts
```

For Docker, pass the same variables through a private Compose override and mount
its secret read-only at `OIDC_CLIENT_SECRET_FILE`. The image runs as uid/gid 65532;
that identity must be able to read it. Do not bake credentials into the image.
Keep the normal SQLite/assets volume from [deployment](deployment.md#docker).

## Identity and permissions

The issuer/subject pair (`iss` + `sub`) maps to a persistent internal account.
Email/name changes preserve ownership. Another subject with the same email/name
cannot take over the account. Provider groups map only through explicit settings and remain profile metadata;
they do not grant artifact or administrator access.

No automatic migration or account-linking UI is included. Existing local/LDAP
artifacts stay with their owners; matching an old email/username grants nothing.
Back up the data and plan sharing/access before switching authentication modes.

OIDC users retain normal artifact permissions and the opaque-origin sandbox.
Provider ID/access tokens cannot be used directly as API or MCP bearer tokens.

## Agent authorization

[MCP OAuth](mcp-oauth.md) remains separate: the agent opens authorization, the
person signs in through OIDC, then returns to the application consent page.
Sign-in alone does not approve the agent. Independent grant revocation and PAT
connections continue to work.

When enabling MCP OAuth, use the same application origin for `PICTOR_OAUTH_ORIGIN`
and `OIDC_ORIGIN`. Do not copy provider tokens into agent configuration.

## Logout, outages and recovery

Browser sessions use random opaque cookies and durable keyed indexes in SQLite,
with the existing seven-day lifetime. Application logout revokes the current
session (including retained cookie copies), clears session/pending-login cookies,
and returns to explicit login. It does not sign the user out at the provider or
revoke other devices, PATs or MCP grants.

### Subject-wide back-channel logout

Register the exact HTTPS receiver `<application origin>/auth/backchannel-logout`
on the configured OIDC client. It accepts POST form `logout_token` notifications
signed by that provider, with `typ=logout+jwt`, the correct issuer/audience,
`sub`, `jti`, the back-channel logout event, valid `iat`/`exp`, and no `nonce` or
`sid`. This initial receiver supports subject-wide notifications (including
Pocket ID); sid-only and sub+sid notifications are rejected rather than broadened
to every session. Tokens may live at most five minutes, with at most one minute
of future issuance allowance. Providers with different token profiles need
separate interoperability validation.

After a valid notification commits, all browser sessions for the exact
issuer/subject are invalid. Unknown subjects create no account but retain a
cutoff to block callbacks already in flight. Callbacks issued at/before the
cutoff or started before notification receipt cannot recreate a session; a
same-second sign-in may need retry. Identical `jti` replay succeeds without
repeating revocation, including across restart. A freshly signed provider retry
is a new subject-wide event and can revoke a later login.

Private HTTP requests and new browser socket handshakes reject revoked sessions,
including tickets issued before revocation. Already-open realtime connections
can remain until their existing maximum one-hour lifetime. Previously issued
artifact/file grants keep their existing up-to-six-hour lifetime and artifact
permission checks; delivered content cannot be recalled. Browser-session
revocation does not revoke these capabilities or independent PAT/MCP grants.

Each authentication table has a 10,000-record cap: active browser sessions,
subject cutoffs, and unexpired event replay markers. Sessions expire after seven
days; cutoffs remain eight days (covering the session and ten-minute callback
windows), and replay markers remain until token expiry. Expired records are
cleaned on session creation/notification. Capacity exhaustion refuses new
sessions or returns retryable 503 for notifications, without discarding active
protection; investigate load and retry after expiry.

Ordinary provider logout is not global application logout. Administrative
account/group/client-access changes revoke application sessions only when the
provider sends and successfully delivers a supported notification. Outages,
exhausted provider retries, and unsupported events may leave sessions valid
until expiry; restore delivery and revoke sessions through a supported provider
administrative action. Do not assume missed notifications are replayed forever.
Invalid tokens return 400; transient key-fetch/storage failures return 503 and
success is sent only after durable revocation.

### Upgrade and rollback

This session-format upgrade invalidates old stateless browser cookies once; the
login page asks those users to sign in again. Accounts, artifacts, ownership,
PATs, MCP grants and the stored signing key are preserved. Back up the stopped
application's full data directory before upgrading. Never restore old session
state to recover revoked sessions. Rolling back to a stateless-session binary
can resurrect old signed cookies: invalidate/rotate its signing key as part of
rollback (also invalidating existing signed artifact/file grants), or keep the
revocable-session version. This is an explicit recovery operation, not an
ordinary image rollback. Test recovery against a separate data copy first.

Provider outages block new sign-ins; invalid callbacks create no session. Existing
sessions keep their normal lifetime. Invalid configuration fails closed rather
than silently falling back to password login. Fix the provider/settings and retry.

For recovery, stop the app, restore the prior local/LDAP mode and its private
account file or directory settings, then restart against the same data directory.
Keep those settings/backups before switching. This restores a login mode without
merging identities or transferring artifacts. If recovering from a compromise,
plan separate session and credential invalidation; switching login modes alone does not revoke existing sessions.

## Verified providers

Verified on 2026-11-10 using isolated synthetic accounts and loopback HTTP
application/provider origins:

| Provider | Authentication | Observed behavior |
|---|---|---|
| Pocket ID 2.19.2 | Fresh Chrome session with a CDP virtual passkey and provider consent | Login return, stable account, logout, OIDC-to-MCP consent, MCP identity/publish and grant revocation passed |
| Dex 2.46.1 (official pinned source) | Local static-password connector | The same login/session/MCP scenarios passed |

Both used confidential clients with private secret files and PKCE S256, the exact
`/auth/oidc/callback` address, `openid profile email` scopes, explicit application-
assignment trust and no group mapping. Group restrictions/mapping are covered by
separate Go integration tests; they were not exercised against these live
providers. Pocket ID's passkey was a browser test authenticator, not a physical
security key. Remote HTTPS deployments and other versions/connectors have not
been exercised and are not claimed verified.

Back-channel logout was separately verified with official Pocket ID 2.19.1 on
isolated loopback HTTP origins using Chrome and a CDP virtual passkey: actual
OIDC login, local logout, rejection of a retained cookie, re-login and an
administrative account-disable notification followed by private API rejection
passed. This is synthetic-account interoperability evidence, not proof of a
physical passkey or remote HTTPS rollout.
