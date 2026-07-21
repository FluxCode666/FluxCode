import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick, type PropType } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup, MediaModelDefinition } from '@/types'
import GroupsView from '../GroupsView.vue'

const {
  createGroup,
  getCapacitySummary,
  getUsageSummary,
  listGroups,
  listMediaModels,
  getMediaScopes,
  replaceMediaScopes,
  showError,
  showSuccess,
  updateGroup,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  getCapacitySummary: vi.fn(),
  getUsageSummary: vi.fn(),
  listGroups: vi.fn(),
  listMediaModels: vi.fn(),
  getMediaScopes: vi.fn(),
  replaceMediaScopes: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  updateGroup: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getById: vi.fn(),
    },
    groups: {
      create: createGroup,
      getCapacitySummary,
      getUsageSummary,
      list: listGroups,
      update: updateGroup,
    },
    mediaModels: {
      listEnabled: listMediaModels,
      getGroupScopes: getMediaScopes,
      replaceGroupScopes: replaceMediaScopes,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: () => false,
    nextStep: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

type LegacyAdminGroup = Omit<
  AdminGroup,
  | 'allow_image_generation'
  | 'allow_video_generation'
  | 'media_cross_platform_enabled'
> & {
  allow_image_generation?: boolean | null
  allow_video_generation?: boolean | null
  media_cross_platform_enabled?: boolean | null
}

const legacyGroup = (overrides: Partial<LegacyAdminGroup> = {}): LegacyAdminGroup => ({
  id: 1,
  name: 'legacy-group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  system_prompt: '',
  system_prompt_mode: 'inherit',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  is_fallback_group: false,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 0,
  ...overrides,
})

const readyMediaModel = (modelID = 'ready-image'): MediaModelDefinition => ({
  id: 1,
  model_id: modelID,
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'xai-image',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_image'],
      sync_upstream: true,
      native_async_upstream: false,
      content_fetch: false,
    },
    reason_code: '',
  },
})

const BaseDialogStub = defineComponent({
  props: {
    show: Boolean,
    title: String,
  },
  emits: ['close'],
  template: `
    <section v-if="show" :data-dialog-title="title">
      <button data-test="dialog-close" type="button" @click="$emit('close')">close</button>
      <slot />
      <slot name="footer" />
    </section>
  `,
})

const DataTableStub = defineComponent({
  props: {
    data: {
      type: Array as PropType<LegacyAdminGroup[]>,
      default: () => [],
    },
  },
  template: `
    <div>
      <div v-for="group in data" :key="group.id" :data-test="'group-row-' + group.id">
        <slot name="cell-actions" :row="group" />
      </div>
    </div>
  `,
})

const mountView = () =>
  mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template:
            '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /><slot /></div>',
        },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        SystemPromptConfigFields: true,
        PlatformIcon: true,
        Icon: true,
        GroupRateMultipliersModal: true,
        GroupCapacityBadge: true,
        VueDraggable: true,
      },
    },
  })

describe('GroupsView media permissions', () => {
  beforeEach(() => {
    createGroup.mockReset()
    getCapacitySummary.mockReset()
    getUsageSummary.mockReset()
    listGroups.mockReset()
    listMediaModels.mockReset()
    getMediaScopes.mockReset()
    replaceMediaScopes.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    updateGroup.mockReset()

    createGroup.mockResolvedValue(undefined)
    getCapacitySummary.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    listGroups.mockResolvedValue({ items: [], total: 0, pages: 0 })
    listMediaModels.mockResolvedValue([])
    getMediaScopes.mockResolvedValue([])
    replaceMediaScopes.mockResolvedValue([])
    updateGroup.mockResolvedValue(undefined)
  })

  it('创建时连续更新三个字段、发送真实请求并在关闭后重置', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-tour="groups-create-btn"]').trigger('click')
    await wrapper.get('#create-group-form [data-tour="group-form-name"]').setValue('media')

    const createForm = wrapper.get('#create-group-form')
    expect(createForm.get('[data-test="allow-image-generation"]').attributes('checked')).toBeUndefined()
    expect(createForm.get('[data-test="allow-video-generation"]').attributes('checked')).toBeUndefined()
    expect(createForm.get('[data-test="media-cross-platform-enabled"]').attributes('checked')).toBeUndefined()

    await createForm.get('[data-test="allow-image-generation"]').setValue(true)
    await createForm.get('[data-test="allow-video-generation"]').setValue(true)
    await createForm.get('[data-test="media-cross-platform-enabled"]').setValue(true)
    await createForm.trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        allow_image_generation: true,
        allow_video_generation: true,
        media_cross_platform_enabled: true,
      }),
    )

    await wrapper.get('[data-tour="groups-create-btn"]').trigger('click')
    const resetForm = wrapper.get('#create-group-form')
    expect(resetForm.get('[data-test="allow-image-generation"]').attributes('checked')).toBeUndefined()
    expect(resetForm.get('[data-test="allow-video-generation"]').attributes('checked')).toBeUndefined()
    expect(resetForm.get('[data-test="media-cross-platform-enabled"]').attributes('checked')).toBeUndefined()
  })

  it('编辑时覆盖先前状态，将旧数据的 missing/null 水合为 false，并提交三个字段', async () => {
    listGroups.mockResolvedValue({
      items: [
        legacyGroup({
          id: 10,
          name: 'enabled-media',
          allow_image_generation: true,
          allow_video_generation: true,
          media_cross_platform_enabled: true,
        }),
        legacyGroup({
          id: 11,
          name: 'legacy-media',
          allow_video_generation: null,
        }),
      ],
      total: 2,
      pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="group-row-10"] button').trigger('click')
    await flushPromises()
    const enabledForm = wrapper.get('#edit-group-form')
    expect(enabledForm.get('[data-test="allow-image-generation"]').attributes()).toHaveProperty('checked')
    expect(enabledForm.get('[data-test="allow-video-generation"]').attributes()).toHaveProperty('checked')
    expect(enabledForm.get('[data-test="media-cross-platform-enabled"]').attributes()).toHaveProperty('checked')
    expect(enabledForm.findAll('[data-test="allow-image-generation"]')).toHaveLength(1)
    expect(enabledForm.text()).not.toContain(
      'admin.groups.openaiMessages.allowImageGeneration',
    )

    await wrapper.get('[data-test="dialog-close"]').trigger('click')
    await wrapper.get('[data-test="group-row-11"] button').trigger('click')
    await flushPromises()

    const legacyForm = wrapper.get('#edit-group-form')
    expect(legacyForm.get('[data-test="allow-image-generation"]').attributes('checked')).toBeUndefined()
    expect(legacyForm.get('[data-test="allow-video-generation"]').attributes('checked')).toBeUndefined()
    expect(legacyForm.get('[data-test="media-cross-platform-enabled"]').attributes('checked')).toBeUndefined()

    await legacyForm.get('[data-test="allow-image-generation"]').setValue(true)
    await legacyForm.get('[data-test="allow-video-generation"]').setValue(true)
    await legacyForm.get('[data-test="media-cross-platform-enabled"]').setValue(true)
    await legacyForm.trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(
      11,
      expect.objectContaining({
        allow_image_generation: true,
        allow_video_generation: true,
        media_cross_platform_enabled: true,
      }),
    )
  })

  it('媒体分组创建后保存独立的模型 scope', async () => {
    listMediaModels.mockResolvedValue([{
      ...readyMediaModel('seedance'),
      vendor: 'bytedance',
      media_type: 'video',
      operations: ['text_to_video'],
      billing_unit: 'second',
      adapter_resolution: {
        ...readyMediaModel('seedance').adapter_resolution,
        resolved_adapter: 'volcengine-seedance',
        capabilities: {
          operations: ['text_to_video'],
          sync_upstream: false,
          native_async_upstream: true,
          content_fetch: false,
        },
      },
    }])
    const created = legacyGroup({ id: 21, name: 'media-group', platform: 'media' }) as AdminGroup
    createGroup.mockResolvedValue(created)
    replaceMediaScopes.mockResolvedValue(['seedance'])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-tour="groups-create-btn"]').trigger('click')
    ;(wrapper.vm as any).createForm.platform = 'media'
    await nextTick()

    await wrapper.get('#create-group-form [data-tour="group-form-name"]').setValue('media-group')
    await wrapper.get('[data-test="media-model-scope-seedance"]').setValue(true)
    await wrapper.get('#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroup).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'media',
      media_cross_platform_enabled: false,
    }))
    expect(replaceMediaScopes).toHaveBeenCalledWith(21, ['seedance'])
  })

  it('允许清空全部由当前不可用模型组成的历史授权', async () => {
    listGroups.mockResolvedValue({
      items: [legacyGroup({ id: 30, name: 'historical-media', platform: 'media' })],
      total: 1,
      pages: 1,
    })
    listMediaModels.mockResolvedValue([readyMediaModel()])
    getMediaScopes.mockResolvedValue(['removed-image'])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-row-30"] button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="remove-unavailable-media-model-removed-image"]').trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalled()
    expect(replaceMediaScopes).toHaveBeenCalledWith(30, [])
  })

  it('模型列表加载失败时不展示历史删除入口且拒绝提交空授权', async () => {
    listGroups.mockResolvedValue({
      items: [legacyGroup({ id: 31, name: 'failed-media', platform: 'media' })],
      total: 1,
      pages: 1,
    })
    listMediaModels.mockRejectedValue(new Error('registry unavailable'))
    getMediaScopes.mockResolvedValue(['removed-image'])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-row-31"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="remove-unavailable-media-model-removed-image"]').exists())
      .toBe(false)
    ;(wrapper.vm as any).editMediaModelIds = []
    await nextTick()
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).not.toHaveBeenCalled()
    expect(replaceMediaScopes).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.mediaModelScopeRequired')
  })

  it('媒体分组授权加载失败时即使重新选择 ready 模型也拒绝覆盖', async () => {
    listGroups.mockResolvedValue({
      items: [legacyGroup({ id: 34, name: 'scope-load-failed', platform: 'media' })],
      total: 1,
      pages: 1,
    })
    listMediaModels.mockResolvedValue([readyMediaModel()])
    getMediaScopes.mockRejectedValue(new Error('scope unavailable'))

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-row-34"] button').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.groups.mediaModelScopeLoadFailed')
    showError.mockClear()
    await wrapper.get('[data-test="media-model-scope-ready-image"]').setValue(true)
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).not.toHaveBeenCalled()
    expect(replaceMediaScopes).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.mediaModelScopeLoadFailed')
  })

  it('原始授权含 ready 模型时不允许全部清空', async () => {
    listGroups.mockResolvedValue({
      items: [legacyGroup({ id: 32, name: 'ready-media', platform: 'media' })],
      total: 1,
      pages: 1,
    })
    listMediaModels.mockResolvedValue([readyMediaModel()])
    getMediaScopes.mockResolvedValue(['ready-image'])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-row-32"] button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="media-model-scope-ready-image"]').setValue(false)
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).not.toHaveBeenCalled()
    expect(replaceMediaScopes).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.mediaModelScopeRequired')
  })

  it('原始授权混合 ready 与不可用模型时同样不允许全部清空', async () => {
    listGroups.mockResolvedValue({
      items: [legacyGroup({ id: 33, name: 'mixed-media', platform: 'media' })],
      total: 1,
      pages: 1,
    })
    listMediaModels.mockResolvedValue([readyMediaModel()])
    getMediaScopes.mockResolvedValue(['ready-image', 'removed-image'])

    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-row-33"] button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="media-model-scope-ready-image"]').setValue(false)
    await wrapper.get('[data-test="remove-unavailable-media-model-removed-image"]').trigger('click')
    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).not.toHaveBeenCalled()
    expect(replaceMediaScopes).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.groups.mediaModelScopeRequired')
  })
})
