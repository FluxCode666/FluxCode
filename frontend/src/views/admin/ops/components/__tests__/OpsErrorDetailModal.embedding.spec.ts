import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getRequestErrorDetail, listRequestErrorUpstreamErrors } = vi.hoisted(() => ({
  getRequestErrorDetail: vi.fn(),
  listRequestErrorUpstreamErrors: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRequestErrorDetail,
    getUpstreamErrorDetail: vi.fn(),
    listRequestErrorUpstreamErrors
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'

describe('OpsErrorDetailModal embedding privacy', () => {
  it('does not render or fetch embedding content previews', async () => {
    getRequestErrorDetail.mockResolvedValue({
      id: 1,
      created_at: '2026-07-24T00:00:00Z',
      platform: 'embedding',
      request_type: 4,
      request_path: '/v1/embeddings',
      request_id: 'req-local',
      status_code: 502,
      phase: 'upstream',
      error_owner: 'provider',
      model: 'embed-public',
      message: 'embedding request failed',
      error_body: 'vector-canary',
      upstream_error_detail: 'upstream-body-canary'
    })

    const wrapper = mount(OpsErrorDetailModal, {
      props: { show: true, errorId: 1, errorType: 'request' },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.errorDetail.contentPreviewUnavailable')
    expect(wrapper.text()).not.toContain('vector-canary')
    expect(wrapper.text()).not.toContain('upstream-body-canary')
    expect(listRequestErrorUpstreamErrors).not.toHaveBeenCalled()
  })
})
