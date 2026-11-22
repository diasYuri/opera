# Usage, cost provenance and structured results

## Accounting

```sh
opera usage
opera usage run <run-id>
opera usage session <session-id>
opera usage agent <run-id>
opera usage turn <run-id> <turn-id>
```

`/usage` selects the current/previous run and its descendants. `usage` defaults
to all requests in the current workspace. Each actual provider stream invocation
gets one persisted request ID before inference. HTTP retries inside that invocation
share its correlation/idempotency identity. Replayed UI/SDK events update no billing
rows. Provider turn labels can recur after resume; the ledger uses its own stable
per-invocation turn IDs, listed in `records`. Turns are also grouped by run, session,
provider and model through public SDK telemetry contracts.

A run aggregate includes descendants once. Agent scope includes only that agent.
Session scope includes its runs and descendants without adding pre-aggregated
child totals. Branches inherit transcript context, never usage claims; new inference
on a branch counts separately. Retry/resume count only new provider invocations.
Each conversation keeps a bounded `context.md` shared with its subagents.
Backstage consolidates it automatically at 60% of the operational context window.
Use `/compact` in chat or `opera compact <run-id>` for the latest completed
checkpoint of the main conversation. Use `sessions branch` for earlier checkpoints.

Compaction inference is recorded with kind `compaction`; rewriting a transcript
never erases the ledger. Work admission's `limits.worker_tokens` reservation is a
separate conservative budget, unlimited by default, not a billing figure.

Token status is `known` for normalized provider usage, `estimated` for a completed
or interrupted stream without reported usage, and `unavailable` for a request
whose process ended before an observation could be finalized. Context/output
estimates are kept separately from reported tokens. Repeated cumulative samples
within one invocation update one observation. A started request can remain
incomplete after a crash; the CLI does not invent its final bill. Provider SDK
transport retries and billing adjustments that are not exposed by the SDK cannot
be independently measured.

Cost status is `known` only when a positive cost is supplied in normalized provider
usage, `estimated` when a matching explicit price table has enough rates and reported
token counts, and `unavailable` otherwise. Zero-valued omitted SDK costs do not
prove a free request. Aggregates report known and estimated subtotals separately,
plus counts of requests with unavailable cost. The provider normalization is not
an invoice. Every priced row records the source/date used.

An optional user-only `accounting.prices_file` selects a JSON array such as
[the illustrative table](../examples/prices.json). Rates are USD per million tokens.
Its fixture numbers are examples, not current provider prices. Supply your own
source and date; Opera does not fetch or silently refresh prices. Missing rates for
nonzero cache/input/output quantities leave cost unavailable. Reasoning tokens
are reported separately and are already included in normalized output where the
adapter defines that; they are not charged twice. Project TOML cannot choose a
price file. No provider price or zero-cost assumption is built in.

## Structured model content

```sh
opera run 'Return a JSON object describing the change' --response-format json_object
opera run 'Return the result in the supplied schema' --response-schema examples/result.schema.json --format jsonl
```

`--format jsonl` selects the **event transport**. `--response-format` and
`--response-schema` select the **model's generated content**. They are independent.
Validated content appears in `execution.result.data.structured_output` and the
normal assistant transcript. The schema is bounded to 1 MiB and validated by the
public SDK before inference; returned JSON is validated too. Unsupported schema
keywords/contracts, invalid JSON, mismatched types and unsupported providers fail.
A validation failure does not silently fall back to text or repeat tool effects.

Support follows the selected SDK provider/model's advertised capabilities.
OpenAI-compatible endpoints with unknown capabilities are refused rather than
assumed equivalent to native OpenAI. Account/model access and additional endpoint
restrictions can still produce provider errors. `models` reports SDK metadata;
it is not an authentication or availability probe. Resume restores the stored
response contract; compaction uses its own summarization request.
