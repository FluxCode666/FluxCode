import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { config, flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import IntegrationDocsView from '../IntegrationDocsView.vue'

const { fetchPublicSettings, copyToClipboard } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'FluxCode',
    siteLogo: '',
    apiBaseUrl: 'https://gateway.example.com/api/v1',
    openaiUseKeyModelId: '',
    fetchPublicSettings
  })
}))

vi.mock('@/utils/openaiUseKeyModel', () => ({
  resolveOpenAIUseKeyModelId: () => 'gpt-4.1'
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        const map: Record<string, string> = {
          'integrationDocs.nav.overview': '接入总览',
          'integrationDocs.fields.authHeader': '鉴权 Header',
          'integrationDocs.fields.contentType': '内容类型',
          'integrationDocs.fields.versionHeader': '协议版本 Header',
          'integrationDocs.fields.endpoint': '请求路径',
          'integrationDocs.fields.method': '请求方法',
          'integrationDocs.fields.parametersTitle': '请求参数',
          'integrationDocs.fields.parameterName': '参数名',
          'integrationDocs.fields.parameterType': '类型',
          'integrationDocs.fields.parameterRequired': '是否必填',
          'integrationDocs.fields.parameterDescription': '说明',
          'integrationDocs.fields.requiredLabel': '必填',
          'integrationDocs.fields.optionalLabel': '可选',
          'integrationDocs.fields.supportedValues': '支持值',
          'integrationDocs.fields.exampleTabsTitle': '调用示例',
          'common.copied': '已复制',
          'common.copiedToClipboard': '已复制到剪贴板',
          'keys.copyToClipboard': '复制到剪贴板',
          'integrationDocs.hero.badge': '开发者接入',
          'integrationDocs.hero.title': '接入文档',
          'integrationDocs.hero.subtitle': '接入说明',
          'integrationDocs.hero.cards.baseUrl': 'Base URL',
          'integrationDocs.hero.cards.auth': '默认鉴权',
          'integrationDocs.hero.cards.compatibility': '兼容协议',
          'integrationDocs.sidebar.title': '页面目录',
          'integrationDocs.overview.title': '接入总览',
          'integrationDocs.overview.description': '总览描述',
          'integrationDocs.overview.callout': '总览提示',
          'integrationDocs.overview.pathsTitle': '标准路径',
          'integrationDocs.overview.pathsDescription': '路径说明',
          'integrationDocs.overview.notesTitle': '接入注意事项',
          'integrationDocs.overview.steps.step1.title': '获取 API Key',
          'integrationDocs.overview.steps.step1.description': '获取 Key',
          'integrationDocs.overview.steps.step2.title': '替换 Base URL',
          'integrationDocs.overview.steps.step2.description': '替换域名',
          'integrationDocs.overview.steps.step3.title': '发送最小请求',
          'integrationDocs.overview.steps.step3.description': '验证请求',
          'integrationDocs.overview.notes.item1': '提示 1',
          'integrationDocs.overview.notes.item2': '提示 2',
          'integrationDocs.overview.notes.item3': '提示 3',
          'integrationDocs.overview.notes.item4': '提示 4',
          'integrationDocs.sections.openaiChat.badge': 'OpenAI Compatible',
          'integrationDocs.sections.openaiChat.title': 'OpenAI Chat 协议',
          'integrationDocs.sections.openaiChat.description': 'Chat 描述',
          'integrationDocs.sections.openaiChat.bullets.item1': 'Chat 要点 1',
          'integrationDocs.sections.openaiChat.bullets.item2': 'Chat 要点 2',
          'integrationDocs.sections.openaiChat.bullets.item3': 'Chat 要点 3',
          'integrationDocs.sections.openaiResponses.badge': 'OpenAI Native',
          'integrationDocs.sections.openaiResponses.title': 'OpenAI Responses 协议',
          'integrationDocs.sections.openaiResponses.description': 'Responses 描述',
          'integrationDocs.sections.openaiResponses.bullets.item1': 'Responses 要点 1',
          'integrationDocs.sections.openaiResponses.bullets.item2': 'Responses 要点 2',
          'integrationDocs.sections.openaiResponses.bullets.item3': 'Responses 要点 3',
          'integrationDocs.sections.anthropicMessages.badge': 'Anthropic Compatible',
          'integrationDocs.sections.anthropicMessages.title': 'Anthropic Messages 协议',
          'integrationDocs.sections.anthropicMessages.description': 'Anthropic 描述',
          'integrationDocs.sections.anthropicMessages.bullets.item1': 'Anthropic 要点 1',
          'integrationDocs.sections.anthropicMessages.bullets.item2': 'Anthropic 要点 2',
          'integrationDocs.sections.anthropicMessages.bullets.item3': 'Anthropic 要点 3',
          'integrationDocs.sections.openaiImages.badge': 'OpenAI Images',
          'integrationDocs.sections.openaiImages.title': 'OpenAI 生图接口',
          'integrationDocs.sections.openaiImages.description': 'Images 描述',
          'integrationDocs.sections.openaiImages.bullets.item1': 'Images 要点 1',
          'integrationDocs.sections.openaiImages.bullets.item2': 'Images 要点 2',
          'integrationDocs.sections.openaiImages.bullets.item3': 'Images 要点 3'
        }
        return map[key] || key
      },
      locale: { value: 'zh-CN' }
    })
  }
})

describe('IntegrationDocsView', () => {
  beforeEach(() => {
    config.global.stubs.RouterLink = RouterLinkStub
    fetchPublicSettings.mockReset()
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)
  })

  afterEach(() => {
    delete config.global.stubs.RouterLink
  })

  it('renders supported parameter names for each protocol section', async () => {
    const wrapper = mount(IntegrationDocsView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    expect(fetchPublicSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('messages[].role')
    expect(wrapper.text()).toContain('previous_response_id')
    expect(wrapper.text()).toContain('images[].image_url')
  })

  it('switches code example tabs independently per section', async () => {
    const wrapper = mount(IntegrationDocsView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="example-tab-openai-chat-curl"]').text()).toBe('cURL')
    expect(wrapper.get('[data-testid="example-tab-openai-chat-python"]').text()).toBe('Python')
    expect(wrapper.get('[data-testid="example-tab-openai-chat-javascript"]').text()).toBe('JavaScript')
    expect(wrapper.get('[data-testid="example-tab-openai-chat-java"]').text()).toBe('Java')

    expect(wrapper.get('[data-testid="example-code-openai-chat"]').text()).toContain('curl "https://gateway.example.com/v1/chat/completions"')

    await wrapper.get('[data-testid="example-tab-openai-chat-python"]').trigger('click')

    expect(wrapper.get('[data-testid="example-code-openai-chat"]').text()).toContain('import requests')
    expect(wrapper.get('[data-testid="example-code-openai-chat"]').text()).toContain('https://gateway.example.com/v1/chat/completions')
    expect(wrapper.get('[data-testid="example-code-openai-responses"]').text()).toContain('curl "https://gateway.example.com/v1/responses"')
  })

  it('copies the active example snippet', async () => {
    const wrapper = mount(IntegrationDocsView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="example-tab-openai-chat-python"]').trigger('click')
    await wrapper.get('[data-testid="example-copy-openai-chat"]').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledTimes(1)
    expect(copyToClipboard).toHaveBeenCalledWith(
      expect.stringContaining('import requests'),
      '已复制到剪贴板'
    )
    expect(wrapper.get('[data-testid="example-copy-openai-chat"]').text()).toContain('已复制')
  })

  it('highlights the current elevator item when a nav item is selected', async () => {
    const wrapper = mount(IntegrationDocsView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    const targetLink = wrapper.get('aside a[href="#openai-chat"]')
    await targetLink.trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('aside a[href="#openai-chat"]').attributes('aria-current')).toBe('location')
  })
})
