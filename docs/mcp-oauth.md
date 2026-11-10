# Connect agents with MCP OAuth

OAuth lets a compatible MCP client open your browser, ask you to sign in to
CodeXArtifacts and request access. You do not copy a personal token into the
client. Every authorization belongs to one account and can be revoked separately
on **Connect your agent**. Existing personal access tokens (PATs) still work.

## Enable it

Set the application's canonical public **origin**, without a path or trailing slash:

```sh
PICTOR_OAUTH_ORIGIN=https://artifacts.example.org ./bin/codexartifacts
```

For the local quick start, use `http://localhost:8080`. Orbit's development app
sets the localhost origin to its assigned port automatically. With Docker Compose,
set `PICTOR_OAUTH_ORIGIN` in your private `.env` before `docker compose up -d`.

Unset means OAuth is disabled. Invalid values fail startup. Remote origins require
HTTPS; HTTP is accepted only for loopback development. Keep the proxy and this
setting aligned, forward the original `Host`, and redirect visitors to the
canonical origin. Arbitrary forwarded-host headers do not select the OAuth issuer.
Do not rewrite `/oauth/*`, `/api/mcp` or `/.well-known/*` to another application.
Protect the public endpoints with proxy request-rate and body-size limits.

## Connect

Use `https://artifacts.example.org/api/mcp` as the Streamable HTTP MCP URL. The
client discovers authorization endpoints and opens browser login/consent. Check
the requesting client's identity and callback before approving. Client names are
self-declared, not proof of a trusted publisher. Only approve clients you intended
to connect.

The app supplies client commands and a copyable URL. For example:

```sh
claude mcp add --transport http codexartifacts https://artifacts.example.org/api/mcp
claude mcp login codexartifacts

codex mcp add codexartifacts --url https://artifacts.example.org/api/mcp
codex mcp login codexartifacts
```

Clients without a compatible browser authorization flow can use the manual PAT
instructions on the same page. Do not add a Bearer header when using OAuth.

## Permissions and revocation

Authorization permits MCP operations as your account, subject to the same artifact
sharing/editing rules as PATs. It does not make another person's private artifacts
visible and does not change the opaque-origin content sandbox. It is not Google
or GitHub SSO: existing local/LDAP account login still applies.

Revoke a client under **Connect your agent** to stop its access and renewal. Other
clients and unrelated PATs remain valid. Reconnecting requires new browser consent.
Removing an account from the login roster does not itself invalidate previously
issued sessions, PATs or grants; revoke them before removing its access.

## Protocol and persistence

The server exposes RFC 9728 protected-resource metadata and RFC 8414 authorization
server metadata. Public clients use Authorization Code with PKCE S256 and exact
registered redirect URIs. Dynamic Client Registration is provided for client
compatibility; URL-based Client ID Metadata Documents are not fetched. No outbound
metadata fetch means client registration cannot request internal network resources.

Tokens are bound to this server's `/api/mcp` resource. Browser cookies alone cannot
call MCP, and OAuth access tokens do not authorize unrelated REST endpoints.
Authorization codes expire after 5 minutes, access tokens after 1 hour, and refresh
tokens after 30 days from the original issuance. Refresh tokens rotate on use;
reuse revokes that authorization. Replaying an exchanged authorization code also
revokes the credentials issued from it. Reauthorize after the refresh lifetime expires.
Consent forms expire after 10 minutes. Inactive client registrations expire after
90 days; a successful authorization renews the registration lifetime.

Client metadata and grants persist in SQLite. Credential lookup keys are hashed;
plaintext access/refresh tokens are not stored in the database. Keep the database,
account files and backups private. Changing the configured origin requires clients
to reconnect; grants for the previous resource cannot authorize the new resource.

See the [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
for protocol details. Supported client versions and observed results are recorded
below; other clients are not assumed compatible merely because they support MCP.

## Verified compatibility

Tested on 2026-11-10 against the Go server on a loopback HTTP development origin:

| Client | Registration and callback | Observed result |
|---|---|---|
| Codex CLI 0.167.0 | Automatic DCR; loopback HTTP callback; `mcp login --no-browser` | Discovery, browser sign-in/consent, code exchange and client credential storage succeeded |
| Claude Code 2.1.321 | Automatic DCR; loopback HTTP callback; `mcp login --no-browser` in an interactive terminal | Discovery, browser consent, code exchange and client credential storage succeeded |

Both runs used isolated client settings, not the operator's normal agent setup.
Headless login may require pasting the callback URL into the client; Claude Code
requires an interactive terminal for this. The app also offers the in-session
`/mcp` authentication path.

The browser suite separately exercises two independent client grants, login return,
MCP initialization and owner revocation while preserving the other client. Go MCP
integration tests cover artifact operations and private-artifact permission refusal.
Remote HTTPS callbacks and other client versions have not been exercised; the
server accepts exact registered HTTPS callbacks, but that is not a client
interoperability claim. URL-based Client ID Metadata Documents and confidential
client authentication are not advertised. Use DCR-compatible public clients or PATs.
