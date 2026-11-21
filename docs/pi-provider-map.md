# PI provider map and Opera integration plan

Reviewed against the local `pi-mono` checkout on 2026-11-17. PI has a wider
catalog than six providers; this map selects six prominent API families that
cover both native protocols and the OpenAI-compatible route used by several
providers.

## Mapping

| PI provider | API / endpoint | Authentication in PI | Opera path |
| --- | --- | --- | --- |
| `openai` | OpenAI Responses API | `OPENAI_API_KEY` | Native Y `openai` adapter; already available |
| `anthropic` | Anthropic Messages API | API key, OAuth/subscription, bearer token and workload identity | Native Y `anthropic` adapter; API key path available |
| `google` | Gemini `generateContent` | `GEMINI_API_KEY` | Native Y `google` adapter; already available |
| `openrouter` | OpenAI Chat Completions plus additional PI-specific APIs | `OPENROUTER_API_KEY`, OAuth | Y OpenAI-compatible adapter; Chat Completions path only |
| `groq` | OpenAI Chat Completions | `GROQ_API_KEY` | Y OpenAI-compatible adapter |
| `deepseek` | OpenAI Chat Completions | `DEEPSEEK_API_KEY` | Y OpenAI-compatible adapter |

PI source references: `packages/ai/src/providers/{openai,anthropic,google,openrouter,groq,deepseek}.ts`.
PI's OpenRouter provider additionally supports Anthropic Messages, OAuth,
personalized model catalogs, image generation and classifier APIs. Those
capabilities are outside Y v1.4.0's OpenAI-compatible contract and are not
claimed by Opera.

## Authentication flows in PI

`pi auth login <provider>` consults the provider's `auth` descriptor. If OAuth
and API-key login are both available, OAuth is the default; `--api-key` selects
the secret-key prompt. Providers with only API-key login prompt for a key.
Credentials are stored by provider ID in `~/.pi/agent/auth.json` (or the
configured agent directory), separate from `settings.json` and `models.json`.
Stored keys and OAuth credentials resolve through the same provider-specific
auth layer; environment credentials remain available as a fallback.

| Provider | PI login choices | Credential source and behavior | PI source |
| --- | --- | --- | --- |
| `openai` | ChatGPT subscription OAuth by default; `--api-key` for an API key | `OPENAI_API_KEY` is the environment fallback. OAuth is PKCE browser login with callback/manual redirect entry, access-token refresh, and subscription-specific model filtering. | `packages/ai/src/providers/openai.ts`; `packages/ai/src/auth/oauth/openai-chatgpt.ts` |
| `anthropic` | Claude Pro/Max OAuth by default; `--api-key` for an API key | Key prompt and OAuth are stored separately by credential type. Resolver accepts stored key, `ANTHROPIC_AUTH_TOKEN` bearer auth, `ANTHROPIC_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, then workload identity federation. OAuth supports browser callback or copy-code entry and refresh. | `packages/ai/src/providers/anthropic.ts`; `packages/ai/src/auth/oauth/anthropic.ts` |
| `google` | API-key prompt | `GEMINI_API_KEY` is the environment fallback. No OAuth flow is registered for this provider. | `packages/ai/src/providers/google.ts` |
| `openrouter` | OAuth by default; `--api-key` for an API key | `OPENROUTER_API_KEY` is the environment fallback. OAuth uses PKCE plus callback/manual code entry and exchanges the code for a long-lived API key; no refresh-token exchange is needed. After login PI refreshes the personalized `/models/user` catalog. | `packages/ai/src/providers/openrouter.ts`; `packages/ai/src/auth/oauth/openrouter.ts` |
| `groq` | API-key prompt | `GROQ_API_KEY` is the environment fallback. No OAuth flow is registered. | `packages/ai/src/providers/groq.ts` |
| `deepseek` | API-key prompt | `DEEPSEEK_API_KEY` is the environment fallback. No OAuth flow is registered. | `packages/ai/src/providers/deepseek.ts` |

The common API-key login helper prompts for a secret, then the auth command
stores the returned `api_key` credential. See
`packages/ai/src/auth/helpers.ts`, `packages/coding-agent/src/cli/auth-login.ts`
and `packages/coding-agent/src/core/auth-storage.ts`. PI creates the auth
directory with mode `0700` when it is missing and writes `auth.json` with mode
`0600` on Unix.

## Opera implementation map

Opera now derives `/login` groups from login descriptors on the same provider
catalog used for execution. It lists the six providers above; custom providers
remain separately configured. API-key entry is masked in the TUI and saved to
the credential store. Accessible mode accepts existing environment/store keys
and directs users to pipe a key to `opera auth set <provider>.<credential_ref>`
when no credential is available.

The target `/login` choices corresponding to this six-provider set are:

| `/login` provider | Initial method | Additional method | Selected execution provider |
| --- | --- | --- | --- |
| OpenAI | ChatGPT OAuth | OpenAI API key | OAuth → `openai-subscription`; key → `openai` |
| Anthropic | Claude Pro/Max OAuth | Anthropic API key | `anthropic` |
| Google Gemini | API key | — | `google` |
| OpenRouter | OpenRouter OAuth | OpenRouter API key | `openrouter` |
| Groq | API key | — | `groq` |
| DeepSeek | API key | — | `deepseek` |

The implementation preserves three boundaries:

1. Keep login discovery separate from model discovery, but derive both from
   explicit provider/auth descriptors rather than parallel hard-coded lists.
2. Add a secret prompt to the auth interaction and persist API keys using the
   Opera credential store. Do not put built-in credentials in `settings.json`,
   provider/model metadata, transcripts, or UI events.
3. Implement provider-specific OAuth where PI uses non-generic flows. The
   existing Opera OAuth command accepts caller-supplied endpoints and does not
   implement ChatGPT, Claude, or OpenRouter's PKCE/token exchanges. Preserve
   refresh semantics for OpenAI and Anthropic; OpenRouter OAuth produces a
   durable API key. Cancellation and callback/manual-code input must remain
   owned by the auth operation.

This map scopes six providers, not six OAuth flows: Google, Groq and DeepSeek
use API-key login only; OpenAI, Anthropic and OpenRouter also offer OAuth.
Opera implements ChatGPT and Claude account OAuth plus OpenRouter PKCE key
exchange. Anthropic OAuth refreshes its token pair; OpenRouter OAuth stores the
long-lived API key returned by its exchange. OpenAI Codex OAuth, Google Vertex
credentials, and other PI providers remain outside this increment.

## Compatibility boundary

Opera uses the published Y SDK v1.4.0. It provides native OpenAI Responses,
Anthropic Messages, Gemini and generic OpenAI-compatible Chat Completions
adapters. Consequently, the six providers above can be composed without adding
another execution loop. The compatible route does not imply provider-specific
reasoning, vision, structured output, pricing, OAuth or model capability
metadata; only conservative text, tools and streaming capability is exposed.

## Implementation plan

1. Preserve the existing native OpenAI, Anthropic and Google adapters.
2. Add explicit, instance-scoped catalog entries for OpenRouter, Groq and
   DeepSeek, using the published Y compatible adapter with provider-specific
   default endpoints and provider IDs for credential resolution.
3. Extend credential lookup to each provider's documented environment key;
   retain the existing credential store and avoid putting secrets in core config.
4. Discover compatible model IDs from each provider's `/models` endpoint only
   when the user explicitly requests `opera models`; use the selected endpoint
   and context cancellation. Honor user-controlled `base_url` overrides.
5. Expose provider and auth choices in `/login` and onboarding and document
   the protocol limitations. The `/login` work is implemented from catalog
   descriptors, with masked API-key input, secret-store persistence, OAuth
   callback handling and cancellation.
6. Keep OpenAI Codex OAuth, GitHub Copilot OAuth/multi-protocol, Vertex ADC and
   Bedrock AWS signing out of this increment: they require provider-specific
   auth/protocol implementations not published by Y v1.4.0.

The acceptance boundary for this increment is one default catalog used for
execution and discovery, distinct provider IDs and credentials, cancellable
model discovery, and no changes to app/UI execution ownership or durable
formats. Native support for the remaining PI-specific auth/protocol routes
requires public Y adapters or a separately scoped Opera adapter with equivalent
contract coverage.
