<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <!-- Current Balance Card -->
      <div class="card overflow-hidden">
        <div class="bg-gradient-to-br from-primary-500 to-primary-600 px-6 py-8 text-center">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center rounded-2xl bg-white/20 backdrop-blur-sm"
          >
            <Icon name="creditCard" size="xl" class="text-white" />
          </div>
          <p class="text-sm font-medium text-primary-100">{{ t('redeem.currentBalance') }}</p>
          <p class="mt-2 text-4xl font-bold text-white">
            ${{ user?.balance?.toFixed(2) || '0.00' }}
          </p>
          <p class="mt-2 text-sm text-primary-100">
            {{ t('redeem.concurrency') }}: {{ user?.concurrency || 0 }} {{ t('redeem.requests') }}
          </p>
        </div>
      </div>

      <!-- Redeem Form -->
      <div class="card">
        <div class="p-6">
          <form @submit.prevent="handleRedeem" class="space-y-5">
            <div>
              <label for="code" class="input-label">
                {{ t('redeem.redeemCodeLabel') }}
              </label>
              <div class="relative mt-1">
                <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-4">
                  <Icon name="gift" size="md" class="text-gray-400 dark:text-dark-500" />
                </div>
                <input
                  id="code"
                  v-model="redeemCode"
                  type="text"
                  required
                  :placeholder="t('redeem.redeemCodePlaceholder')"
                  :disabled="submitting"
                  class="input py-3 pl-12 text-lg"
                />
              </div>
              <p class="input-hint">
                {{ t('redeem.redeemCodeHint') }}
              </p>
            </div>

            <button
              type="submit"
              :disabled="!redeemCode || submitting"
              class="btn btn-primary w-full py-3"
            >
              <svg
                v-if="submitting"
                class="-ml-1 mr-2 h-5 w-5 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
              <Icon v-else name="checkCircle" size="md" class="mr-2" />
              {{ submitting ? t('redeem.redeeming') : t('redeem.redeemButton') }}
            </button>
          </form>
        </div>
      </div>

      <!-- Redeem Mode Guide -->
      <div
        class="card border-primary-100 bg-gradient-to-br from-primary-50/80 to-blue-50/60 dark:border-primary-900/40 dark:from-primary-900/20 dark:to-dark-800"
      >
        <div class="space-y-4 p-6">
          <div class="space-y-1">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('redeem.modeGuideTitle') }}
            </h2>
            <p class="text-sm text-gray-600 dark:text-dark-300">
              {{ t('redeem.modeGuideDesc') }}
            </p>
          </div>

          <div class="inline-flex rounded-xl bg-white p-1 shadow-sm dark:bg-dark-800">
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
              :class="
                modeGuideSelection === 'stack'
                  ? 'bg-primary-500 text-white'
                  : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'
              "
              @click="modeGuideSelection = 'stack'"
            >
              {{ t('redeem.modeGuideStackTab') }}
            </button>
            <button
              type="button"
              class="rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
              :class="
                modeGuideSelection === 'extend'
                  ? 'bg-primary-500 text-white'
                  : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'
              "
              @click="modeGuideSelection = 'extend'"
            >
              {{ t('redeem.modeGuideExtendTab') }}
            </button>
          </div>

          <div v-if="modeGuideSelection === 'stack'" class="w-full">
            <div class="inline-flex w-fit flex-nowrap rounded-xl bg-white p-1 shadow-sm dark:bg-dark-800">
              <button
                type="button"
                class="whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
                :class="
                  modeGuideStackScenario === 'case1'
                    ? 'bg-primary-500 text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'
                "
                @click="modeGuideStackScenario = 'case1'"
              >
                {{ t('redeem.modeGuideStackCase1Tab') }}
              </button>
              <button
                type="button"
                class="whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
                :class="
                  modeGuideStackScenario === 'case2'
                    ? 'bg-primary-500 text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'
                "
                @click="modeGuideStackScenario = 'case2'"
              >
                {{ t('redeem.modeGuideStackCase2Tab') }}
              </button>
            </div>
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800">
              <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                {{ t('redeem.modeGuideBeforeCard') }}
              </p>
              <div class="mt-2 h-3 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="flex h-full w-full">
                  <div
                    v-for="(segment, idx) in modeGuideBeforeSegments"
                    :key="`before-${idx}`"
                    :class="[
                      getModeGuideSegmentClass(segment.tone),
                      'h-full border-r border-white/80 last:border-r-0 dark:border-dark-900/80'
                    ]"
                    :style="{ width: `${segment.widthPct ?? 100 / modeGuideBeforeSegments.length}%` }"
                  ></div>
                </div>
              </div>
              <div class="mt-2 space-y-1">
                <div
                  v-for="(segment, idx) in modeGuideBeforeSegments"
                  :key="`before-line-${idx}`"
                  class="text-xs"
                  :class="getModeGuideTextClass(segment.tone)"
                >
                  <div
                    v-if="modeGuideSelection === 'stack' && segment.startLabel && segment.endLabel"
                    class="flex items-start gap-2"
                  >
                    <span
                      :class="[getModeGuideSegmentClass(segment.tone), 'mt-1 h-2.5 w-2.5 rounded-full']"
                    ></span>
                    <span class="leading-relaxed">
                      {{
                        t('redeem.modeGuideSegmentLine', {
                          start: segment.startLabel,
                          end: segment.endLabel,
                          quota: t('redeem.modeGuideQuota', { quota: segment.quota })
                        })
                      }}
                    </span>
                  </div>
                  <div v-else class="flex items-center justify-between">
                    <span>{{ segment.label }}</span>
                    <span class="font-medium text-gray-700 dark:text-gray-200">
                      {{ t('redeem.modeGuideQuota', { quota: segment.quota }) }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800">
              <p class="text-xs font-semibold text-gray-700 dark:text-gray-200">
                {{ t('redeem.modeGuideAfterCard') }}
              </p>
              <div class="mt-2 h-3 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="flex h-full w-full">
                  <div
                    v-for="(segment, idx) in modeGuideAfterSegments"
                    :key="`after-${idx}`"
                    :class="[
                      getModeGuideSegmentClass(segment.tone),
                      'h-full border-r border-white/80 last:border-r-0 dark:border-dark-900/80'
                    ]"
                    :style="{ width: `${segment.widthPct ?? 100 / modeGuideAfterSegments.length}%` }"
                  ></div>
                </div>
              </div>
              <div class="mt-2 space-y-1">
                <div
                  v-for="(segment, idx) in modeGuideAfterSegments"
                  :key="`after-line-${idx}`"
                  class="text-xs"
                  :class="getModeGuideTextClass(segment.tone)"
                >
                  <div
                    v-if="modeGuideSelection === 'stack' && segment.startLabel && segment.endLabel"
                    class="flex items-start gap-2"
                  >
                    <span
                      :class="[getModeGuideSegmentClass(segment.tone), 'mt-1 h-2.5 w-2.5 rounded-full']"
                    ></span>
                    <span class="leading-relaxed">
                      {{
                        t('redeem.modeGuideSegmentLine', {
                          start: segment.startLabel,
                          end: segment.endLabel,
                          quota: t('redeem.modeGuideQuota', { quota: segment.quota })
                        })
                      }}
                    </span>
                  </div>
                  <div v-else class="flex items-center justify-between">
                    <span>{{ segment.label }}</span>
                    <span class="font-medium text-gray-700 dark:text-gray-200">
                      {{ t('redeem.modeGuideQuota', { quota: segment.quota }) }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <p class="whitespace-pre-line text-xs font-medium text-primary-700 dark:text-primary-300">
            {{ modeGuideHint }}
          </p>
          <p
            v-if="modeGuideSelection === 'stack' && modeGuideStackScenario === 'case2'"
            class="text-xs font-medium text-red-600 dark:text-red-400"
          >
            {{ t('redeem.modeGuideStackPriorityNote') }}
          </p>
        </div>
      </div>

      <!-- Success Message -->
      <transition name="fade">
        <div
          v-if="redeemResult"
          class="card border-emerald-200 bg-emerald-50 dark:border-emerald-800/50 dark:bg-emerald-900/20"
        >
          <div class="p-6">
            <div class="flex items-start gap-4">
              <div
                class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-emerald-100 dark:bg-emerald-900/30"
              >
                <Icon name="checkCircle" size="md" class="text-emerald-600 dark:text-emerald-400" />
              </div>
              <div class="flex-1">
                <h3 class="text-sm font-semibold text-emerald-800 dark:text-emerald-300">
                  {{ t('redeem.redeemSuccess') }}
                </h3>
                <div class="mt-2 text-sm text-emerald-700 dark:text-emerald-400">
                  <p>{{ redeemResult.message }}</p>
                  <div class="mt-3 space-y-1">
                    <p v-if="redeemResult.type === 'balance'" class="font-medium">
                      {{ t('redeem.added') }}: ${{ redeemResult.value.toFixed(2) }}
                    </p>
                    <p v-else-if="redeemResult.type === 'concurrency'" class="font-medium">
                      {{ t('redeem.added') }}: {{ redeemResult.value }}
                      {{ t('redeem.concurrentRequests') }}
                    </p>
                    <p v-else-if="redeemResult.type === 'subscription'" class="font-medium">
                      {{ t('redeem.subscriptionAssigned') }}
                      <span v-if="redeemResult.group_name"> - {{ redeemResult.group_name }}</span>
                      <span v-if="redeemResult.validity_days">
                        ({{
                          t('redeem.subscriptionDays', { days: redeemResult.validity_days })
                        }})</span
                      >
                    </p>
                    <p v-if="redeemResult.new_balance !== undefined">
                      {{ t('redeem.newBalance') }}:
                      <span class="font-semibold">${{ redeemResult.new_balance.toFixed(2) }}</span>
                    </p>
                    <p v-if="redeemResult.new_concurrency !== undefined">
                      {{ t('redeem.newConcurrency') }}:
                      <span class="font-semibold"
                        >{{ redeemResult.new_concurrency }} {{ t('redeem.requests') }}</span
                      >
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </transition>

      <!-- Error Message -->
      <transition name="fade">
        <div
          v-if="errorMessage"
          class="card border-red-200 bg-red-50 dark:border-red-800/50 dark:bg-red-900/20"
        >
          <div class="p-6">
            <div class="flex items-start gap-4">
              <div
                class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-red-100 dark:bg-red-900/30"
              >
                <Icon
                  name="exclamationCircle"
                  size="md"
                  class="text-red-600 dark:text-red-400"
                />
              </div>
              <div class="flex-1">
                <h3 class="text-sm font-semibold text-red-800 dark:text-red-300">
                  {{ t('redeem.redeemFailed') }}
                </h3>
                <p class="mt-2 text-sm text-red-700 dark:text-red-400">
                  {{ errorMessage }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </transition>

      <BaseDialog
        :show="subscriptionChoiceOpen"
        :title="t('redeem.subscriptionChoiceTitle')"
        width="wide"
        :close-on-click-outside="false"
        :close-on-escape="!submitting"
        :show-close-button="!submitting"
        @close="closeSubscriptionChoice"
      >
        <div class="space-y-4" data-test="subscription-choice-dialog">
          <p class="text-sm text-gray-600 dark:text-dark-300">
            {{ t('redeem.subscriptionChoiceDesc') }}
          </p>

          <div class="grid gap-3 rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800 sm:grid-cols-2">
            <div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceGroup') }}
              </p>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">
                {{ subscriptionChoiceMeta.group_name || '-' }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceCurrentExpires') }}
              </p>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">
                {{
                  subscriptionChoiceMeta.current_expires_at
                    ? formatDateTime(subscriptionChoiceMeta.current_expires_at)
                    : '-'
                }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceValidityDays') }}
              </p>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">
                {{ validityDays }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceCurrentMultiplier') }}
              </p>
              <p class="mt-1 font-medium text-gray-900 dark:text-white">
                ×{{ baseQuotaMultiplier }}
              </p>
            </div>
          </div>

          <div class="space-y-2">
            <button
              type="button"
              data-test="subscription-mode-extend"
              class="w-full rounded-xl border px-4 py-3 text-left transition-colors"
              :class="
                selectedSubscriptionMode === 'extend'
                  ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-500/10'
                  : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-800'
              "
              @click="toggleSubscriptionMode('extend')"
            >
              <p class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('redeem.subscriptionChoiceExtend') }}
              </p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceOptionExtendDesc', { days: validityDays }) }}
              </p>
            </button>

            <button
              type="button"
              data-test="subscription-mode-stack"
              class="w-full rounded-xl border px-4 py-3 text-left transition-colors"
              :class="
                selectedSubscriptionMode === 'stack'
                  ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-500/10'
                  : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-800'
              "
              @click="toggleSubscriptionMode('stack')"
            >
              <p class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('redeem.subscriptionChoiceStack') }}
              </p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('redeem.subscriptionChoiceOptionStackDesc', { days: validityDays }) }}
              </p>
            </button>
          </div>

          <div data-test="subscription-choice-preview" class="relative grid gap-4 sm:grid-cols-2">
            <SubscriptionUsageCard
              :title="t('redeem.subscriptionChoiceBeforeCardTitle')"
              :subscription="choiceSubscription"
              :loading="choiceSubscriptionLoading"
              :emptyHint="t('redeem.subscriptionChoicePreviewNotFound')"
              :expiresLabel="t('redeem.subscriptionChoiceCurrentExpires')"
              :showUsageSections="false"
            >
              <template #extra>
                <div v-if="choiceBeforeTimelineWindows.length > 0" class="space-y-2">
                  <SubscriptionQuotaTimeline
                    v-for="timeline in choiceBeforeTimelineWindows"
                    :key="`before-${timeline.key}`"
                    :title="timeline.label"
                    :segments="timeline.segments"
                  />
                </div>
              </template>
            </SubscriptionUsageCard>

            <SubscriptionUsageCard
              :title="t('redeem.subscriptionChoiceAfterCardTitle')"
              :subscription="choiceSubscription"
              :loading="choiceSubscriptionLoading"
              :placeholderText="selectedSubscriptionMode ? '' : t('redeem.subscriptionChoiceAfterHint')"
              :emptyHint="t('redeem.subscriptionChoicePreviewNotFound')"
              :quotaMultiplierOverride="previewQuotaMultiplier"
              :expiresAtOverride="previewTotalExpiresAt"
              :expiresLabel="t('redeem.subscriptionChoiceTotalExpires')"
              :showUsageSections="false"
            >
              <template #extra>
                <div v-if="choiceAfterTimelineWindows.length > 0" class="space-y-2">
                  <SubscriptionQuotaTimeline
                    v-for="timeline in choiceAfterTimelineWindows"
                    :key="`after-${timeline.key}`"
                    :title="timeline.label"
                    :segments="timeline.segments"
                  />
                </div>
                <div
                  v-if="selectedSubscriptionMode === 'stack' && previewStackUntil"
                  class="flex items-center justify-between rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800"
                >
                  <span class="text-gray-500 dark:text-dark-400">
                    {{ t('redeem.subscriptionChoiceStackUntil') }}
                  </span>
                  <span class="font-medium text-gray-900 dark:text-white">
                    {{ formatDateTime(previewStackUntil) }}
                  </span>
                </div>
              </template>
            </SubscriptionUsageCard>
          </div>

          <p v-if="choiceSubscriptionLoadError" class="text-xs text-orange-600 dark:text-orange-400">
            {{ choiceSubscriptionLoadError }}
          </p>
        </div>

        <template #footer>
          <div class="flex justify-end gap-3">
            <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeSubscriptionChoice">
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              data-test="subscription-choice-confirm"
              :disabled="submitting || !selectedSubscriptionMode"
              @click="confirmSubscriptionChoice"
            >
              {{ t('common.confirm') }}
            </button>
          </div>
        </template>
      </BaseDialog>

      <!-- Information Card -->
      <div
        class="card border-primary-200 bg-primary-50 dark:border-primary-800/50 dark:bg-primary-900/20"
      >
        <div class="p-6">
          <div class="flex items-start gap-4">
            <div
              class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/30"
            >
              <Icon name="infoCircle" size="md" class="text-primary-600 dark:text-primary-400" />
            </div>
            <div class="flex-1">
              <h3 class="text-sm font-semibold text-primary-800 dark:text-primary-300">
                {{ t('redeem.aboutCodes') }}
              </h3>
              <ul
                class="mt-2 list-inside list-disc space-y-1 text-sm text-primary-700 dark:text-primary-400"
              >
                <li>{{ t('redeem.codeRule1') }}</li>
                <li>{{ t('redeem.codeRule2') }}</li>
                <li>
                  {{ t('redeem.codeRule3') }}
                  <span
                    v-if="contactInfo"
                    class="ml-1.5 inline-flex items-center rounded-md bg-primary-200/50 px-2 py-0.5 text-xs font-medium text-primary-800 dark:bg-primary-800/40 dark:text-primary-200"
                  >
                    {{ contactInfo }}
                  </span>
                </li>
                <li>{{ t('redeem.codeRule4') }}</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('redeem.recentActivity') }}
          </h2>
        </div>
        <div class="p-6">
          <!-- Loading State -->
          <div v-if="loadingHistory" class="flex items-center justify-center py-8">
            <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
          </div>

          <!-- History List -->
          <div v-else-if="history.length > 0" class="space-y-3">
            <div
              v-for="item in history"
              :key="item.id"
              class="flex items-center justify-between rounded-xl bg-gray-50 p-4 dark:bg-dark-800"
            >
              <div class="flex items-center gap-4">
                <div
                  :class="[
                    'flex h-10 w-10 items-center justify-center rounded-xl',
                    isBalanceType(item.type)
                      ? item.value >= 0
                        ? 'bg-emerald-100 dark:bg-emerald-900/30'
                        : 'bg-red-100 dark:bg-red-900/30'
                      : isSubscriptionType(item.type)
                        ? 'bg-purple-100 dark:bg-purple-900/30'
                        : item.value >= 0
                          ? 'bg-blue-100 dark:bg-blue-900/30'
                          : 'bg-orange-100 dark:bg-orange-900/30'
                  ]"
                >
                  <!-- 余额类型图标 -->
                  <Icon
                    v-if="isBalanceType(item.type)"
                    name="dollar"
                    size="md"
                    :class="
                      item.value >= 0
                        ? 'text-emerald-600 dark:text-emerald-400'
                        : 'text-red-600 dark:text-red-400'
                    "
                  />
                  <!-- 订阅类型图标 -->
                  <Icon
                    v-else-if="isSubscriptionType(item.type)"
                    name="badge"
                    size="md"
                    class="text-purple-600 dark:text-purple-400"
                  />
                  <!-- 并发类型图标 -->
                  <Icon
                    v-else
                    name="bolt"
                    size="md"
                    :class="
                      item.value >= 0
                        ? 'text-blue-600 dark:text-blue-400'
                        : 'text-orange-600 dark:text-orange-400'
                    "
                  />
                </div>
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ getHistoryItemTitle(item) }}
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{ formatDateTime(item.used_at) }}
                  </p>
                </div>
              </div>
              <div class="text-right">
                <p
                  :class="[
                    'text-sm font-semibold',
                    isBalanceType(item.type)
                      ? item.value >= 0
                        ? 'text-emerald-600 dark:text-emerald-400'
                        : 'text-red-600 dark:text-red-400'
                      : isSubscriptionType(item.type)
                        ? 'text-purple-600 dark:text-purple-400'
                        : item.value >= 0
                          ? 'text-blue-600 dark:text-blue-400'
                          : 'text-orange-600 dark:text-orange-400'
                  ]"
                >
                  {{ formatHistoryValue(item) }}
                </p>
                <p
                  v-if="!isAdminAdjustment(item.type)"
                  class="font-mono text-xs text-gray-400 dark:text-dark-500"
                >
                  {{ item.code.slice(0, 8) }}...
                </p>
                <p v-else class="text-xs text-gray-400 dark:text-dark-500">
                  {{ t('redeem.adminAdjustment') }}
                </p>
                <!-- Display notes for admin adjustments -->
                <p
                  v-if="item.notes"
                  class="mt-1 text-xs text-gray-500 dark:text-dark-400 italic max-w-[200px] truncate"
                  :title="item.notes"
                >
                  {{ item.notes }}
                </p>
              </div>
            </div>
          </div>

          <!-- Empty State -->
          <div v-else class="empty-state py-8">
            <div
              class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800"
            >
              <Icon name="clock" size="xl" class="text-gray-400 dark:text-dark-500" />
            </div>
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('redeem.historyWillAppear') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { redeemAPI, authAPI, type RedeemHistoryItem } from '@/api'
import subscriptionsAPI from '@/api/subscriptions'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SubscriptionUsageCard from '@/components/common/SubscriptionUsageCard.vue'
import SubscriptionQuotaTimeline from '@/components/common/SubscriptionQuotaTimeline.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime, formatCurrency } from '@/utils/format'
import type {
  UserSubscription,
  UserSubscriptionGrantUsageResponse,
  AdminSubscriptionGrantUsage
} from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)

const redeemCode = ref('')
const pendingRedeemCode = ref('')
const submitting = ref(false)
const redeemResult = ref<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
  group_name?: string
  validity_days?: number
} | null>(null)
const errorMessage = ref('')
const subscriptionChoiceOpen = ref(false)
const subscriptionChoiceMeta = ref<Record<string, string>>({})
const selectedSubscriptionMode = ref<'extend' | 'stack' | ''>('')

type ModeGuideSelection = 'extend' | 'stack'
type ModeGuideStackScenario = 'case1' | 'case2'
type ModeGuideTone = 'base' | 'added' | 'boost' | 'priority'

interface ModeGuideSegment {
  label: string
  quota: number
  tone: ModeGuideTone
  startLabel?: string
  endLabel?: string
  widthPct?: number
}

const modeGuideSelection = ref<ModeGuideSelection>('stack')
const modeGuideStackScenario = ref<ModeGuideStackScenario>('case1')
const MODE_GUIDE_BASE_QUOTA = 60
const MODE_GUIDE_STACK_START = new Date(2026, 1, 24, 15, 0, 0)
const MODE_GUIDE_STACK_CASE1_BEFORE_START = new Date(2026, 1, 23, 18, 10, 0)
const MODE_GUIDE_STACK_CASE2_BEFORE_START = new Date(2026, 1, 23, 18, 10, 0)
const MODE_GUIDE_STACK_CASE1_OVERLAY_END = new Date(2026, 1, 25, 15, 0, 0)
const MODE_GUIDE_STACK_CASE1_BASE_END = new Date(2026, 2, 1, 18, 10, 0)
const MODE_GUIDE_STACK_CASE2_BASE_END = new Date(2026, 1, 24, 18, 10, 0)
const MODE_GUIDE_STACK_CASE2_OVERLAY_END = new Date(2026, 1, 25, 15, 0, 0)

const addDays = (base: Date, days: number): Date => {
  const d = new Date(base)
  d.setDate(d.getDate() + days)
  return d
}

const buildModeGuideBaseSegments = (): ModeGuideSegment[] => [
  { label: t('redeem.modeGuideDay', { day: 1 }), quota: MODE_GUIDE_BASE_QUOTA, tone: 'base' },
  { label: t('redeem.modeGuideDay', { day: 2 }), quota: MODE_GUIDE_BASE_QUOTA, tone: 'base' },
  { label: t('redeem.modeGuideDay', { day: 3 }), quota: MODE_GUIDE_BASE_QUOTA, tone: 'base' }
]

const formatGuideTime = (date: Date, withYear: boolean): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return withYear ? `${year}-${month}-${day} ${hour}:${minute}` : `${month}-${day} ${hour}:${minute}`
}

const getModeGuideStackConfig = (): {
  beforeStart: Date
  firstStart: Date
  beforeEnd: Date
  firstEnd: Date
  secondEnd: Date
  firstTone: ModeGuideTone
  withYear: boolean
} => {
  if (modeGuideStackScenario.value === 'case2') {
    return {
      beforeStart: MODE_GUIDE_STACK_CASE2_BEFORE_START,
      firstStart: MODE_GUIDE_STACK_START,
      beforeEnd: MODE_GUIDE_STACK_CASE2_BASE_END,
      firstEnd: MODE_GUIDE_STACK_CASE2_BASE_END,
      secondEnd: MODE_GUIDE_STACK_CASE2_OVERLAY_END,
      firstTone: 'priority',
      withYear: false
    }
  }

  return {
    beforeStart: MODE_GUIDE_STACK_CASE1_BEFORE_START,
    firstStart: MODE_GUIDE_STACK_START,
    beforeEnd: MODE_GUIDE_STACK_CASE1_BASE_END,
    firstEnd: MODE_GUIDE_STACK_CASE1_OVERLAY_END,
    secondEnd: MODE_GUIDE_STACK_CASE1_BASE_END,
    firstTone: 'boost',
    withYear: false
  }
}

const buildModeGuideStackBeforeSegments = (): ModeGuideSegment[] => {
  const config = getModeGuideStackConfig()
  return [
    {
      label: t('redeem.modeGuideBeforeCard'),
      quota: MODE_GUIDE_BASE_QUOTA,
      tone: 'base',
      startLabel: formatGuideTime(config.beforeStart, config.withYear),
      endLabel: formatGuideTime(config.beforeEnd, config.withYear),
      widthPct: 100
    }
  ]
}

const buildModeGuideStackAfterSegments = (): ModeGuideSegment[] => {
  const config = getModeGuideStackConfig()
  const firstMs = config.firstEnd.getTime() - config.firstStart.getTime()
  const secondMs = config.secondEnd.getTime() - config.firstEnd.getTime()
  const totalMs = Math.max(1, firstMs + secondMs)
  const firstWidth = (firstMs / totalMs) * 100

  return [
    {
      label: t('redeem.modeGuideDay', { day: 1 }),
      quota: MODE_GUIDE_BASE_QUOTA * 2,
      tone: config.firstTone,
      startLabel: formatGuideTime(config.firstStart, config.withYear),
      endLabel: formatGuideTime(config.firstEnd, config.withYear),
      widthPct: firstWidth
    },
    {
      label: t('redeem.modeGuideDay', { day: 2 }),
      quota: MODE_GUIDE_BASE_QUOTA,
      tone: 'base',
      startLabel: formatGuideTime(config.firstEnd, config.withYear),
      endLabel: formatGuideTime(config.secondEnd, config.withYear),
      widthPct: 100 - firstWidth
    }
  ]
}

const modeGuideBeforeSegments = computed<ModeGuideSegment[]>(() =>
  modeGuideSelection.value === 'stack' ? buildModeGuideStackBeforeSegments() : buildModeGuideBaseSegments()
)

const modeGuideAfterSegments = computed<ModeGuideSegment[]>(() => {
  if (modeGuideSelection.value === 'extend') {
    return [
      ...buildModeGuideBaseSegments(),
      { label: t('redeem.modeGuideAddedDay'), quota: MODE_GUIDE_BASE_QUOTA, tone: 'added' }
    ]
  }
  return buildModeGuideStackAfterSegments()
})

const modeGuideHint = computed(() =>
  modeGuideSelection.value === 'extend'
    ? t('redeem.modeGuideExtendHint')
    : modeGuideStackScenario.value === 'case2'
      ? t('redeem.modeGuideStackHintCase2')
      : t('redeem.modeGuideStackHint')
)

const getModeGuideSegmentClass = (tone: ModeGuideTone): string => {
  if (tone === 'added') return 'bg-emerald-500'
  if (tone === 'boost') return 'bg-amber-500'
  if (tone === 'priority') return 'bg-red-500'
  return 'bg-sky-500'
}

const getModeGuideTextClass = (_tone: ModeGuideTone): string => 'text-gray-500 dark:text-dark-400'

// History data
const history = ref<RedeemHistoryItem[]>([])
const loadingHistory = ref(false)
const contactInfo = ref('')

// Helper functions for history display
const isBalanceType = (type: string) => {
  return type === 'balance' || type === 'admin_balance'
}

const isSubscriptionType = (type: string) => {
  return type === 'subscription'
}

const isAdminAdjustment = (type: string) => {
  return type === 'admin_balance' || type === 'admin_concurrency'
}

const choiceSubscription = ref<UserSubscription | null>(null)
const choiceSubscriptionGrants = ref<UserSubscriptionGrantUsageResponse | null>(null)
const choiceSubscriptionLoading = ref(false)
const choiceSubscriptionLoadError = ref('')
const choiceDialogOpenedAt = ref<Date | null>(null)

const validityDays = computed(() => {
  const raw = subscriptionChoiceMeta.value.validity_days
  const n = parseInt(String(raw ?? ''), 10)
  if (!Number.isFinite(n) || n <= 0) return 30
  return n
})

const baseQuotaMultiplier = computed(() => {
  const fromSub = choiceSubscription.value?.quota_multiplier
  if (fromSub !== null && fromSub !== undefined) return Math.max(1, fromSub)
  const raw = subscriptionChoiceMeta.value.current_quota_multiplier
  const n = parseInt(String(raw ?? ''), 10)
  if (!Number.isFinite(n) || n <= 0) return 1
  return n
})

const previewQuotaMultiplier = computed(() => {
  if (!selectedSubscriptionMode.value) return null
  if (selectedSubscriptionMode.value === 'extend') return baseQuotaMultiplier.value
  return baseQuotaMultiplier.value + 1
})

const previewStackUntil = computed(() => {
  if (selectedSubscriptionMode.value !== 'stack') return null
  const now = choiceDialogOpenedAt.value || new Date()
  return addDays(now, validityDays.value)
})

const previewBaseExpiresDate = computed(() => {
  const base = choiceSubscription.value?.expires_at || subscriptionChoiceMeta.value.current_expires_at
  if (!base) return null
  const date = new Date(base)
  if (Number.isNaN(date.getTime())) return null
  return date
})

const previewTimelineNow = computed(() => choiceDialogOpenedAt.value || new Date())

interface PreviewTimelineSegmentView {
  key: string
  startLabel: string
  endLabel: string
  quotaText: string
  widthPct: number
  colorClass: string
}

interface GrantTimelineRawSegment {
  startMs: number
  endMs: number
  quota: number
}

interface ChoiceTimelineWindowView {
  key: 'daily' | 'weekly' | 'monthly'
  label: string
  segments: PreviewTimelineSegmentView[]
}

interface GrantTimelineInterval {
  startMs: number
  endMs: number
}

const CHOICE_TIMELINE_COLORS = [
  'bg-blue-600',
  'bg-orange-500',
  'bg-violet-600',
  'bg-lime-500',
  'bg-rose-600',
  'bg-cyan-500',
  'bg-amber-600',
  'bg-emerald-600',
  'bg-fuchsia-600',
  'bg-sky-500'
]

const normalizeQuota = (quota: number): string => quota.toFixed(6)

const buildQuotaColorMap = (segments: GrantTimelineRawSegment[]): Map<string, string> => {
  const unique = Array.from(new Set(segments.map((item) => normalizeQuota(item.quota))))
  unique.sort((a, b) => Number(a) - Number(b))

  const colorMap = new Map<string, string>()
  unique.forEach((quota, index) => {
    colorMap.set(quota, CHOICE_TIMELINE_COLORS[index % CHOICE_TIMELINE_COLORS.length])
  })
  return colorMap
}

const buildGrantTimelineSegments = (
  grants: AdminSubscriptionGrantUsage[],
  baseLimit: number,
  nowMs: number,
  extraIntervals: GrantTimelineInterval[] = []
): GrantTimelineRawSegment[] => {
  if (!Number.isFinite(baseLimit) || baseLimit <= 0) return []

  const intervals: GrantTimelineInterval[] = []
  const boundaries = new Set<number>([nowMs])

  for (const grant of grants) {
    const startMs = new Date(grant.starts_at).getTime()
    const endMs = new Date(grant.expires_at).getTime()
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= nowMs) continue

    intervals.push({ startMs, endMs })
    if (startMs > nowMs) boundaries.add(startMs)
    boundaries.add(endMs)
  }

  for (const extra of extraIntervals) {
    if (!Number.isFinite(extra.startMs) || !Number.isFinite(extra.endMs) || extra.endMs <= nowMs) continue
    intervals.push({ startMs: extra.startMs, endMs: extra.endMs })
    if (extra.startMs > nowMs) boundaries.add(extra.startMs)
    boundaries.add(extra.endMs)
  }

  const points = Array.from(boundaries).sort((a, b) => a - b)
  if (points.length < 2) return []

  const raw: GrantTimelineRawSegment[] = []
  for (let i = 0; i < points.length - 1; i++) {
    const segmentStart = points[i]
    const segmentEnd = points[i + 1]
    if (segmentEnd <= segmentStart) continue

    let activeCount = 0
    for (const interval of intervals) {
      if (interval.startMs <= segmentStart && interval.endMs > segmentStart) {
        activeCount += 1
      }
    }

    if (activeCount <= 0) continue
    raw.push({
      startMs: segmentStart,
      endMs: segmentEnd,
      quota: activeCount * baseLimit
    })
  }

  if (raw.length === 0) return []

  const merged: GrantTimelineRawSegment[] = []
  for (const segment of raw) {
    const last = merged[merged.length - 1]
    if (
      last &&
      normalizeQuota(last.quota) === normalizeQuota(segment.quota) &&
      last.endMs === segment.startMs
    ) {
      last.endMs = segment.endMs
    } else {
      merged.push({ ...segment })
    }
  }

  return merged
}

const formatTimelineDateTime = (ms: number): string => {
  return formatDateTime(new Date(ms), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}

const buildTimelineWindowViews = (
  windows: Array<{ key: ChoiceTimelineWindowView['key']; label: string; segments: GrantTimelineRawSegment[] }>,
  nowMs: number
): ChoiceTimelineWindowView[] => {
  if (windows.length === 0) return []
  const colorMap = buildQuotaColorMap(windows.flatMap((item) => item.segments))

  return windows.map((window) => {
    const timelineStart = window.segments[0].startMs
    const timelineEnd = window.segments[window.segments.length - 1].endMs
    const totalMs = Math.max(1, timelineEnd - timelineStart)

    const segments: PreviewTimelineSegmentView[] = window.segments.map((segment, index) => ({
      key: `${window.key}-${segment.startMs}-${segment.endMs}-${index}`,
      startLabel:
        segment.startMs === nowMs ? t('userSubscriptions.timeline.now') : formatTimelineDateTime(segment.startMs),
      endLabel: formatTimelineDateTime(segment.endMs),
      quotaText: formatCurrency(segment.quota),
      widthPct: Math.max(2, ((segment.endMs - segment.startMs) / totalMs) * 100),
      colorClass: colorMap.get(normalizeQuota(segment.quota)) || CHOICE_TIMELINE_COLORS[0]
    }))

    return {
      key: window.key,
      label: window.label,
      segments
    }
  })
}

const choiceBeforeTimelineWindows = computed<ChoiceTimelineWindowView[]>(() => {
  const sub = choiceSubscription.value
  if (!sub) return []

  const nowMs = previewTimelineNow.value.getTime()
  const grants = choiceSubscriptionGrants.value?.grants || []
  const windowsMeta: Array<{
    key: ChoiceTimelineWindowView['key']
    label: string
    baseLimit: number | null
    fallbackEffectiveLimit: number | null
  }> = [
    {
      key: 'daily',
      label: t('userSubscriptions.daily'),
      baseLimit: sub.group?.daily_limit_usd ?? null,
      fallbackEffectiveLimit: sub.group?.daily_limit_usd ? sub.group.daily_limit_usd * baseQuotaMultiplier.value : null
    },
    {
      key: 'weekly',
      label: t('userSubscriptions.weekly'),
      baseLimit: sub.group?.weekly_limit_usd ?? null,
      fallbackEffectiveLimit: sub.group?.weekly_limit_usd ? sub.group.weekly_limit_usd * baseQuotaMultiplier.value : null
    },
    {
      key: 'monthly',
      label: t('userSubscriptions.monthly'),
      baseLimit: sub.group?.monthly_limit_usd ?? null,
      fallbackEffectiveLimit: sub.group?.monthly_limit_usd ? sub.group.monthly_limit_usd * baseQuotaMultiplier.value : null
    }
  ]

  const rawWindows: Array<{ key: ChoiceTimelineWindowView['key']; label: string; segments: GrantTimelineRawSegment[] }> = []
  for (const item of windowsMeta) {
    if (!item.baseLimit || item.baseLimit <= 0) continue

    let segments = buildGrantTimelineSegments(grants, item.baseLimit, nowMs)
    if (segments.length === 0 && grants.length === 0 && item.fallbackEffectiveLimit) {
      const baseExpires = previewBaseExpiresDate.value?.getTime() || 0
      if (baseExpires > nowMs) {
        segments = [
          {
            startMs: nowMs,
            endMs: baseExpires,
            quota: item.fallbackEffectiveLimit
          }
        ]
      }
    }

    if (segments.length > 0) {
      rawWindows.push({
        key: item.key,
        label: item.label,
        segments
      })
    }
  }

  return buildTimelineWindowViews(rawWindows, nowMs)
})

const choiceAfterTimelineWindows = computed<ChoiceTimelineWindowView[]>(() => {
  if (!selectedSubscriptionMode.value) return []

  const sub = choiceSubscription.value
  if (!sub) return []

  const nowMs = previewTimelineNow.value.getTime()
  const grants = choiceSubscriptionGrants.value?.grants || []
  const baseEndMs = previewBaseExpiresDate.value?.getTime() ?? nowMs
  const stackEndMs = previewStackUntil.value?.getTime() || 0
  const afterEndMs = previewTotalExpiresAt.value ? new Date(previewTotalExpiresAt.value).getTime() : 0

  const windowsMeta: Array<{
    key: ChoiceTimelineWindowView['key']
    label: string
    baseLimit: number | null
    fallbackBaseLimit: number | null
    fallbackBoostLimit: number | null
  }> = [
    {
      key: 'daily',
      label: t('userSubscriptions.daily'),
      baseLimit: sub.group?.daily_limit_usd ?? null,
      fallbackBaseLimit: sub.group?.daily_limit_usd ? sub.group.daily_limit_usd * baseQuotaMultiplier.value : null,
      fallbackBoostLimit: sub.group?.daily_limit_usd
        ? sub.group.daily_limit_usd * (baseQuotaMultiplier.value + 1)
        : null
    },
    {
      key: 'weekly',
      label: t('userSubscriptions.weekly'),
      baseLimit: sub.group?.weekly_limit_usd ?? null,
      fallbackBaseLimit: sub.group?.weekly_limit_usd ? sub.group.weekly_limit_usd * baseQuotaMultiplier.value : null,
      fallbackBoostLimit: sub.group?.weekly_limit_usd
        ? sub.group.weekly_limit_usd * (baseQuotaMultiplier.value + 1)
        : null
    },
    {
      key: 'monthly',
      label: t('userSubscriptions.monthly'),
      baseLimit: sub.group?.monthly_limit_usd ?? null,
      fallbackBaseLimit: sub.group?.monthly_limit_usd ? sub.group.monthly_limit_usd * baseQuotaMultiplier.value : null,
      fallbackBoostLimit: sub.group?.monthly_limit_usd
        ? sub.group.monthly_limit_usd * (baseQuotaMultiplier.value + 1)
        : null
    }
  ]

  const rawWindows: Array<{ key: ChoiceTimelineWindowView['key']; label: string; segments: GrantTimelineRawSegment[] }> = []
  for (const item of windowsMeta) {
    if (!item.baseLimit || item.baseLimit <= 0) continue

    const extraIntervals: GrantTimelineInterval[] = []
    if (selectedSubscriptionMode.value === 'extend') {
      const tailStart = Math.max(nowMs, baseEndMs)
      if (afterEndMs > tailStart) {
        extraIntervals.push({ startMs: tailStart, endMs: afterEndMs })
      }
    } else if (selectedSubscriptionMode.value === 'stack' && stackEndMs > nowMs) {
      extraIntervals.push({ startMs: nowMs, endMs: stackEndMs })
    }

    let segments = buildGrantTimelineSegments(grants, item.baseLimit, nowMs, extraIntervals)

    if (segments.length === 0 && grants.length === 0 && item.fallbackBaseLimit) {
      if (selectedSubscriptionMode.value === 'extend' && afterEndMs > nowMs) {
        segments = [
          {
            startMs: nowMs,
            endMs: afterEndMs,
            quota: item.fallbackBaseLimit
          }
        ]
      } else if (selectedSubscriptionMode.value === 'stack' && stackEndMs > nowMs) {
        const fallbackBoost = item.fallbackBoostLimit || item.fallbackBaseLimit
        const overlapEndMs = Math.min(baseEndMs, stackEndMs)
        const tailEndMs = Math.max(baseEndMs, stackEndMs)
        const fallbackSegments: GrantTimelineRawSegment[] = []

        if (baseEndMs > nowMs && overlapEndMs > nowMs) {
          fallbackSegments.push({
            startMs: nowMs,
            endMs: overlapEndMs,
            quota: fallbackBoost
          })
        }

        const tailStartMs = Math.max(nowMs, overlapEndMs)
        if (tailEndMs > tailStartMs) {
          fallbackSegments.push({
            startMs: tailStartMs,
            endMs: tailEndMs,
            quota: item.fallbackBaseLimit
          })
        }

        if (fallbackSegments.length === 0) {
          fallbackSegments.push({
            startMs: nowMs,
            endMs: stackEndMs,
            quota: item.fallbackBaseLimit
          })
        }

        segments = fallbackSegments
      }
    }

    if (segments.length > 0) {
      rawWindows.push({
        key: item.key,
        label: item.label,
        segments
      })
    }
  }

  return buildTimelineWindowViews(rawWindows, nowMs)
})

const previewTotalExpiresAt = computed(() => {
  if (!selectedSubscriptionMode.value) return null
  const base = choiceSubscription.value?.expires_at || subscriptionChoiceMeta.value.current_expires_at
  const baseDate = base ? new Date(base) : null
  const baseValid = !!baseDate && !Number.isNaN(baseDate.getTime())

  if (selectedSubscriptionMode.value === 'extend') {
    if (!baseValid) return null
    return addDays(baseDate as Date, validityDays.value).toISOString()
  }

  const stackEndMs = previewStackUntil.value?.getTime()
  const stackUntil = stackEndMs ? new Date(stackEndMs) : null
  if (!stackUntil) return baseValid ? (baseDate as Date).toISOString() : null
  if (!baseValid) return stackUntil.toISOString()
  return (stackUntil.getTime() > (baseDate as Date).getTime() ? stackUntil : (baseDate as Date)).toISOString()
})

const getSubscriptionModeLabel = (item: RedeemHistoryItem) => {
  if (item.subscription_mode === 'extend') {
    return t('redeem.subscriptionModeExtend')
  }
  if (item.subscription_mode === 'stack') {
    return t('redeem.subscriptionModeStack')
  }
  return ''
}

const getHistoryItemTitle = (item: RedeemHistoryItem) => {
  if (item.type === 'balance') {
    return t('redeem.balanceAddedRedeem')
  } else if (item.type === 'admin_balance') {
    return item.value >= 0 ? t('redeem.balanceAddedAdmin') : t('redeem.balanceDeductedAdmin')
  } else if (item.type === 'concurrency') {
    return t('redeem.concurrencyAddedRedeem')
  } else if (item.type === 'admin_concurrency') {
    return item.value >= 0 ? t('redeem.concurrencyAddedAdmin') : t('redeem.concurrencyReducedAdmin')
  } else if (item.type === 'subscription') {
    if (item.subscription_mode === 'extend') {
      return t('redeem.subscriptionExtended')
    }
    if (item.subscription_mode === 'stack') {
      return t('redeem.subscriptionStacked')
    }
    return t('redeem.subscriptionAssigned')
  }
  return t('common.unknown')
}

const formatHistoryValue = (item: RedeemHistoryItem) => {
  if (isBalanceType(item.type)) {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}$${item.value.toFixed(2)}`
  } else if (isSubscriptionType(item.type)) {
    // 订阅类型显示有效天数和分组名称
    const days = item.validity_days || Math.round(item.value)
    const groupName = item.group?.name || ''
    const base = groupName ? `${days}${t('redeem.days')} - ${groupName}` : `${days}${t('redeem.days')}`
    const modeLabel = getSubscriptionModeLabel(item)
    return modeLabel ? `${base} · ${modeLabel}` : base
  } else {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}${item.value} ${t('redeem.requests')}`
  }
}

const fetchHistory = async () => {
  loadingHistory.value = true
  try {
    history.value = await redeemAPI.getHistory()
  } catch (error) {
    console.error('Failed to fetch history:', error)
  } finally {
    loadingHistory.value = false
  }
}

const handleRedeem = async () => {
  if (!redeemCode.value.trim()) {
    appStore.showError(t('redeem.pleaseEnterCode'))
    return
  }

  pendingRedeemCode.value = redeemCode.value.trim()
  submitting.value = true
  errorMessage.value = ''
  redeemResult.value = null

  try {
    const result = await redeemAPI.redeem(pendingRedeemCode.value)

    redeemResult.value = result

    // Refresh user data to get updated balance/concurrency
    await authStore.refreshUser()

    // If subscription type, immediately refresh subscription status
    if (result.type === 'subscription') {
      try {
        await subscriptionStore.fetchActiveSubscriptions(true) // force refresh
      } catch (error) {
        console.error('Failed to refresh subscriptions after redeem:', error)
        appStore.showWarning(t('redeem.subscriptionRefreshFailed'))
      }
    }

    // Clear the input
    redeemCode.value = ''
    pendingRedeemCode.value = ''

    // Refresh history
    await fetchHistory()

    // Show success toast
    appStore.showSuccess(t('redeem.codeRedeemSuccess'))
  } catch (error: any) {
    if (error?.reason === 'SUBSCRIPTION_REDEEM_CHOICE_REQUIRED') {
      subscriptionChoiceMeta.value = error?.metadata || {}
      selectedSubscriptionMode.value = ''
      subscriptionChoiceOpen.value = true
      return
    }

    errorMessage.value = error?.message || t('redeem.failedToRedeem')
    appStore.showError(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

const toggleSubscriptionMode = (mode: 'extend' | 'stack') => {
  if (submitting.value) return
  selectedSubscriptionMode.value = selectedSubscriptionMode.value === mode ? '' : mode
}

const confirmSubscriptionChoice = () => {
  if (selectedSubscriptionMode.value === 'extend' || selectedSubscriptionMode.value === 'stack') {
    handleRedeemWithMode(selectedSubscriptionMode.value)
  }
}

const loadChoiceSubscription = async () => {
  choiceSubscriptionLoading.value = true
  choiceSubscriptionLoadError.value = ''
  choiceSubscription.value = null
  choiceSubscriptionGrants.value = null
  try {
    const groupID = parseInt(String(subscriptionChoiceMeta.value.group_id || ''), 10)
    if (!Number.isFinite(groupID) || groupID <= 0) {
      choiceSubscriptionLoadError.value = t('redeem.subscriptionChoicePreviewLoadFailed')
      return
    }

    const all = await subscriptionsAPI.getMySubscriptions()
    const sub = all.find((s) => s.group_id === groupID) || null
    if (!sub) {
      choiceSubscriptionLoadError.value = t('redeem.subscriptionChoicePreviewNotFound')
      return
    }

    choiceSubscription.value = sub

    try {
      choiceSubscriptionGrants.value = await subscriptionsAPI.getSubscriptionGrants(sub.id)
    } catch (error) {
      console.error('Failed to load subscription grants for redeem preview:', error)
      choiceSubscriptionGrants.value = null
    }
  } catch (error) {
    console.error('Failed to load subscription for redeem preview:', error)
    choiceSubscriptionLoadError.value = t('redeem.subscriptionChoicePreviewLoadFailed')
  } finally {
    choiceSubscriptionLoading.value = false
  }
}

const closeSubscriptionChoice = () => {
  if (submitting.value) {
    return
  }
  subscriptionChoiceOpen.value = false
  subscriptionChoiceMeta.value = {}
  selectedSubscriptionMode.value = ''
  pendingRedeemCode.value = ''
  choiceSubscription.value = null
  choiceSubscriptionGrants.value = null
  choiceSubscriptionLoadError.value = ''
  choiceSubscriptionLoading.value = false
  choiceDialogOpenedAt.value = null
}

const handleRedeemWithMode = async (mode: 'extend' | 'stack') => {
  if (!pendingRedeemCode.value) {
    return
  }

  submitting.value = true
  errorMessage.value = ''
  redeemResult.value = null

  try {
    const result = await redeemAPI.redeem(pendingRedeemCode.value, mode)
    subscriptionChoiceOpen.value = false
    subscriptionChoiceMeta.value = {}
    selectedSubscriptionMode.value = ''
    choiceSubscription.value = null
    choiceSubscriptionGrants.value = null
    choiceSubscriptionLoadError.value = ''
    choiceSubscriptionLoading.value = false
    choiceDialogOpenedAt.value = null

    redeemResult.value = result
    await authStore.refreshUser()

    if (result.type === 'subscription') {
      try {
        await subscriptionStore.fetchActiveSubscriptions(true)
      } catch (error) {
        console.error('Failed to refresh subscriptions after redeem:', error)
        appStore.showWarning(t('redeem.subscriptionRefreshFailed'))
      }
    }

    redeemCode.value = ''
    pendingRedeemCode.value = ''
    await fetchHistory()
    appStore.showSuccess(t('redeem.codeRedeemSuccess'))
  } catch (error: any) {
    errorMessage.value = error?.message || t('redeem.failedToRedeem')
    appStore.showError(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

watch(
  () => subscriptionChoiceOpen.value,
  (open) => {
    if (!open) return
    selectedSubscriptionMode.value = ''
    choiceDialogOpenedAt.value = new Date()
    loadChoiceSubscription()
  }
)

onMounted(async () => {
  fetchHistory()
  try {
    const settings = await authAPI.getPublicSettings()
    contactInfo.value = settings.contact_info || ''
  } catch (error) {
    console.error('Failed to load contact info:', error)
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
