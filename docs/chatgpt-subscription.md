# ChatGPT subscription

Opera supports the official [Sign in with ChatGPT flow for open-source/local
apps](https://developers.openai.com/siwc/token-sharing-open-source). The provider
is `openai-subscription`. `openai` continues to use API keys or separately
configured generic OAuth credentials.

## Sign in inside Opera

Start `opera` and enter `/login`. The provider submenu currently offers **OpenAI**.
Use arrows and Enter to select it, or `/` to filter. Opera opens the system browser
and prints the authorization link as a fallback. The chat shows connection progress;
Esc or Ctrl+C cancels sign-in without exiting the session. Once authorized, Opera
fetches the account catalog and opens the model selector. Choosing a model saves
the workspace selection and returns to chat. No prior onboarding or model is
required to open this flow.

The submenu reads a provider registry; adding a supported login method does not
require changing the terminal menu. Only OpenAI is registered at present.

With `--accessible`, `/login` prints a numbered provider menu. Choose a number or
`openai`; after authorization, choose a model number. `/cancel` goes back or cancels
the active login. This path stays append-only and opens the same browser flow.

## Sign in, list models, run

```sh
opera auth login openai-subscription
opera auth status openai-subscription
opera models --provider openai-subscription
opera run "Inspect this workspace" --provider openai-subscription --model <slug>
opera chat --provider openai-subscription --model <slug>
opera auth logout openai-subscription
```

The login command prints a short-lived authorization URL. Open it in a browser
on the same computer, select your ChatGPT account/workspace, and authorize plan
usage. The callback listens only on `127.0.0.1` at an available port and
`/auth/callback`. Ctrl+C cancels login; attempts expire after five minutes.
Opera does not launch a browser automatically. JSONL mode emits
`auth.authorization` and `auth.status` without interactive forms.

First sign-in dynamically registers Opera without a client secret or a
preconfigured client ID. Opera persists one opaque host ID and reuses the issued
client ID on later sign-ins. It validates PKCE state and the ID token's signature,
issuer, audience, expiry, nonce and account identity before saving credentials.
The token grant must include permission to use the ChatGPT plan.

The model command fetches the selected account's live catalog, keeps entries
with `visibility: list`, and preserves the server's order. Use a returned model
ID (the API's `slug`) for execution. Catalog errors are reported; Opera does not
substitute an API-key catalog or a bundled list for this provider. The interactive
model picker uses the same authenticated catalog.

## Accounts and storage

Credentials use the OS keyring by default. Large OAuth records are stored as
versioned keyring parts, within the native entry limits on macOS and Windows.
The new manifest is published only after all parts are saved; failed writes
preserve the previous credential, and existing single-entry keys remain readable.

`--store file` explicitly selects an
unencrypted file store protected by owner-only permissions/ACLs. Pass the same
store option on login, models and execution commands. No automatic fallback
occurs. If `/login` cannot read or write the system keyring, the chat offers
**Retry system keyring**, **Use private local file**, and **Cancel login**. File
storage is unencrypted and requires an explicit choice. Opera remembers that
choice in the user-owned workspace settings and applies it to model listing,
execution and later launches (explicit flags/environment still take precedence).
If authorization has already completed, the same token write is retried in the
chosen store; it does not repeat browser authorization. The host ID and any
previously read account registration are preserved when switching stores.

Tokens never enter TOML, checkpoints, JSONL status, or model metadata.

Use `credential_ref` as the stable account label. Each reference stores a separate
verified account/client registration. For example:

```sh
opera auth login openai-subscription --set credential_ref=work
opera models --provider openai-subscription --set credential_ref=work
opera run "Inspect this workspace" --provider openai-subscription --model <slug> --set credential_ref=work
```

Choose a different reference to add another account/workspace. Login with an
existing reference reauthorizes its registered account; it rejects a mismatched
identity. Status shows the selected reference, account email, registration and
connection state. The `--provider` and `credential_ref` configuration also apply
to the interactive chat session; `/settings` identifies the selected provider.

API-key environment variables and generic stored API keys are ignored for
`openai-subscription`. It requires its own OAuth record. `--base-url` is rejected
for this provider, and credentials can reach only the supported public OpenAI
model-list and Responses routes.

Expiring tokens are renewed before requests, with rotating tokens saved together
atomically. A process lock serializes renewal across workspaces; contention has
a bounded wait. Transient errors preserve credentials. Terminal refresh errors
ask for a new login. Logout attempts remote revocation and clears local tokens,
retaining the account/client mapping and host identity for later login. If remote
revocation cannot be confirmed, Opera reports that explicitly.

## Inference and limits

Opera follows the [subscription Responses contract](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference):
public `/v1/responses`, `store: false`, `stream: true`, full input history, developer
instructions and namespaced local function tools. It omits the fields currently
unsupported by this flow, including `max_output_tokens`, temperature and persistent
server-side conversation state. Local time/context/tool budgets still apply;
the output-token setting does not impose a server-side cap in this flow.

Assistant history is replayed as `output_text`; user input uses `input_text`.
HTTP rejections retain their status, error code, affected parameter and request ID
for diagnostics, with arbitrary API messages removed to avoid echoing secrets.

A stream is successful only after its completion event. Interrupted streams,
incomplete generation, inference failures and subscription limits are errors.
Account/workspace eligibility and available models are controlled by OpenAI;
authentication does not guarantee every model or capability. Review plan usage
and app limits at [ChatGPT Settings → Usage](https://chatgpt.com/settings/usage).
See OpenAI's [preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations).

Automated tests use local OAuth/JWKS fixtures and intercepted Responses requests.
They do not sign into a real account or consume subscription usage.

After a failed chat request, submit a message or use `/retry` to resume the saved
execution. Repeating the unanswered prompt does not duplicate it in history.
Failed requests do not redisplay an earlier answer. Approval and uncertain-effect
checks still apply before any retry can execute tools.

The optional live regression checks use the selected subscription account with
short text prompts; the harness check uses an empty temporary workspace:

```sh
OPERA_SUBSCRIPTION_LIVE_TEST=1 OPERA_SUBSCRIPTION_WORKSPACE="$PWD" go test ./internal/ui/cli -run 'TestLiveSubscription(ConversationHistory|HarnessConversation)' -v
```

These checks consume a small amount of subscription usage. Default tests stay
offline. The harness check sends its temporary workspace path and tool schemas,
never the original workspace files or credential values.
