import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  getAccessKey: vi.fn(),
  generateAccessKey: vi.fn(),
  fetchPublicSettings: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  copySensitiveToClipboard: vi.fn()
}))

vi.mock('@/api/userAccessKey', () => ({
  userAccessKeyAPI: {
    get: mocks.getAccessKey,
    generate: mocks.generateAccessKey
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    apiBaseUrl: 'https://gateway.example.com/api/v1',
    fetchPublicSettings: mocks.fetchPublicSettings,
    showSuccess: mocks.showSuccess,
    showError: mocks.showError
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copySensitiveToClipboard: mocks.copySensitiveToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' }
    })
  }
})

import ApiDocsView from '../ApiDocsView.vue'

const scrollIntoView = vi.fn()
const originalScrollIntoView = HTMLElement.prototype.scrollIntoView
let mountedWrapper: ReturnType<typeof mount> | null = null

function mountView() {
  const wrapper = mount(ApiDocsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        CodeBlock: {
          props: ['code'],
          template: '<pre><code>{{ code }}</code></pre>'
        },
        Icon: true
      }
    }
  })
  mountedWrapper = wrapper
  return wrapper
}

describe('ApiDocsView', () => {
  beforeEach(() => {
    mocks.getAccessKey.mockReset()
    mocks.generateAccessKey.mockReset()
    mocks.fetchPublicSettings.mockReset()
    mocks.showSuccess.mockReset()
    mocks.showError.mockReset()
    mocks.copySensitiveToClipboard.mockReset()

    mocks.getAccessKey.mockResolvedValue({ key: '', exists: false })
    mocks.copySensitiveToClipboard.mockResolvedValue(true)
    scrollIntoView.mockReset()
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView
    })
    window.history.replaceState(null, '', '/')
  })

  afterEach(() => {
    mountedWrapper?.unmount()
    mountedWrapper = null
  })

  afterAll(() => {
    if (originalScrollIntoView) {
      Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
        configurable: true,
        value: originalScrollIntoView
      })
      return
    }

    Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
  })

  it('展示已生成用户密钥的脱敏值，并允许再次复制完整密钥', async () => {
    const accessKey = 'uak_persisted_full_value'
    mocks.getAccessKey.mockResolvedValue({
      key: accessKey,
      exists: true,
      created_at: '2026-07-30T08:00:00Z'
    })

    const wrapper = mountView()
    await flushPromises()

    expect(mocks.getAccessKey).toHaveBeenCalledTimes(1)
    expect(mocks.fetchPublicSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="user-access-key-value"]').text()).toBe('uak_pers••••••••alue')
    expect(wrapper.html()).not.toContain(accessKey)
    expect(wrapper.find('[data-testid="user-access-key-generate"]').exists()).toBe(false)

    await wrapper.get('[data-testid="user-access-key-copy"]').trigger('click')

    expect(mocks.copySensitiveToClipboard).toHaveBeenCalledWith(accessKey, 'apiDocs.accessKey.copied')
  })

  it('在尚未生成密钥时生成，随后显示并允许复制新密钥', async () => {
    const generatedKey = 'uak_newly_generated_value'
    mocks.generateAccessKey.mockResolvedValue({ key: generatedKey, exists: true })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="user-access-key-copy"]').exists()).toBe(false)
    await wrapper.get('[data-testid="user-access-key-generate"]').trigger('click')
    await flushPromises()

    expect(mocks.generateAccessKey).toHaveBeenCalledTimes(1)
    expect(mocks.showSuccess).toHaveBeenCalledWith('apiDocs.accessKey.generated')
    expect(wrapper.get('[data-testid="user-access-key-value"]').text()).not.toContain(generatedKey)
    expect(wrapper.get('[data-testid="user-access-key-value"]').text()).toContain('••••••••')
    expect(wrapper.html()).not.toContain(generatedKey)

    await wrapper.get('[data-testid="user-access-key-copy"]').trigger('click')

    expect(mocks.copySensitiveToClipboard).toHaveBeenCalledWith(generatedKey, 'apiDocs.accessKey.copied')
  })

  it('生成失败时显示错误提示且不把页面切换为可复制状态', async () => {
    mocks.generateAccessKey.mockRejectedValue(new Error('network failure'))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="user-access-key-generate"]').trigger('click')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('apiDocs.accessKey.generateFailed')
    expect(wrapper.find('[data-testid="user-access-key-copy"]').exists()).toBe(false)
  })

  it('未配置外部加密密钥时明确提示且不允许生成用户密钥', async () => {
    mocks.getAccessKey.mockResolvedValue({ key: '', exists: false, available: false })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="user-access-key-unavailable"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="user-access-key-configuration-required"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="user-access-key-generate"]').exists()).toBe(false)
  })

  it('列出用户余额与 API Key 的完整 CRUD 接口文档', async () => {
    const wrapper = mountView()
    await flushPromises()

    const document = wrapper.text()

    expect(document).toContain('https://gateway.example.com/api/v1/openapi')
    expect(document).toContain('X-User-Access-Key: YOUR_USER_ACCESS_KEY')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/balance')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/keys')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/keys/:id')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/groups/available')
    expect(document).toContain('Idempotency-Key')
    expect(document).toContain('curl --request POST')
    expect(document).toContain('curl --request PUT')
    expect(document).toContain('curl --request DELETE')
  })

  it('按访问密钥、账户、使用记录和 API Key 管理分组，并完整展示使用记录接口', async () => {
    const wrapper = mountView()
    await flushPromises()

    const navigation = wrapper.get('[aria-labelledby="api-docs-navigation-title"]')
    expect(navigation.get('[data-testid="docs-nav-module-accessKey"]').attributes('aria-expanded')).toBe('true')
    expect(navigation.get('[data-testid="docs-nav-module-account"]').exists()).toBe(true)
    expect(navigation.get('[data-testid="docs-nav-module-usage"]').exists()).toBe(true)
    expect(navigation.get('[data-testid="docs-nav-module-apiKeys"]').exists()).toBe(true)
    expect(navigation.get('a[href="#access-key-read"]').exists()).toBe(true)
    expect(navigation.get('a[href="#access-key-generate"]').exists()).toBe(true)
    expect(navigation.get('a[href="#balance"]').exists()).toBe(true)
    expect(navigation.get('a[href="#usage-list"]').exists()).toBe(true)
    expect(navigation.get('a[href="#usage-stats"]').exists()).toBe(true)
    expect(navigation.get('a[href="#usage-detail"]').exists()).toBe(true)
    expect(navigation.get('a[href="#api-key-create"]').exists()).toBe(true)
    expect(navigation.get('a[href="#available-groups"]').exists()).toBe(true)

    const document = wrapper.text()
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/usage')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/usage/stats')
    expect(document).toContain('https://gateway.example.com/api/v1/openapi/usage/:id')
    expect(document).toContain('page_size')
    expect(document).toContain('start_date')
    expect(document).toContain('end_date')
    expect(document).toContain('api_key_id')
    expect(document).toContain('sort_by')
    expect(document).toContain('sort_order')
    expect(document).toContain('--data-urlencode "page=1"')
    expect(document).toContain('--data-urlencode "sort_order=desc"')
  })

  it('左侧电梯的模块可独立收起和展开接口目录', async () => {
    const wrapper = mountView()
    await flushPromises()

    const moduleToggle = wrapper.get('[data-testid="docs-nav-module-usage"]')
    const endpointList = wrapper.get('#docs-nav-usage-endpoints')

    expect(wrapper.find('a[href="#usage"]').exists()).toBe(false)
    expect(moduleToggle.attributes('aria-expanded')).toBe('true')
    await moduleToggle.trigger('click')
    expect(moduleToggle.attributes('aria-expanded')).toBe('false')
    expect(endpointList.attributes('style')).toContain('display: none')
    expect(window.location.hash).toBe('')
    expect(scrollIntoView).not.toHaveBeenCalled()

    await moduleToggle.trigger('click')
    expect(moduleToggle.attributes('aria-expanded')).toBe('true')
    expect(endpointList.attributes('style')).not.toContain('display: none')
  })

  it('将每个文档接口拆为独立卡片，并为每张卡片展示参数与请求、响应示例', async () => {
    const wrapper = mountView()
    await flushPromises()

    const endpointCards = wrapper.findAll('[data-testid="api-endpoint-card"]')

    expect(endpointCards).toHaveLength(12)
    endpointCards.forEach((card) => {
      expect(card.text()).toContain('apiDocs.endpointDocumentation.requestParameters')
      expect(card.text()).toContain('apiDocs.endpointDocumentation.requestDescription')
      expect(card.text()).toContain('apiDocs.endpointDocumentation.responseParameters')
      expect(card.findAll('[data-testid="api-endpoint-request-description"]')).toHaveLength(1)
      expect(card.findAll('table')).toHaveLength(2)
      expect(card.findAll('pre')).toHaveLength(2)
    })

    expect(wrapper.get('#usage-list').attributes('class')).toContain('rounded-xl')
    expect(wrapper.get('#api-key-create').attributes('class')).toContain('rounded-xl')
    expect(wrapper.get('#api-key-create').text()).toContain('ip_whitelist')
    expect(wrapper.get('#api-key-create').text()).toContain('ip_blacklist')
    expect(wrapper.get('#api-key-update').text()).toContain('rate_limit_5h')
    expect(wrapper.get('#api-key-update').text()).toContain('rate_limit_1d')
    expect(wrapper.get('#api-key-update').text()).toContain('rate_limit_7d')

    const usageDefaultValues = wrapper.get('#usage-list').findAll('[data-testid="api-endpoint-default-value"]')
    expect(usageDefaultValues.map((cell) => cell.text())).toContain('1')
    expect(usageDefaultValues.map((cell) => cell.text())).toContain('-')
  })

  it('允许折叠模块，并在目录点击具体接口时展开模块并滚动定位', async () => {
    const wrapper = mountView()
    await flushPromises()

    const usageSection = wrapper.get('section#usage')
    const usageToggle = usageSection.get('button[aria-controls="usage-content"]')
    const usageContent = usageSection.get('#usage-content')

    expect(usageToggle.attributes('aria-expanded')).toBe('true')
    await usageToggle.trigger('click')
    expect(usageToggle.attributes('aria-expanded')).toBe('false')
    expect(usageContent.attributes('style')).toContain('display: none')

    const usageStatsTarget = wrapper.get('#usage-stats').element
    await wrapper.get('a[href="#usage-stats"]').trigger('click')
    await flushPromises()

    expect(usageToggle.attributes('aria-expanded')).toBe('true')
    expect(usageContent.attributes('style')).not.toContain('display: none')
    expect(wrapper.get('a[href="#usage-stats"]').attributes('aria-current')).toBe('location')
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
    expect(scrollIntoView.mock.contexts).toContain(usageStatsTarget)
    expect(window.location.hash).toBe('#usage-stats')
    expect(document.activeElement).toBe(usageStatsTarget)
  })

  it('在不支持 scrollIntoView 时仍保留具体接口锚点定位', async () => {
    const wrapper = mountView()
    await flushPromises()

    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: undefined
    })

    await wrapper.get('a[href="#api-key-create"]').trigger('click')
    await flushPromises()

    expect(window.location.hash).toBe('#api-key-create')
    expect(wrapper.get('a[href="#api-key-create"]').attributes('aria-current')).toBe('location')
  })

  it('支持通过模块深链直接定位，并为折叠控件提供关联的无障碍属性', async () => {
    window.history.replaceState(null, '', '/#usage-detail')

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('a[href="#usage-detail"]').attributes('aria-current')).toBe('location')
    expect(wrapper.get('[data-testid="docs-nav-module-usage"]').attributes('aria-expanded')).toBe('true')
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'start' })
    expect(document.activeElement).toBe(wrapper.get('#usage-detail').element)

    const sections = [
      ['access-key', 'access-key-content'],
      ['account', 'account-content'],
      ['usage', 'usage-content'],
      ['api-key-management', 'api-keys-content']
    ]

    sections.forEach(([sectionId, contentId]) => {
      const section = wrapper.get('section#' + sectionId)
      const toggle = section.get('button[aria-controls="' + contentId + '"]')

      expect(toggle.attributes('aria-expanded')).toBe('true')
      expect(wrapper.get('#' + contentId).exists()).toBe(true)
    })

    expect(wrapper.html()).toContain('lg:grid')
    expect(wrapper.html()).toContain('overflow-x-auto')
  })
})
