import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const {
  listSubscriptions,
  assignSubscription,
  listGroups,
  searchUsers,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  assignSubscription: vi.fn(),
  listGroups: vi.fn(),
  searchUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

const messages: Record<string, string> = {
  'common.cancel': '取消',
  'common.loading': '加载中',
  'common.noOptionsFound': '没有结果',
  'common.refresh': '刷新',
  'common.clear': '清空',
  'admin.users.searchUsers': '搜索用户',
  'admin.users.columnSettings': '列设置',
  'admin.users.columns.email': '邮箱',
  'admin.users.columns.username': '用户名',
  'admin.redeem.userPrefix': '用户 #{id}',
  'admin.usage.searchUserPlaceholder': '搜索用户',
  'admin.subscriptions.title': '订阅管理',
  'admin.subscriptions.assignSubscription': '分配订阅',
  'admin.subscriptions.assign': '分配',
  'admin.subscriptions.assigning': '分配中...',
  'admin.subscriptions.assignChoiceTitle': '选择分配方式',
  'admin.subscriptions.assignChoiceDesc': '请选择延长时长或叠加额度。',
  'admin.subscriptions.assignChoiceGroup': '分组',
  'admin.subscriptions.assignChoiceCurrentExpires': '当前到期',
  'admin.subscriptions.assignChoiceValidityDays': '本次有效期（天）',
  'admin.subscriptions.assignChoiceCurrentMultiplier': '当前额度',
  'admin.subscriptions.assignChoiceCancel': '取消',
  'admin.subscriptions.assignChoiceExtend': '延长有效期',
  'admin.subscriptions.assignChoiceStack': '叠加额度（从现在开始）',
  'admin.subscriptions.failedToAssign': '分配订阅失败',
  'admin.subscriptions.subscriptionAssigned': '订阅分配成功',
  'admin.subscriptions.pleaseSelectUser': '请选择用户',
  'admin.subscriptions.pleaseSelectGroup': '请选择订阅分组',
  'admin.subscriptions.validityDaysRequired': '请输入有效期天数',
  'admin.subscriptions.form.user': '用户',
  'admin.subscriptions.form.group': '订阅分组',
  'admin.subscriptions.form.validityDays': '有效期（天）',
  'admin.subscriptions.selectGroup': '选择订阅分组',
  'admin.subscriptions.groupHint': '仅显示订阅计费类型的分组',
  'admin.subscriptions.validityHint': '订阅的有效天数',
  'admin.subscriptions.noSubscriptionsYet': '暂无订阅',
  'admin.subscriptions.assignFirstSubscription': '分配一个订阅以开始使用。',
  'admin.subscriptions.columns.user': '用户',
  'admin.subscriptions.columns.group': '分组',
  'admin.subscriptions.columns.usage': '用量',
  'admin.subscriptions.columns.expires': '到期时间',
  'admin.subscriptions.columns.status': '状态',
  'admin.subscriptions.columns.actions': '操作',
  'admin.subscriptions.daily': '每日',
  'admin.subscriptions.weekly': '每周',
  'admin.subscriptions.monthly': '每月',
  'admin.subscriptions.allStatus': '全部状态',
  'admin.subscriptions.allGroups': '全部分组',
  'admin.subscriptions.allPlatforms': '全部平台',
  'admin.subscriptions.status.active': '生效中',
  'admin.subscriptions.status.expired': '已过期',
  'admin.subscriptions.status.revoked': '已撤销',
  'admin.subscriptions.guide.showGuide': '使用指南',
  'admin.subscriptions.unlimited': '无限制'
}

const translate = (key: string, params?: Record<string, unknown>) => {
  const template = messages[key] ?? key
  if (!params) {
    return template
  }
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template
  )
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      assign: assignSubscription
    },
    groups: {
      getAll: listGroups
    },
    usage: {
      searchUsers
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate
    })
  }
})

const TablePageLayoutStub = defineComponent({
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
})

const DataTableStub = defineComponent({
  props: {
    data: {
      type: Array,
      default: () => []
    }
  },
  template: `
    <div>
      <div v-if="data.length === 0">
        <slot name="empty" />
      </div>
      <div v-for="row in data" :key="row.id" data-test="subscription-row">
        <div data-test="group-cell">
          <slot name="cell-group" :row="row" />
        </div>
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
})

const BaseDialogStub = defineComponent({
  props: {
    show: {
      type: Boolean,
      default: false
    },
    title: {
      type: String,
      default: ''
    }
  },
  template: `
    <div v-if="show">
      <h2>{{ title }}</h2>
      <slot />
      <slot name="footer" />
    </div>
  `
})

describe('admin SubscriptionsView', () => {
  beforeEach(() => {
    listSubscriptions.mockReset()
    assignSubscription.mockReset()
    listGroups.mockReset()
    searchUsers.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listSubscriptions.mockResolvedValue({
      items: [
        {
          id: 11,
          user_id: 7,
          group_id: 3,
          status: 'active',
          quota_multiplier: 2,
          daily_usage_usd: 4,
          weekly_usage_usd: 0,
          monthly_usage_usd: 0,
          daily_window_start: null,
          weekly_window_start: null,
          monthly_window_start: null,
          created_at: '2026-03-26T00:00:00Z',
          updated_at: '2026-03-26T00:00:00Z',
          expires_at: '2026-04-26T00:00:00Z',
          user: {
            id: 7,
            email: 'user@example.com',
            username: 'demo'
          },
          group: {
            id: 3,
            name: 'Pro',
            description: null,
            platform: 'openai',
            rate_multiplier: 1,
            is_exclusive: false,
            status: 'active',
            subscription_type: 'subscription',
            daily_limit_usd: 10,
            weekly_limit_usd: null,
            monthly_limit_usd: null,
            image_price_1k: null,
            image_price_2k: null,
            image_price_4k: null,
            sora_image_price_360: null,
            sora_image_price_540: null,
            sora_video_price_per_request: null,
            sora_video_price_per_request_hd: null,
            sora_storage_quota_bytes: 0,
            claude_code_only: false,
            fallback_group_id: null,
            fallback_group_id_on_invalid_request: null,
            created_at: '2026-03-26T00:00:00Z',
            updated_at: '2026-03-26T00:00:00Z'
          }
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listGroups.mockResolvedValue([
      {
        id: 3,
        name: 'Pro',
        description: null,
        platform: 'openai',
        rate_multiplier: 1,
        is_exclusive: false,
        status: 'active',
        subscription_type: 'subscription',
        daily_limit_usd: 10,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
        image_price_1k: null,
        image_price_2k: null,
        image_price_4k: null,
        sora_image_price_360: null,
        sora_image_price_540: null,
        sora_video_price_per_request: null,
        sora_video_price_per_request_hd: null,
        sora_storage_quota_bytes: 0,
        claude_code_only: false,
        fallback_group_id: null,
        fallback_group_id_on_invalid_request: null,
        created_at: '2026-03-26T00:00:00Z',
        updated_at: '2026-03-26T00:00:00Z'
      }
    ])
    searchUsers.mockResolvedValue([])
  })

  const mountView = () =>
    mount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: TablePageLayoutStub,
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: BaseDialogStub,
          ConfirmDialog: true,
          EmptyState: true,
          Select: {
            props: ['modelValue', 'options', 'placeholder'],
            template: '<div><slot name="selected" /><slot name="option" :option="options?.[0]" :selected="false" /></div>'
          },
          GroupBadge: true,
          GroupOptionItem: true,
          Icon: true,
          'router-link': { template: '<a><slot /></a>' }
        }
      }
    })

  it('retries assignment with stack mode when backend requires a choice', async () => {
    assignSubscription
      .mockRejectedValueOnce({
        reason: 'SUBSCRIPTION_REDEEM_CHOICE_REQUIRED',
        metadata: {
          group_name: 'Pro',
          current_expires_at: '2026-04-26T00:00:00Z',
          validity_days: '30',
          current_quota_multiplier: '2'
        }
      })
      .mockResolvedValueOnce({
        id: 99
      })

    const wrapper = mountView()
    await flushPromises()

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.showAssignModal = true
    setupState.assignForm.user_id = 7
    setupState.assignForm.group_id = 3
    setupState.assignForm.validity_days = 30
    await nextTick()

    await wrapper.get('#assign-subscription-form').trigger('submit.prevent')
    await flushPromises()

    expect(assignSubscription).toHaveBeenCalledWith({
      user_id: 7,
      group_id: 3,
      validity_days: 30
    })
    expect(wrapper.find('[data-test="assign-choice-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-test="assign-choice-stack"]').trigger('click')
    await flushPromises()

    expect(assignSubscription).toHaveBeenNthCalledWith(2, {
      user_id: 7,
      group_id: 3,
      validity_days: 30,
      subscription_mode: 'stack'
    })
    expect(showSuccess).toHaveBeenCalledWith('订阅分配成功')
  })

  it('uses quota_multiplier when rendering usage limits', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('$20.00')
  })

  it('shows quota multiplier badge in group cell when subscription is stacked', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="group-cell"]').text()).toContain('×2')
  })
})
