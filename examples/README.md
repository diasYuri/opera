# Executable examples

`offline.sh` / `offline.ps1` require only an Opera binary on PATH. `OPERA_BIN`
can point to a local binary. Each demo runs the real shared application service
with a deterministic provider in a temporary workspace and prints acceptance
evidence. It is safe to run without an account or an SDK checkout.

For real inference first configure provider/model/policy through onboarding,
user TOML or OPERA_* variables. The following use the current workspace:

```sh
opera demo headless --format jsonl

opera chat
opera run 'Inspect this workspace' --format jsonl
opera worker 'Plan, implement and verify the requested change'
opera run 'Return the verified result as JSON' --response-schema examples/result.schema.json --format jsonl
```

For a reproducible headless approval without paid inference, `tools call` uses
the same durable harness and operation store:

```sh
opera tools call write_file '{"path":"opera-example.txt","content":"approved example"}' --policy ask --format jsonl
# Exit 3 is expected. Read the execution.result run_id and approval.id.
opera resume <run-id> --approve <approval-id> --format jsonl
# Or explicitly deny it:
opera resume <run-id> --deny <approval-id> --format jsonl
```

Do this in a throwaway workspace. `demo` automates these approval boundaries and
also proves a denial has no effect. JSONL stdout contains complete event objects;
error envelopes use stderr. Event JSONL is independent of a model JSON schema.
`prices.json` contains illustrative numbers only, not provider pricing.
