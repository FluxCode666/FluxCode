import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MediaStorageSettingsCard from '../MediaStorageSettingsCard.vue'

const { getMock, updateMock, testMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  updateMock: vi.fn(),
  testMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/api/admin/settings', () => ({
  getMediaStorageConfig: getMock,
  updateMediaStorageConfig: updateMock,
  testMediaStorageConfig: testMock,
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (error: unknown) => String(error),
}))

const config = {
  provider: 'local' as const,
  local_path: '/app/.fluxcode/generated',
  minio: {
    endpoint: 'https://minio.example.com',
    bucket: 'media',
    access_key_id: 'access',
    secret_access_key: '',
    secret_access_key_configured: true,
    region: 'us-east-1',
    use_ssl: true,
    force_path_style: true,
    prefix: 'media',
  },
}

describe('MediaStorageSettingsCard', () => {
  beforeEach(() => {
    getMock.mockReset().mockResolvedValue(structuredClone(config))
    updateMock.mockReset().mockImplementation(async (value) => ({
      ...value,
      minio: { ...value.minio, secret_access_key: '', secret_access_key_configured: true },
    }))
    testMock.mockReset().mockResolvedValue({ ok: true, message: 'ok' })
  })

  it('加载 Local 默认路径并显示多实例警告', async () => {
    const wrapper = mount(MediaStorageSettingsCard)
    await flushPromises()

    expect(wrapper.get<HTMLSelectElement>('[data-test="media-storage-provider"]').element.value).toBe('local')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-local-path"]').element.value).toBe('/app/.fluxcode/generated')
    expect(wrapper.text()).toContain('admin.settings.mediaStorage.multiInstanceWarning')
  })

  it('切换 MinIO 后测试并保存，空密钥由后端保留', async () => {
    const wrapper = mount(MediaStorageSettingsCard)
    await flushPromises()
    await wrapper.get('[data-test="media-storage-provider"]').setValue('minio')
    expect(wrapper.get('[data-test="media-minio-fields"]').exists()).toBe(true)

    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    await flushPromises()
    expect(testMock).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'minio',
      minio: expect.objectContaining({ secret_access_key: '' }),
    }))

    await buttons[1].trigger('click')
    await flushPromises()
    expect(updateMock).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('admin.settings.mediaStorage.saved')
  })
})
