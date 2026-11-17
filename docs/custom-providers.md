# Custom providers

Opera reads custom provider definitions from `~/.agents/opera/providers.json`.
The file is optional; built-in providers remain available. Its top-level object
contains a `providers` array. Provider names are stable lowercase identifiers
and are selected as `custom:<name>`. Keep `name` stable because Opera records
the provider ID in run metadata and checkpoints.

```json
{
  "providers": [
    {
      "name": "acme",
      "baseUrl": "https://api.acme.example/v1",
      "provider_type": "openai",
      "apikeyhelper": "/usr/local/bin/acme-api-key",
      "models": [
        {
          "id": "acme-reasoner",
          "name": "Acme Reasoner",
          "thinkingLevel": ["low", "medium", "high"],
          "input": ["text", "image"],
          "cost": {"input": 2.0, "output": 8.0, "cacheRead": 0.2, "cacheWrite": 2.5},
          "contextWindow": 128000,
          "maxTokens": 8192
        }
      ]
    }
  ]
}
```

`provider_type` is `openai` or `anthropic`. `openai` uses Y's OpenAI-compatible
Chat Completions adapter; `anthropic` uses Anthropic Messages. `thinkingLevel`
lists the levels accepted by that model. `input` contains one or both of `text`
and `image`. Cost values are USD per million tokens. Provider names and model
IDs within each provider must be unique. `baseUrl` must use HTTPS, except for
loopback HTTP endpoints. `contextWindow` and `maxTokens` must be positive, and
`maxTokens` cannot exceed `contextWindow`.

Configure exactly one of `apikey` or `apikeyhelper`. `apikey` is a literal key
stored in this private file. `apikeyhelper` is an absolute executable path, without
shell syntax or arguments. Opera invokes it only when a provider request needs a
credential, with a 10-second deadline. Stderr is discarded, stdout is limited to
64 KiB, and surrounding whitespace is removed. The helper must write only the
key to stdout. Helpers run as the current user and are trusted code.

Protect the directory and file before storing keys:

```sh
mkdir -p ~/.agents/opera
chmod 700 ~/.agents/opera
chmod 600 ~/.agents/opera/providers.json
# If settings.json already exists:
chmod 600 ~/.agents/opera/settings.json
```

On Windows, use an ACL granting access only to the current user. `config show`
and `doctor` do not display API keys or run helpers. `opera models --provider
custom:acme` lists the configured model metadata without contacting the endpoint.
`opera run "..." --provider custom:acme --model acme-reasoner` selects the
custom provider directly.
