import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.geminiImagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.',
    'admin.accounts.geminiImageTestHint': 'Gemini image test hint',
    'admin.accounts.geminiImageTestMode': 'Mode: Gemini image generation test',
    'admin.accounts.openaiImagePromptDefault': 'Generate a clean product-style OpenAI image test.',
    'admin.accounts.openaiImageTestHint': 'OpenAI image test hint',
    'admin.accounts.openaiImageTestMode': 'Mode: OpenAI image generation test',
    'admin.accounts.sendingGeminiImageRequest': 'Sending Gemini image generation test request...',
    'admin.accounts.sendingOpenAIImageRequest': 'Sending OpenAI image generation test request...'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.geminiImageReceived' && params?.count) {
          return `received-${params.count}`
        }
        return messages[key] || key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function mountModal(accountOverrides: Record<string, unknown> = {}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account: {
        id: 42,
        name: 'Gemini Image Test',
        platform: 'gemini',
        type: 'apikey',
        status: 'active',
        ...accountOverrides
      }
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
        TextArea: {
          props: ['modelValue', 'label', 'placeholder', 'hint'],
          emits: ['update:modelValue'],
          template: '<label><span class="textarea-label">{{ label }}</span><textarea class="textarea-stub" :placeholder="placeholder" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" /><small class="textarea-hint">{{ hint }}</small></label>'
        },
        Icon: true
      }
    }
  })
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut'
    })

    const preview = wrapper.find('img[alt="gemini-test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('openai 图片模型测试使用 OpenAI 生图文案和模式', async () => {
    getAvailableModels.mockResolvedValueOnce([
      { id: 'gpt-image-1', display_name: 'GPT Image 1' },
      { id: 'gpt-4.1', display_name: 'GPT 4.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gpt-image-1"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      name: 'OpenAI Image Test',
      platform: 'openai'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.find('.textarea-hint').text()).toBe('OpenAI image test hint')
    expect(wrapper.text()).toContain('Mode: OpenAI image generation test')
    expect(wrapper.text()).not.toContain('Mode: Gemini image generation test')
    expect(wrapper.find('textarea.textarea-stub').element.value).toBe('Generate a clean product-style OpenAI image test.')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('Sending OpenAI image generation test request...')
    expect(wrapper.text()).not.toContain('Sending Gemini image generation test request...')
  })

  it('embedding 账号测试直接选择显式公开模型首项', async () => {
    getAvailableModels.mockResolvedValueOnce([
      { id: 'public-embed-z', display_name: 'Public Z' },
      { id: 'public-embed-a', display_name: 'Public A' }
    ])
    const wrapper = mountModal({ name: 'Embedding Test', platform: 'embedding' })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const startButton = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.startTest'))
    await startButton!.trigger('click')
    await flushPromises()
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({ model_id: 'public-embed-z', prompt: '' })
  })
})
