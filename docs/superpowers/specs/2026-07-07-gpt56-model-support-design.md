# GPT-5.6 Model Support Migration Design

## Context

This branch tracks FluxCode-specific changes and is currently behind upstream `Wei-Shaw/sub2api` for the GPT-5.6 model support work. After fetching `upstream/main`, the relevant upstream change is commit `6cea1c35b` (`feat: 适配 OpenAI 新模型 gpt-5.6-sol/terra/luna`).

The upstream commit touches eight files:

- `backend/internal/pkg/openai/constants.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_model_alias.go`
- `backend/internal/service/pricing_service.go`
- `backend/resources/model-pricing/model_prices_and_context_window.json`
- `frontend/src/components/keys/UseKeyModal.vue`
- `frontend/src/composables/useModelWhitelist.ts`

This branch does not have `backend/internal/service/openai_model_alias.go`; model normalization still lives in `normalizeCodexModel` inside `backend/internal/service/openai_codex_transform.go`. Therefore the migration should be semantic and hand-applied rather than cherry-picking the upstream commit.

## Goal

Add support for three OpenAI GPT-5.6 models:

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

Support means:

- Models appear in the default OpenAI model list.
- Codex/OpenAI model normalization preserves each GPT-5.6 variant instead of falling back to `gpt-5.1`.
- OpenAI OAuth/Codex forwarding can route the exact model IDs.
- Dynamic pricing data contains the three model entries.
- Static pricing fallback remains available and uses GPT-5.4 pricing/long-context policy when dynamic pricing is absent.
- Frontend model selection, preset mapping shortcuts, and OpenCode config generation expose the three models.

## Approach

Use semantic hand migration. Do not cherry-pick `6cea1c35b` directly, and do not introduce upstream's newer `openai_model_alias.go` structure into this branch.

This keeps the patch small and aligned with the current branch's older normalization architecture.

## Backend Design

### Default OpenAI Models

In `backend/internal/pkg/openai/constants.go`, add the three GPT-5.6 model entries near the top of `DefaultModels`, before `gpt-5.5`.

Use upstream metadata:

- `Created`: `1780876800`
- `OwnedBy`: `openai`
- `Type`: `model`
- Display names: `GPT-5.6 Sol`, `GPT-5.6 Terra`, `GPT-5.6 Luna`

Do not change `DefaultTestModel`; this branch currently uses `gpt-5.1-codex` for tests and account probes.

### Codex Model Mapping

In `backend/internal/service/openai_codex_transform.go`, add exact entries to `codexModelMap`:

- `gpt-5.6-sol` -> `gpt-5.6-sol`
- `gpt-5.6-terra` -> `gpt-5.6-terra`
- `gpt-5.6-luna` -> `gpt-5.6-luna`

Update `normalizeCodexModel` so it recognizes the three models before the broader `gpt-5.5`, `gpt-5.4`, and `gpt-5` checks. It should support the current branch's alias style:

- Hyphenated names such as `gpt-5.6-sol-high`
- Space-separated names such as `gpt 5.6 sol`
- Provider-prefixed names such as `openai/gpt-5.6-sol`

Each GPT-5.6 variant should normalize to its exact base model, not to `gpt-5.1`, `gpt-5.4`, or `gpt-5.5`.

### Billing Fallback

In `backend/internal/service/billing_service.go`, wire GPT-5.6 fallback prices to the existing GPT-5.4 fallback:

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

In `getFallbackPricing`, return those fallback entries after `normalizeCodexModel` resolves them.

In `isOpenAIGPT54Model`, include the three GPT-5.6 models so long-context pricing policy applies when pricing data lacks long-context multipliers.

### Dynamic Pricing Fallback

In `backend/internal/service/pricing_service.go`, update `matchOpenAIModel` so any model starting with `gpt-5.6` falls back to `openAIGPT54FallbackPricing` when dynamic pricing does not have an exact or variant match.

This mirrors `gpt-5.5` behavior and preserves existing GPT-5.4 mini/nano special cases.

### Pricing Resource

In `backend/resources/model-pricing/model_prices_and_context_window.json`, add the three upstream model objects before `gpt-5.5`.

Use the upstream fields, including:

- `max_input_tokens`: `1050000`
- `max_output_tokens`: `128000`
- `supported_endpoints`: `/v1/chat/completions`, `/v1/batch`, `/v1/responses`
- Text and image input support
- Prompt caching, reasoning, service tier, tool choice, vision, web search support
- Long-context fields above `272k` tokens

## Frontend Design

### Model Whitelist

In `frontend/src/composables/useModelWhitelist.ts`, add the three GPT-5.6 models to the OpenAI list near the existing GPT-5.5/GPT-5.4 entries.

Add OpenAI preset mapping buttons:

- `GPT-5.6 Sol`
- `GPT-5.6 Terra`
- `GPT-5.6 Luna`

Each preset maps the model to itself. Use distinct existing Tailwind color families, following the upstream choices unless they conflict with local style.

### Use Key Modal

In `frontend/src/components/keys/UseKeyModal.vue`, add GPT-5.6 entries to the generated OpenCode `openaiModels` object.

Each model should use:

- `context`: `1050000`
- `output`: `128000`
- `store`: `false`
- `variants`: `low`, `medium`, `high`, `xhigh`

Do not change the generated Codex `config.toml` default model behavior. The modal already resolves `props.openaiUseKeyModelId`; this migration only makes GPT-5.6 valid when configured or selected.

## Error Handling

Unknown OpenAI models should continue to return the existing pricing error path. The GPT-5.6 migration should not broaden fallback matching to arbitrary `gpt-*` models.

If dynamic pricing data is unavailable or does not contain GPT-5.6, the billing path should still calculate cost via GPT-5.4 fallback. If dynamic pricing does contain GPT-5.6, it should be preferred.

## Testing

Backend tests:

- Extend normalization tests to cover `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, at least one reasoning suffix, and one space-separated alias.
- Add or extend billing fallback tests to verify GPT-5.6 uses GPT-5.4 fallback prices.
- Add or extend long-context billing tests to verify GPT-5.6 inherits GPT-5.4 multipliers.
- Add or extend `PricingService` fallback tests to verify dynamic pricing absence falls back to `openAIGPT54FallbackPricing`.

Frontend tests:

- Extend `useModelWhitelist` tests to assert the three models appear for OpenAI.
- Extend model mapping tests to ensure whitelist mapping can produce self-mappings for GPT-5.6.
- Extend `UseKeyModal` tests to assert the OpenCode generated config includes the GPT-5.6 model metadata.

Verification commands should focus on the touched areas:

- From `backend/`: `go test -tags unit ./internal/service ./internal/pkg/openai`
- From the repo root: `pnpm --dir frontend test -- useModelWhitelist UseKeyModal`

If the repo's exact test command differs locally, use the closest existing targeted command from package scripts.

## Non-Goals

- Do not sync all of `upstream/main`.
- Do not introduce `openai_model_alias.go`.
- Do not change `DefaultTestModel`.
- Do not change admin default `openai_use_key_model_id`.
- Do not change model pricing for unrelated GPT, Claude, Gemini, xAI, or image models.
- Do not refactor frontend model whitelist structure.

## Implementation Boundary

The implementation should be a small patch over the current branch. It should adapt upstream semantics to local structure and include targeted tests. Broader upstream changes, including account import, risk control, batch image, or new auth flows, are outside this migration.
