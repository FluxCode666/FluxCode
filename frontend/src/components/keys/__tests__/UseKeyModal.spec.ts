import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

import UseKeyModal from '../UseKeyModal.vue'

describe('UseKeyModal', () => {
  const mountModal = (props: Record<string, unknown> = {}) => mount(UseKeyModal, {
    props: {
      show: true,
      apiKey: 'sk-test',
      baseUrl: 'https://example.com/v1',
      platform: 'openai',
      ...props
    },
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>'
        },
        Icon: {
          template: '<span />'
        }
      }
    }
  })

  it('renders updated GPT-5.4 mini/nano names in OpenCode config', async () => {
    const wrapper = mountModal()

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )

    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"gpt-5.6"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.6 (Sol)"')
    expect(codeBlock.text()).toContain('"gpt-5.6-sol"')
    expect(codeBlock.text()).toContain('"gpt-5.6-terra"')
    expect(codeBlock.text()).toContain('"gpt-5.6-luna"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.6 Sol"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.6 Terra"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.6 Luna"')
    expect(codeBlock.text()).toContain('"context": 1050000')
    expect(codeBlock.text()).toContain('"output": 128000')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Mini"')
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Nano"')

    const config = JSON.parse(codeBlock.text()) as {
      provider: {
        openai: {
          models: Record<string, { variants: Record<string, unknown> }>
        }
      }
    }

    for (const model of ['gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna']) {
      expect(config.provider.openai.models[model].variants.max).toEqual({})
    }
  })

  it('renders OpenAI Windows quick setup as a single PowerShell command', async () => {
    const wrapper = mountModal()

    const windowsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('Windows')
    )

    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code')
    const quickSetupCommand = codeBlocks.at(-1)?.text() ?? ''

    expect(quickSetupCommand).toContain('$env:USERPROFILE\\.codex\\config.toml')
    expect(quickSetupCommand).toContain('$env:USERPROFILE\\.codex\\auth.json')
    expect(quickSetupCommand).not.toContain("@'")
    expect(quickSetupCommand).not.toMatch(/\r?\n/)
  })

  it('uses configured OpenAI model id in Codex config files', () => {
    const wrapper = mountModal({ openaiUseKeyModelId: 'gpt-5.4-mini' })

    const configBlock = wrapper.find('pre code').text()

    expect(configBlock).toContain('model = "gpt-5.4-mini"')
    expect(configBlock).toContain('review_model = "gpt-5.4-mini"')
    expect(configBlock).not.toContain('model = "gpt-5.4"')
  })

  it('falls back to GPT-5.5 when OpenAI model id is empty', () => {
    const wrapper = mountModal({ openaiUseKeyModelId: '   ' })

    const configBlock = wrapper.find('pre code').text()

    expect(configBlock).toContain('model = "gpt-5.5"')
    expect(configBlock).toContain('review_model = "gpt-5.5"')
  })

  it('renders dedicated embedding discovery, curl, and OpenAI SDK examples', () => {
    const wrapper = mountModal({ platform: 'embedding' })
    const text = wrapper.text()

    expect(text).toContain('GET /v1/models')
    expect(text).toContain('POST /v1/embeddings')
    expect(text).toContain('client.embeddings.create')
    expect(text).toContain('sk-test')
    expect(text).not.toContain('ANTHROPIC_BASE_URL')
    expect(text).not.toContain('OPENAI_BASE_URL')
  })
})
