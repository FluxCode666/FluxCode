import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, type PropType } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '../GroupsView.vue'

const {
  createGroup,
  getCapacitySummary,
  getUsageSummary,
  listGroups,
  showError,
  showSuccess,
  updateGroup,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  getCapacitySummary: vi.fn(),
  getUsageSummary: vi.fn(),
  listGroups: vi.fn(),
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
    showError.mockReset()
    showSuccess.mockReset()
    updateGroup.mockReset()

    createGroup.mockResolvedValue(undefined)
    getCapacitySummary.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    listGroups.mockResolvedValue({ items: [], total: 0, pages: 0 })
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
})
