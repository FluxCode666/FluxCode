import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, type PropType } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { MediaModelDefinition } from '@/types'
import MediaModelsView from '../MediaModelsView.vue'

const { createMock, listMock, removeMock, showErrorMock, showSuccessMock, updateMock } =
  vi.hoisted(() => ({
    createMock: vi.fn(),
    listMock: vi.fn(),
    removeMock: vi.fn(),
    showErrorMock: vi.fn(),
    showSuccessMock: vi.fn(),
    updateMock: vi.fn(),
  }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    mediaModels: {
      create: createMock,
      list: listMock,
      remove: removeMock,
      update: updateMock,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: showSuccessMock }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function readyViewModel(): MediaModelDefinition {
  return {
    id: 1,
    model_id: 'grok-2-image',
    vendor: 'xai',
    media_type: 'image',
    operations: ['text_to_image'],
    constraints: {},
    billing_unit: 'image',
    enabled: true,
    aliases: ['grok-image'],
    adapter_resolution: {
      status: 'ready',
      resolved_adapter: 'xai-image',
      matched_by: 'family',
      matched_family: 'grok-image',
      capabilities: {
        operations: ['text_to_image'],
        sync_upstream: true,
        native_async_upstream: false,
        content_fetch: true,
      },
      reason_code: '',
    },
  }
}

const BaseDialogStub = defineComponent({
  props: { show: Boolean, title: String },
  template: '<section v-if="show"><slot /><slot name="footer" /></section>',
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array as PropType<MediaModelDefinition[]>, default: () => [] },
  },
  template: `
    <section>
      <article v-for="row in data" :key="row.id" :data-test="'media-row-' + row.id">
        <slot name="cell-model_id" :row="row" />
        <slot name="cell-adapter_resolution" :row="row" />
        <slot name="cell-enabled" :row="row" />
        <slot name="cell-actions" :row="row" />
      </article>
    </section>
  `,
})

const MediaModelEditorStub = defineComponent({
  name: 'MediaModelEditor',
  props: {
    modelValue: { type: Object, required: true },
    adapterResolution: { type: Object, default: null },
    editing: Boolean,
  },
  emits: ['update:modelValue', 'update:valid'],
  mounted() {
    this.$emit('update:valid', true)
  },
  template: `
    <div data-test="media-model-editor-stub">
      <span data-test="editor-resolution">{{ adapterResolution?.resolved_adapter || 'pending' }}</span>
    </div>
  `,
})

function mountView() {
  return mount(MediaModelsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="actions" /><slot name="filters" /><slot name="table" /><slot /></div>',
        },
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        DataTable: DataTableStub,
        EmptyState: true,
        Icon: true,
        MediaModelEditor: MediaModelEditorStub,
      },
    },
  })
}

describe('MediaModelsView', () => {
  beforeEach(() => {
    createMock.mockReset()
    listMock.mockReset()
    removeMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    updateMock.mockReset()

    listMock.mockResolvedValue([readyViewModel()])
    createMock.mockResolvedValue(readyViewModel())
    updateMock.mockResolvedValue(readyViewModel())
    removeMock.mockResolvedValue(undefined)
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
  })

  it('在列表和编辑器中只读展示系统解析结果', async () => {
    const wrapper = mountView()
    await flushPromises()

    const row = wrapper.get('[data-test="media-row-1"]')
    expect(row.text()).toContain('xai-image')
    expect(row.text()).toContain('grok-image')
    expect(row.text()).toContain('admin.mediaModels.resolution.capabilities.sync')

    await wrapper.get('[data-test="edit-media-model-1"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="editor-resolution"]').text()).toBe('xai-image')
  })

  it('创建、编辑与启停 payload 均不含废弃 Adapter 字段', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="create-media-model"]').trigger('click')
    await flushPromises()
    await wrapper.get('#media-model-form').trigger('submit')
    await flushPromises()
    expect(createMock.mock.calls[0][0]).not.toHaveProperty('default_adapter')
    expect(createMock.mock.calls[0][0]).not.toHaveProperty('default_async_mode')

    await wrapper.get('[data-test="edit-media-model-1"]').trigger('click')
    await flushPromises()
    await wrapper.get('#media-model-form').trigger('submit')
    await flushPromises()
    expect(updateMock.mock.calls[0][1]).not.toHaveProperty('default_adapter')
    expect(updateMock.mock.calls[0][1]).not.toHaveProperty('default_async_mode')

    await wrapper.get('[data-test="toggle-media-model-1"]').trigger('click')
    await flushPromises()
    const togglePayload = updateMock.mock.calls.at(-1)?.[1]
    expect(togglePayload).not.toHaveProperty('default_adapter')
    expect(togglePayload).not.toHaveProperty('default_async_mode')
  })
})
