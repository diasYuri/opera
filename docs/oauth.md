# OAuth configuration and credentials

For ChatGPT plan usage, use `opera auth login openai-subscription` with no
endpoint environment configuration. See [ChatGPT subscription](chatgpt-subscription.md).
The generic configuration below applies to separately registered OAuth clients.

`opera auth login <provider>` uses the public Y OAuth client. Opera supplies UI
notifications and secret storage; the SDK never reads stdin, prints a URL or
starts a browser. Login works through PKCE/loopback or device authorization.

Set these environment values for a client registered or permitted by your provider:

| Variable | Purpose |
| --- | --- |
| `OPERA_OAUTH_CLIENT_ID` | Required registered client ID; Opera does not invent an embedded registration |
| `OPERA_OAUTH_CLIENT_SECRET` | Optional secret, read only from the environment and stored with the OAuth record |
| `OPERA_OAUTH_TOKEN_URL` | Required token endpoint |
| `OPERA_OAUTH_AUTH_URL` | Authorization URL for PKCE/loopback |
| `OPERA_OAUTH_DEVICE_URL` | Device endpoint; selects device authorization instead of PKCE |
| `OPERA_OAUTH_SCOPES` | Space-separated scopes |
| `OPERA_OAUTH_LISTEN_ADDRESS` | Numeric loopback listen address for PKCE, default `127.0.0.1:0` |

```sh
opera auth login openai
opera auth login google --store file --format jsonl
```

The CLI displays the authorization URL and device user code where applicable,
without starting a browser. The user completes authorization externally while
the process waits with cancellation and a bounded timeout. Headless mode emits
`auth.authorization` and `auth.status` JSONL records, without forms. HTTPS is
required for endpoints, with HTTP allowed only on loopback. PKCE validates state;
token-bearing HTTP redirects cannot leave the original endpoint origin.

API-key environment variables are resolved first, so headless CI does not unlock a
desktop keyring: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`/`ANTHROPIC_OAUTH_TOKEN`,
`GEMINI_API_KEY`/`GOOGLE_API_KEY`, or `OPENAI_COMPATIBLE_API_KEY`. Otherwise Opera
tries the configured `credential_ref` API key, then `<provider>.<reference>.oauth`
in the selected secret store. Expired OAuth records are refreshed and persisted;
an omitted refresh-token rotation preserves the existing token.

Token endpoint/client configuration is provider-specific. OAuth registration and
provider account entitlements are external requirements; storing a token does not
prove access to every model or API family. `doctor` does not test authentication.
Default tests use injected HTTP fixtures, never real OAuth accounts or credentials.

The file fallback is explicitly selected and unencrypted, with private ownership/
permissions or Windows ACLs. No automatic fallback occurs. Interactive `/login` offers an explicit storage
choice when the keyring is unavailable and remembers the chosen store for the
workspace. Access/refresh tokens
and optional client secrets are encoded only inside that secret store; they are
excluded from configuration, events, checkpoints and exported execution metadata.
