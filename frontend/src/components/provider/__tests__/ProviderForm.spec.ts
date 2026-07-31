import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ProviderForm from '../ProviderForm.vue'
import type { ProviderWriteRequest } from '@/api/admin/providers'

const groupSelectorStub = {
  name: 'GroupSelector',
  props: ['modelValue', 'groups'],
  emits: ['update:modelValue'],
  template: '<div data-testid="group-selector">{{ modelValue.join(",") }}</div>'
}

function form(overrides: Partial<ProviderWriteRequest> = {}): ProviderWriteRequest {
  return {
    name: 'NewAPI',
    base_url: 'https://newapi.example.com',
    auth_type: 'bearer',
    allow_protocol_conversion: false,
    group_ids: [3],
    endpoints: [],
    capabilities: [],
    ...overrides
  }
}

function mountForm(modelValue = form()) {
  return mount(ProviderForm, {
    props: { modelValue },
    global: { stubs: { GroupSelector: groupSelectorStub } }
  })
}

function buttonByText(wrapper: ReturnType<typeof mountForm>, text: string) {
  const button = wrapper.findAll('button').find((item) => item.text() === text)
  if (!button) throw new Error(`button not found: ${text}`)
  return button
}

describe('ProviderForm', () => {
  it('保留 Group 关系且协议转换默认关闭', async () => {
    const wrapper = mountForm()

    await wrapper.find('form').trigger('submit')

    const payload = wrapper.emitted('submit')?.[0]?.[0] as ProviderWriteRequest
    expect(payload.group_ids).toEqual([3])
    expect(payload.allow_protocol_conversion).toBe(false)
    expect(wrapper.get('[data-testid="group-selector"]').text()).toBe('3')
  })

  it('切换为 Embeddings 时自动使用原生 embedding profile', async () => {
    const wrapper = mountForm()
    await buttonByText(wrapper, '添加能力').trigger('click')

    const protocolSelect = wrapper
      .findAll('select')
      .find((item) => item.findAll('option').some((option) => option.attributes('value') === 'embeddings'))
    expect(protocolSelect).toBeDefined()
    await protocolSelect!.setValue('embeddings')
    await wrapper.find('form').trigger('submit')

    const payload = wrapper.emitted('submit')?.[0]?.[0] as ProviderWriteRequest
    expect(payload.capabilities[0]).toMatchObject({
      protocol: 'embeddings',
      feature_profile: 'embeddings_v1'
    })
  })

  it('按协议生成端点路径并显式保存 wire profile', async () => {
    const wrapper = mountForm()
    await buttonByText(wrapper, '添加端点').trigger('click')

    const protocolSelect = wrapper
      .findAll('select')
      .find((item) => item.findAll('option').some((option) => option.attributes('value') === 'responses'))
    expect(protocolSelect).toBeDefined()
    await protocolSelect!.setValue('responses')
    await wrapper.find('form').trigger('submit')

    const payload = wrapper.emitted('submit')?.[0]?.[0] as ProviderWriteRequest
    expect(payload.endpoints[0]).toMatchObject({
      protocol: 'responses',
      path: '/v1/responses',
      wire_profile: 'canonical_v1'
    })
  })
})
