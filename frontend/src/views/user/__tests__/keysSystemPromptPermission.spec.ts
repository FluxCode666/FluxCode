import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const keysViewSource = readFileSync(resolve(__dirname, '../KeysView.vue'), 'utf8')

describe('KeysView system prompt permission', () => {
  it('hides API Key system prompt fields when current user cannot configure them', () => {
    expect(keysViewSource).toContain('canConfigureSystemPrompt')
    expect(keysViewSource).toContain('v-if="canConfigureSystemPrompt"')
    expect(keysViewSource.indexOf('v-if="canConfigureSystemPrompt"')).toBeLessThan(
      keysViewSource.indexOf('<SystemPromptConfigFields')
    )
  })

  it('only submits API Key system prompt payload when the current user is allowed', () => {
    expect(keysViewSource).toContain('systemPromptConfigPayload')
    expect(keysViewSource).toContain('canConfigureSystemPrompt.value')
    expect(keysViewSource).toContain('...(systemPromptConfigPayload.value')
  })
})
