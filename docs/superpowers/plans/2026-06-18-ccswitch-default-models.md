# CC-Switch Default Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Include the expected default model values in `ccswitch://v1/import` provider links for OpenAI/Codex and Claude.

**Architecture:** Move CC-Switch provider link construction into a focused utility so URL parameter behavior is directly unit-testable. `KeysView.vue` keeps platform/client routing and delegates parameter assembly to the utility. The utility reuses `resolveOpenAIUseKeyModelId` so OpenAI falls back to `gpt-5.5` consistently with the existing one-click Codex config.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, `URLSearchParams`, Vitest.

---

## File Structure

- Create `frontend/src/utils/ccswitchDeepLink.ts`: builds provider import deep links and owns CC-Switch default model constants.
- Create `frontend/src/utils/__tests__/ccswitchDeepLink.spec.ts`: verifies app-specific model parameters.
- Modify `frontend/src/views/user/KeysView.vue`: imports the builder and passes page-derived values into it.
- Read-only reference `frontend/src/utils/openaiUseKeyModel.ts`: existing OpenAI model fallback.

### Task 1: Add Tested CC-Switch Deep Link Builder

**Files:**
- Create: `frontend/src/utils/ccswitchDeepLink.ts`
- Create: `frontend/src/utils/__tests__/ccswitchDeepLink.spec.ts`
- Modify: `frontend/src/views/user/KeysView.vue`

- [ ] **Step 1: Write the failing utility test**

Create `frontend/src/utils/__tests__/ccswitchDeepLink.spec.ts`:

```ts
import { describe, expect, it } from 'vitest'

import {
  DEFAULT_CCSWITCH_CLAUDE_MODEL_ID,
  buildCcswitchProviderDeepLink
} from '../ccswitchDeepLink'

function paramsFor(link: string) {
  return new URL(link).searchParams
}

const baseOptions = {
  name: 'FluxCode',
  homepage: 'https://flux.example',
  endpoint: 'https://flux.example',
  apiKey: 'sk-test',
  usageScript: '({ request: { url: "{{baseUrl}}/v1/usage" } })'
}

describe('buildCcswitchProviderDeepLink', () => {
  it('adds GPT-5.5 as the default Codex model', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'codex',
      openaiModelId: '   '
    }))

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('model')).toBe('gpt-5.5')
    expect(params.has('opusModel')).toBe(false)
  })

  it('uses configured OpenAI model id for Codex imports', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'codex',
      openaiModelId: 'gpt-5.5-pro'
    }))

    expect(params.get('model')).toBe('gpt-5.5-pro')
  })

  it('adds Claude Opus 4.7 as the Claude default and opus model', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'claude'
    }))

    expect(params.get('model')).toBe(DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
    expect(params.get('opusModel')).toBe(DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  })

  it('does not add model fields for Gemini imports', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'gemini'
    }))

    expect(params.has('model')).toBe(false)
    expect(params.has('opusModel')).toBe(false)
  })
})
```

- [ ] **Step 2: Run the test to verify RED**

Run:

```bash
pnpm -C frontend vitest run src/utils/__tests__/ccswitchDeepLink.spec.ts
```

Expected: FAIL because `../ccswitchDeepLink` does not exist yet.

- [ ] **Step 3: Implement the minimal builder**

Create `frontend/src/utils/ccswitchDeepLink.ts`:

```ts
import { resolveOpenAIUseKeyModelId } from './openaiUseKeyModel'

export type CcswitchProviderApp = 'claude' | 'codex' | 'gemini'

export const DEFAULT_CCSWITCH_CLAUDE_MODEL_ID = 'claude-opus-4-7'

interface BuildCcswitchProviderDeepLinkOptions {
  app: CcswitchProviderApp
  name: string
  homepage: string
  endpoint: string
  apiKey: string
  usageScript: string
  openaiModelId?: string | null
}

export function buildCcswitchProviderDeepLink(options: BuildCcswitchProviderDeepLinkOptions): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app: options.app,
    name: options.name,
    homepage: options.homepage,
    endpoint: options.endpoint,
    apiKey: options.apiKey,
    configFormat: 'json',
    usageEnabled: 'true',
    usageScript: btoa(options.usageScript),
    usageAutoInterval: '30'
  })

  if (options.app === 'codex') {
    params.set('model', resolveOpenAIUseKeyModelId(options.openaiModelId))
  }

  if (options.app === 'claude') {
    params.set('model', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
    params.set('opusModel', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  }

  return `ccswitch://v1/import?${params.toString()}`
}
```

- [ ] **Step 4: Wire `KeysView.vue` to the builder**

In `frontend/src/views/user/KeysView.vue`, add this script import:

```ts
import { buildCcswitchProviderDeepLink, type CcswitchProviderApp } from '@/utils/ccswitchDeepLink'
```

In `executeCcsImport`, replace local `new URLSearchParams` and `deeplink` assembly with:

```ts
  const deeplink = buildCcswitchProviderDeepLink({
    app: app as CcswitchProviderApp,
    name: providerName,
    homepage: baseUrl,
    endpoint,
    apiKey: row.key,
    usageScript,
    openaiModelId: publicSettings.value?.openai_use_key_model_id
  })
```

- [ ] **Step 5: Run focused tests to verify GREEN**

Run:

```bash
pnpm -C frontend vitest run src/utils/__tests__/ccswitchDeepLink.spec.ts
```

Expected: PASS.

- [ ] **Step 6: Run relevant broader checks**

Run:

```bash
pnpm -C frontend vitest run src/components/keys/__tests__/UseKeyModal.spec.ts src/utils/__tests__/ccswitchDeepLink.spec.ts
pnpm -C frontend typecheck
```

Expected: both commands complete successfully.

- [ ] **Step 7: Commit implementation**

Stage only the new utility, its test, the plan, and the focused `KeysView.vue` hunks for this task:

```bash
git add docs/superpowers/plans/2026-06-18-ccswitch-default-models.md frontend/src/utils/ccswitchDeepLink.ts frontend/src/utils/__tests__/ccswitchDeepLink.spec.ts
git add -p frontend/src/views/user/KeysView.vue
git commit -m "feat: add ccswitch default model imports"
```

Expected: commit contains the builder, tests, plan, and only the `KeysView.vue` import/deep-link builder changes.
