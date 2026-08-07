<template>
  <div class="flex min-h-screen flex-col bg-[#f6f1e8] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="flex-1 pb-20 pt-24">
      <section class="relative overflow-hidden border-b border-black/5 dark:border-white/10">
        <div class="pointer-events-none absolute inset-0">
          <div class="absolute left-1/2 top-0 h-[420px] w-[420px] -translate-x-1/2 rounded-full bg-[#b89a7a]/20 blur-3xl dark:bg-primary-500/10"></div>
          <div class="absolute -top-24 right-[-40px] h-[280px] w-[280px] rounded-full bg-[#7b6857]/12 blur-3xl dark:bg-primary-500/10"></div>
          <div class="absolute -bottom-28 left-[-80px] h-[260px] w-[260px] rounded-full bg-[#dbc8b0]/35 blur-3xl dark:bg-sky-500/10"></div>
          <div class="absolute inset-0 opacity-[0.2] [background-image:linear-gradient(rgba(123,104,87,0.08)_1px,transparent_1px),linear-gradient(90deg,rgba(123,104,87,0.08)_1px,transparent_1px)] [background-size:96px_96px] dark:opacity-[0.05]"></div>
        </div>

        <div class="relative mx-auto max-w-6xl px-6 py-16 lg:py-20">
          <div class="grid gap-8 lg:grid-cols-[minmax(0,1.1fr)_360px] lg:items-start">
            <div class="max-w-3xl">
              <span class="inline-flex items-center rounded-full border border-[#7b6857]/15 bg-white/75 px-4 py-1.5 text-sm font-medium text-[#7b6857] shadow-sm backdrop-blur dark:border-white/10 dark:bg-white/5 dark:text-primary-200">
                {{ t('integrationDocs.hero.badge') }}
              </span>
              <h1 class="mt-6 max-w-2xl text-4xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-5xl lg:text-[3.6rem]">
                {{ t('integrationDocs.hero.title') }}
              </h1>
              <p class="mt-4 max-w-2xl text-base leading-relaxed text-gray-600 dark:text-dark-300 sm:text-lg">
                {{ t('integrationDocs.hero.subtitle') }}
              </p>

              <div class="mt-8 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <div
                  v-for="metric in heroMetrics"
                  :key="metric.label"
                  class="rounded-[28px] border border-[#7b6857]/10 bg-white/78 px-4 py-4 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40"
                >
                  <div class="text-2xl font-semibold tracking-tight text-[#5f4e40] dark:text-white">
                    {{ metric.value }}
                  </div>
                  <div class="mt-1 text-xs uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                    {{ metric.label }}
                  </div>
                </div>
              </div>

              <div class="mt-6 grid gap-4 md:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
                <div class="rounded-[28px] border border-[#7b6857]/10 bg-white/80 p-5 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
                  <div class="text-xs font-semibold uppercase tracking-[0.16em] text-[#7b6857] dark:text-primary-200">
                    {{ t('integrationDocs.hero.cards.baseUrl') }}
                  </div>
                  <div class="mt-3 break-all rounded-2xl bg-[#221b16] px-4 py-3 font-mono text-sm text-[#f7efe5]">
                    {{ baseUrl }}
                  </div>
                  <div class="mt-4 flex flex-wrap gap-2">
                    <span
                      v-for="path in standardPaths"
                      :key="`hero-path-${path}`"
                      class="rounded-full border border-[#7b6857]/12 bg-[#f4ede2] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200"
                    >
                      {{ path }}
                    </span>
                  </div>
                </div>

                <div class="rounded-[28px] border border-[#7b6857]/10 bg-[#2d241d] p-5 text-[#f4e8db] shadow-[0_20px_60px_rgba(53,41,33,0.18)] dark:border-white/10 dark:bg-dark-900/70">
                  <div class="text-xs font-semibold uppercase tracking-[0.16em] text-[#d4b896]">
                    {{ t('integrationDocs.hero.cards.auth') }}
                  </div>
                  <div class="mt-3 rounded-2xl border border-white/10 bg-white/5 px-4 py-3 font-mono text-sm text-white">
                    Authorization: Bearer {{ apiKeyPlaceholder }}
                  </div>
                  <div class="mt-4 text-xs uppercase tracking-[0.16em] text-[#d4b896]">
                    {{ t('integrationDocs.hero.cards.compatibility') }}
                  </div>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <span
                      v-for="item in compatibilityChips"
                      :key="item"
                      class="rounded-full border border-white/10 bg-white/8 px-3 py-1 text-xs font-medium text-[#f4e8db]"
                    >
                      {{ item }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <div class="rounded-[32px] border border-[#7b6857]/10 bg-white/82 p-5 shadow-[0_22px_60px_rgba(91,73,55,0.12)] backdrop-blur dark:border-white/10 dark:bg-dark-900/45">
              <div class="flex items-center justify-between">
                <div>
                  <div class="text-xs font-semibold uppercase tracking-[0.16em] text-[#7b6857] dark:text-primary-200">
                    {{ localizeText(text('协议矩阵', 'Protocol Matrix')) }}
                  </div>
                  <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
                    {{ localizeText(text('按协议直接落地', 'Ship by protocol')) }}
                  </div>
                </div>
                <div class="rounded-full border border-[#7b6857]/12 bg-[#f4ede2] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200">
                  {{ localizeText(text('4 组示例', '4 example sets')) }}
                </div>
              </div>

              <div class="mt-5 space-y-3">
                <a
                  v-for="card in heroProtocolCards"
                  :key="card.id"
                  :href="`#${card.id}`"
                  class="group block rounded-[24px] border border-[#7b6857]/10 bg-[#fcfaf6] p-4 transition-all duration-200 hover:-translate-y-0.5 hover:border-[#7b6857]/18 hover:bg-white dark:border-white/10 dark:bg-dark-950/35 dark:hover:bg-dark-950/50"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div class="min-w-0">
                      <div class="text-sm font-semibold text-gray-900 dark:text-white">
                        {{ card.title }}
                      </div>
                      <div class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-dark-300">
                        {{ card.endpoint }}
                      </div>
                    </div>
                    <div class="rounded-full bg-[#efe4d6] px-2.5 py-1 text-xs font-semibold text-[#6f5a49] dark:bg-white/10 dark:text-dark-100">
                      {{ card.paramCount }}
                    </div>
                  </div>
                </a>
              </div>

              <div class="mt-5 rounded-[24px] border border-dashed border-[#7b6857]/14 bg-[#f7f0e7] px-4 py-3 text-sm leading-relaxed text-[#6f5a49] dark:border-white/10 dark:bg-white/5 dark:text-dark-200">
                {{ localizeText(text('每个协议区块都内置参数表、请求头说明和 4 种语言调用示例。', 'Each protocol block includes request parameters, header guidance, and code samples in four languages.')) }}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-6xl px-6 py-12 lg:grid lg:grid-cols-[260px_minmax(0,1fr)] lg:gap-8">
        <aside class="mb-8 lg:sticky lg:top-28 lg:mb-0 lg:self-start">
          <div class="rounded-[30px] border border-[#7b6857]/10 bg-white/80 p-5 shadow-[0_18px_45px_rgba(91,73,55,0.08)] backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <div class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('integrationDocs.sidebar.title') }}
            </div>
            <p class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-dark-300">
              {{ localizeText(text('从协议说明、参数到示例代码，按协议快速定位。', 'Jump from protocol details to parameters and code samples.')) }}
            </p>
            <nav class="mt-5 flex flex-col gap-2">
              <a
                v-for="item in pageNav"
                :key="item.id"
                :href="`#${item.id}`"
                :aria-current="activeSectionId === item.id ? 'location' : undefined"
                class="group flex items-center justify-between rounded-2xl border px-3 py-3 text-sm transition-all"
                :class="
                  activeSectionId === item.id
                    ? 'border-[#7b6857]/16 bg-[#f1e5d8] text-gray-900 shadow-sm dark:border-white/10 dark:bg-white/12 dark:text-white'
                    : 'border-transparent text-gray-600 hover:border-[#7b6857]/10 hover:bg-[#f6efe6] hover:text-gray-900 dark:text-dark-300 dark:hover:border-white/10 dark:hover:bg-white/10 dark:hover:text-white'
                "
                @click="setActiveSection(item.id)"
              >
                <span class="flex items-center gap-2 font-medium">
                  <span
                    class="h-2 w-2 rounded-full transition-colors"
                    :class="
                      activeSectionId === item.id
                        ? 'bg-[#7b6857] dark:bg-primary-200'
                        : 'bg-[#d7c3ae] group-hover:bg-[#7b6857] dark:bg-white/15 dark:group-hover:bg-dark-100'
                    "
                  ></span>
                  {{ item.label }}
                </span>
                <span
                  class="text-xs transition-colors"
                  :class="
                    activeSectionId === item.id
                      ? 'text-[#7b6857] dark:text-dark-100'
                      : 'text-gray-400 group-hover:text-[#7b6857] dark:text-dark-400 dark:group-hover:text-dark-100'
                  "
                >
                  {{ item.meta }}
                </span>
              </a>
            </nav>
          </div>
        </aside>

        <div class="space-y-8">
          <section
            v-for="section in protocols"
            :id="section.id"
            :key="section.id"
            class="relative overflow-hidden rounded-[32px] border border-[#7b6857]/10 bg-white/80 p-6 shadow-[0_22px_60px_rgba(91,73,55,0.08)] backdrop-blur dark:border-white/10 dark:bg-dark-900/40 sm:p-8 scroll-mt-32"
          >
            <div class="pointer-events-none absolute right-0 top-0 h-40 w-40 rounded-full bg-[#e7d8c4]/55 blur-3xl dark:bg-primary-500/10"></div>
            <div class="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[#b89a7a]/70 to-transparent dark:via-white/20"></div>
            <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div class="max-w-3xl">
                <span class="inline-flex items-center rounded-full border border-[#7b6857]/15 bg-[#f5ece0] px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-[#7b6857] dark:border-primary-400/20 dark:bg-primary-500/10 dark:text-primary-200">
                  {{ section.badge }}
                </span>
                <h2 class="mt-4 text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">
                  {{ section.title }}
                </h2>
                <p class="mt-3 text-base leading-relaxed text-gray-600 dark:text-dark-300">
                  {{ section.description }}
                </p>
                <div class="mt-5 flex flex-wrap gap-2">
                  <span
                    v-for="chip in sectionSummaryChips(section)"
                    :key="`${section.id}-${chip}`"
                    class="rounded-full border border-[#7b6857]/10 bg-[#f7efe4] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200"
                  >
                    {{ chip }}
                  </span>
                </div>
              </div>

              <div class="rounded-[28px] border border-[#7b6857]/10 bg-[#fbf8f2] p-4 text-sm shadow-sm dark:border-white/10 dark:bg-dark-950/30 lg:min-w-[280px]">
                <div class="text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ t('integrationDocs.fields.endpoint') }}
                </div>
                <div class="mt-3 space-y-2">
                  <div
                    v-for="endpoint in section.endpoints"
                    :key="`${section.id}-${endpoint}`"
                    class="rounded-2xl border border-[#7b6857]/10 bg-[#231d17] px-3 py-2 font-mono text-sm text-[#f8efe3]"
                  >
                    {{ endpoint }}
                  </div>
                </div>
                <div class="mt-4 text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ t('integrationDocs.fields.method') }}
                </div>
                <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ section.method }}
                </div>
              </div>
            </div>

            <div class="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div
                v-for="header in section.headers"
                :key="`${section.id}-${header.label}`"
                class="rounded-[24px] border border-[#7b6857]/10 bg-[#fcfaf6] p-4 dark:border-white/10 dark:bg-dark-950/30"
              >
                <div class="text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ header.label }}
                </div>
                <div class="mt-2 break-all font-mono text-sm text-gray-900 dark:text-white">
                  {{ header.value }}
                </div>
              </div>
            </div>

            <ul class="mt-6 space-y-3 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
              <li
                v-for="item in section.bullets"
                :key="item"
                class="flex gap-3 rounded-[24px] border border-[#7b6857]/8 bg-[#fbf8f2] px-4 py-3 dark:border-white/10 dark:bg-dark-950/25"
              >
                <span class="mt-1 h-2 w-2 rounded-full bg-[#7b6857] dark:bg-primary-300"></span>
                <span>{{ item }}</span>
              </li>
            </ul>

            <div class="mt-8">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                    {{ t('integrationDocs.fields.parametersTitle') }}
                  </h3>
                  <p v-if="section.parameterNotice" class="mt-1 text-sm text-gray-600 dark:text-dark-300">
                    {{ section.parameterNotice }}
                  </p>
                </div>
                <div class="flex flex-wrap gap-2">
                  <span
                    class="rounded-full border border-[#7b6857]/10 bg-[#f7efe4] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200"
                  >
                    {{ localizeText(text(`${section.params.length} 个主参数`, `${section.params.length} primary fields`)) }}
                  </span>
                  <span
                    v-if="section.extraParameterGroups.length"
                    class="rounded-full border border-[#7b6857]/10 bg-[#f7efe4] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200"
                  >
                    {{ localizeText(text(`${section.extraParameterGroups.length} 组扩展参数`, `${section.extraParameterGroups.length} extension groups`)) }}
                  </span>
                </div>
              </div>

              <div class="mt-4 overflow-hidden rounded-[28px] border border-[#7b6857]/10 bg-white/70 shadow-sm dark:border-white/10 dark:bg-dark-900/30">
                <div class="overflow-x-auto">
                  <table class="min-w-full divide-y divide-black/5 text-sm dark:divide-white/10">
                    <thead class="bg-[#f7efe4] dark:bg-dark-950/30">
                      <tr>
                        <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                          {{ t('integrationDocs.fields.parameterName') }}
                        </th>
                        <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                          {{ t('integrationDocs.fields.parameterType') }}
                        </th>
                        <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                          {{ t('integrationDocs.fields.parameterRequired') }}
                        </th>
                        <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                          {{ t('integrationDocs.fields.parameterDescription') }}
                        </th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-black/5 bg-white/70 dark:divide-white/10 dark:bg-dark-900/30">
                      <tr
                        v-for="param in section.params"
                        :key="`${section.id}-${param.name}`"
                        class="transition-colors hover:bg-[#fcf7f0] dark:hover:bg-white/5"
                      >
                        <td class="px-4 py-4 align-top">
                          <code class="rounded-xl bg-[#f2e6d9] px-2 py-1 text-[13px] text-gray-900 dark:bg-white/10 dark:text-white">
                            {{ param.name }}
                          </code>
                        </td>
                        <td class="px-4 py-4 align-top text-gray-700 dark:text-dark-200">
                          {{ param.type }}
                        </td>
                        <td class="px-4 py-4 align-top">
                          <span
                            class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold"
                            :class="
                              param.required
                                ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200'
                                : 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200'
                            "
                          >
                            {{ param.required ? t('integrationDocs.fields.requiredLabel') : t('integrationDocs.fields.optionalLabel') }}
                          </span>
                        </td>
                        <td class="px-4 py-4 align-top">
                          <p class="leading-relaxed text-gray-700 dark:text-dark-200">
                            {{ localizeText(param.description) }}
                          </p>
                          <p v-if="param.values" class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                            {{ t('integrationDocs.fields.supportedValues') }}: {{ localizeText(param.values) }}
                          </p>
                          <p v-if="param.notes" class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                            {{ localizeText(param.notes) }}
                          </p>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <div
                v-for="group in section.extraParameterGroups"
                :key="`${section.id}-${localizeText(group.title)}`"
                class="mt-6 rounded-[28px] border border-[#7b6857]/10 bg-[#fcfaf6] p-5 dark:border-white/10 dark:bg-dark-950/20"
              >
                <div class="flex flex-col gap-2">
                  <h4 class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ localizeText(group.title) }}
                  </h4>
                  <p v-if="group.description" class="text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                    {{ localizeText(group.description) }}
                  </p>
                </div>

                <div class="mt-4 overflow-hidden rounded-[24px] border border-[#7b6857]/10 bg-white/70 dark:border-white/10 dark:bg-dark-900/30">
                  <div class="overflow-x-auto">
                    <table class="min-w-full divide-y divide-black/5 text-sm dark:divide-white/10">
                      <thead class="bg-[#f7efe4] dark:bg-dark-950/30">
                        <tr>
                          <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                            {{ t('integrationDocs.fields.parameterName') }}
                          </th>
                          <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                            {{ t('integrationDocs.fields.parameterType') }}
                          </th>
                          <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                            {{ t('integrationDocs.fields.parameterRequired') }}
                          </th>
                          <th class="px-4 py-3 text-left font-semibold text-gray-900 dark:text-white">
                            {{ t('integrationDocs.fields.parameterDescription') }}
                          </th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-black/5 bg-white/70 dark:divide-white/10 dark:bg-dark-900/30">
                        <tr
                          v-for="param in group.params"
                          :key="`${section.id}-${localizeText(group.title)}-${param.name}`"
                          class="transition-colors hover:bg-[#fcf7f0] dark:hover:bg-white/5"
                        >
                          <td class="px-4 py-4 align-top">
                            <code class="rounded-xl bg-[#f2e6d9] px-2 py-1 text-[13px] text-gray-900 dark:bg-white/10 dark:text-white">
                              {{ param.name }}
                            </code>
                          </td>
                          <td class="px-4 py-4 align-top text-gray-700 dark:text-dark-200">
                            {{ param.type }}
                          </td>
                          <td class="px-4 py-4 align-top">
                            <span
                              class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold"
                              :class="
                                param.required
                                  ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200'
                                  : 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200'
                              "
                            >
                              {{ param.required ? t('integrationDocs.fields.requiredLabel') : t('integrationDocs.fields.optionalLabel') }}
                            </span>
                          </td>
                          <td class="px-4 py-4 align-top">
                            <p class="leading-relaxed text-gray-700 dark:text-dark-200">
                              {{ localizeText(param.description) }}
                            </p>
                            <p v-if="param.values" class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                              {{ t('integrationDocs.fields.supportedValues') }}: {{ localizeText(param.values) }}
                            </p>
                            <p v-if="param.notes" class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                              {{ localizeText(param.notes) }}
                            </p>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-8">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('integrationDocs.fields.exampleTabsTitle') }}
                </div>
                <div class="rounded-full border border-[#7b6857]/10 bg-[#f7efe4] px-3 py-1 text-xs font-medium text-[#6d5c4d] dark:border-white/10 dark:bg-white/5 dark:text-dark-200">
                  {{ localizeText(text(`${section.examples.length} 种语言切换`, `${section.examples.length} switchable languages`)) }}
                </div>
              </div>
              <div class="mt-4 inline-flex flex-wrap gap-2 rounded-[24px] border border-[#7b6857]/10 bg-[#fbf8f2] p-2 dark:border-white/10 dark:bg-dark-950/25">
                <button
                  v-for="example in section.examples"
                  :key="`${section.id}-${example.id}`"
                  type="button"
                  :data-testid="`example-tab-${section.id}-${example.id}`"
                  :aria-pressed="activeExampleTab(section.id) === example.id"
                  class="rounded-full px-4 py-2 text-sm font-medium transition-all"
                  :class="
                    activeExampleTab(section.id) === example.id
                      ? 'bg-[#7b6857] text-white shadow-[0_10px_25px_rgba(123,104,87,0.25)] dark:bg-white dark:text-gray-900'
                      : 'bg-transparent text-gray-700 hover:bg-white hover:text-gray-900 dark:text-dark-200 dark:hover:bg-white/10 dark:hover:text-white'
                  "
                  @click="setActiveExampleTab(section.id, example.id)"
                >
                  {{ example.label }}
                </button>
              </div>

              <div class="mt-4 overflow-hidden rounded-[28px] border border-[#231d17] bg-[#231d17] text-[14px] text-[#f7efe5] shadow-[0_25px_70px_rgba(35,29,23,0.22)]">
                <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
                  <div class="flex items-center gap-2">
                    <span class="h-2.5 w-2.5 rounded-full bg-[#c7765d]"></span>
                    <span class="h-2.5 w-2.5 rounded-full bg-[#d9b267]"></span>
                    <span class="h-2.5 w-2.5 rounded-full bg-[#7caa73]"></span>
                  </div>
                  <div class="flex items-center gap-2">
                    <div class="rounded-full border border-white/10 bg-white/5 px-3 py-1 font-mono text-xs text-[#d9c8b6]">
                      {{ section.endpoints[0] }}
                    </div>
                    <button
                      type="button"
                      :data-testid="`example-copy-${section.id}`"
                      class="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-1.5 text-xs font-medium text-[#f7efe5] transition-all hover:bg-white/10"
                      :aria-label="copiedExampleSectionId === section.id ? t('common.copied') : t('keys.copyToClipboard')"
                      @click="copyExampleSnippet(section)"
                    >
                      <Icon
                        :name="copiedExampleSectionId === section.id ? 'check' : 'clipboard'"
                        size="sm"
                        :stroke-width="2"
                      />
                      <span>{{ copiedExampleSectionId === section.id ? t('common.copied') : t('keys.copyToClipboard') }}</span>
                    </button>
                  </div>
                </div>
                <pre
                  :data-testid="`example-code-${section.id}`"
                  class="overflow-x-auto p-4 font-mono leading-relaxed"
                ><code v-text="activeExampleSnippet(section)"></code></pre>
              </div>
            </div>
          </section>
        </div>
      </section>
    </main>

    <footer class="border-t border-[#7b6857]/15 bg-white/25 px-6 py-2 dark:border-white/10 dark:bg-dark-900/20">
      <div class="mx-auto max-w-6xl text-center">
        <p class="text-xs text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { resolveOpenAIUseKeyModelId } from '@/utils/openaiUseKeyModel'

type LocalizedText = {
  zh: string
  en: string
}

type HeaderItem = {
  label: string
  value: string
}

type ProtocolParam = {
  name: string
  type: string
  required: boolean
  description: LocalizedText
  values?: LocalizedText
  notes?: LocalizedText
}

type ExampleTabId = 'curl' | 'python' | 'javascript' | 'java'

type ProtocolExample = {
  id: ExampleTabId
  label: string
  code: string
}

type ExtraParameterGroup = {
  title: LocalizedText
  description?: LocalizedText
  params: ProtocolParam[]
}

type ProtocolSection = {
  id: string
  badge: string
  title: string
  description: string
  method: string
  endpoints: string[]
  headers: HeaderItem[]
  bullets: string[]
  parameterNotice?: string
  params: ProtocolParam[]
  extraParameterGroups: ExtraParameterGroup[]
  examples: ProtocolExample[]
}

const { t, locale } = useI18n()

const appStore = useAppStore()

const apiKeyPlaceholder = 'sk-your-api-key'
const anthropicVersion = '2023-06-01'
const defaultExampleTab: ExampleTabId = 'curl'

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const currentYear = new Date().getFullYear()
const docsLocale = computed<'zh' | 'en'>(() => (locale.value || '').startsWith('zh') ? 'zh' : 'en')
const { copyToClipboard } = useClipboard()

const baseUrl = computed(() => {
  const raw = (appStore.apiBaseUrl || '').trim()
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const candidate = raw || origin
  return candidate.replace(/\/api\/v1\/?$/, '').replace(/\/+$/, '')
})

const openAIModel = computed(() => resolveOpenAIUseKeyModelId(appStore.openaiUseKeyModelId))
const anthropicModel = 'claude-sonnet-4-5'
const imageModel = 'gpt-image-1'
const activeExampleTabs = ref<Record<string, ExampleTabId>>({})
const copiedExampleSectionId = ref<string | null>(null)
const activeSectionId = ref('')
let copiedResetTimer: ReturnType<typeof setTimeout> | null = null
let sectionObserver: IntersectionObserver | null = null

const standardPaths = computed(() => [
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1/images/generations',
  '/v1/images/edits'
])

function text(zh: string, en: string): LocalizedText {
  return { zh, en }
}

function localizeText(value: LocalizedText): string {
  return docsLocale.value === 'zh' ? value.zh : value.en
}

function param(
  name: string,
  type: string,
  required: boolean,
  description: LocalizedText,
  values?: LocalizedText,
  notes?: LocalizedText
): ProtocolParam {
  return {
    name,
    type,
    required,
    description,
    values,
    notes
  }
}

function stringifyBody(body: unknown): string {
  return JSON.stringify(body, null, 2)
}

function buildCurl(url: string, headers: HeaderItem[], body: unknown): string {
  const headerLines = headers.map((header) => `  -H "${header.value}"`).join(' \\\n')
  return `curl "${url}" \\
${headerLines} \\
  -d '${stringifyBody(body)}'`
}

function buildPython(url: string, headers: Record<string, string>, body: unknown): string {
  return `import requests

url = "${url}"
headers = ${stringifyBody(headers)}
payload = ${stringifyBody(body)}

response = requests.post(url, headers=headers, json=payload, timeout=60)
response.raise_for_status()
print(response.json())`
}

function buildJavaScript(url: string, headers: Record<string, string>, body: unknown): string {
  return `const response = await fetch("${url}", {
  method: "POST",
  headers: ${stringifyBody(headers)},
  body: JSON.stringify(${stringifyBody(body)})
})

const data = await response.json()
console.log(data)`
}

function buildJava(url: string, headers: Record<string, string>, body: unknown): string {
  const headerLines = Object.entries(headers)
    .map(([key, value]) => `.header("${key}", "${value}")`)
    .join('\n    ')

  return `import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class Example {
  public static void main(String[] args) throws Exception {
    HttpClient client = HttpClient.newHttpClient();
    String body = """
${stringifyBody(body)}
""";

    HttpRequest request = HttpRequest.newBuilder()
      .uri(URI.create("${url}"))
      ${headerLines}
      .POST(HttpRequest.BodyPublishers.ofString(body))
      .build();

    HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());
    System.out.println(response.body());
  }
}`
}

function activeExampleTab(sectionId: string): ExampleTabId {
  return activeExampleTabs.value[sectionId] || defaultExampleTab
}

function setActiveSection(sectionId: string): void {
  activeSectionId.value = sectionId
}

function setActiveExampleTab(sectionId: string, exampleId: ExampleTabId): void {
  activeExampleTabs.value = {
    ...activeExampleTabs.value,
    [sectionId]: exampleId
  }
}

function activeExampleSnippet(section: ProtocolSection): string {
  const targetId = activeExampleTab(section.id)
  const selected = section.examples.find((item) => item.id === targetId)
  return selected?.code || section.examples[0]?.code || ''
}

async function copyExampleSnippet(section: ProtocolSection): Promise<void> {
  const code = activeExampleSnippet(section)
  const success = await copyToClipboard(code, t('common.copiedToClipboard'))
  if (!success) {
    return
  }

  copiedExampleSectionId.value = section.id
  if (copiedResetTimer) {
    clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = setTimeout(() => {
    copiedExampleSectionId.value = null
    copiedResetTimer = null
  }, 2000)
}

const protocols = computed<ProtocolSection[]>(() => {
  const openAIHeaders: HeaderItem[] = [
    {
      label: t('integrationDocs.fields.authHeader'),
      value: `Authorization: Bearer ${apiKeyPlaceholder}`
    },
    {
      label: t('integrationDocs.fields.contentType'),
      value: 'Content-Type: application/json'
    }
  ]

  const anthropicHeaders: HeaderItem[] = [
    {
      label: t('integrationDocs.fields.authHeader'),
      value: `x-api-key: ${apiKeyPlaceholder}`
    },
    {
      label: t('integrationDocs.fields.versionHeader'),
      value: `anthropic-version: ${anthropicVersion}`
    },
    {
      label: t('integrationDocs.fields.contentType'),
      value: 'Content-Type: application/json'
    }
  ]

  const openAIChatBody = {
    model: openAIModel.value,
    messages: [
      { role: 'system', content: 'You are a helpful assistant.' },
      { role: 'user', content: '用一句话介绍 FluxCode。' }
    ],
    temperature: 0.7
  }

  const responsesBody = {
    model: openAIModel.value,
    instructions: 'You are a helpful assistant.',
    input: [
      {
        role: 'user',
        content: [{ type: 'input_text', text: '给我一个接入成功排查清单。' }]
      }
    ]
  }

  const anthropicBody = {
    model: anthropicModel,
    max_tokens: 1024,
    messages: [{ role: 'user', content: '请总结 FluxCode 的主要能力。' }]
  }

  const imageBody = {
    model: imageModel,
    prompt: '一张具有未来感的 API 网关控制台插画，浅色背景，蓝金配色',
    size: '1024x1024',
    response_format: 'url'
  }

  const openAIChatExamples: ProtocolExample[] = [
    {
      id: 'curl',
      label: 'cURL',
      code: buildCurl(`${baseUrl.value}/v1/chat/completions`, openAIHeaders, openAIChatBody)
    },
    {
      id: 'python',
      label: 'Python',
      code: buildPython(
        `${baseUrl.value}/v1/chat/completions`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        openAIChatBody
      )
    },
    {
      id: 'javascript',
      label: 'JavaScript',
      code: buildJavaScript(
        `${baseUrl.value}/v1/chat/completions`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        openAIChatBody
      )
    },
    {
      id: 'java',
      label: 'Java',
      code: buildJava(
        `${baseUrl.value}/v1/chat/completions`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        openAIChatBody
      )
    }
  ]

  const responsesExamples: ProtocolExample[] = [
    {
      id: 'curl',
      label: 'cURL',
      code: buildCurl(`${baseUrl.value}/v1/responses`, openAIHeaders, responsesBody)
    },
    {
      id: 'python',
      label: 'Python',
      code: buildPython(
        `${baseUrl.value}/v1/responses`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        responsesBody
      )
    },
    {
      id: 'javascript',
      label: 'JavaScript',
      code: buildJavaScript(
        `${baseUrl.value}/v1/responses`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        responsesBody
      )
    },
    {
      id: 'java',
      label: 'Java',
      code: buildJava(
        `${baseUrl.value}/v1/responses`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        responsesBody
      )
    }
  ]

  const anthropicExamples: ProtocolExample[] = [
    {
      id: 'curl',
      label: 'cURL',
      code: buildCurl(`${baseUrl.value}/v1/messages`, anthropicHeaders, anthropicBody)
    },
    {
      id: 'python',
      label: 'Python',
      code: buildPython(
        `${baseUrl.value}/v1/messages`,
        {
          'x-api-key': apiKeyPlaceholder,
          'anthropic-version': anthropicVersion,
          'Content-Type': 'application/json'
        },
        anthropicBody
      )
    },
    {
      id: 'javascript',
      label: 'JavaScript',
      code: buildJavaScript(
        `${baseUrl.value}/v1/messages`,
        {
          'x-api-key': apiKeyPlaceholder,
          'anthropic-version': anthropicVersion,
          'Content-Type': 'application/json'
        },
        anthropicBody
      )
    },
    {
      id: 'java',
      label: 'Java',
      code: buildJava(
        `${baseUrl.value}/v1/messages`,
        {
          'x-api-key': apiKeyPlaceholder,
          'anthropic-version': anthropicVersion,
          'Content-Type': 'application/json'
        },
        anthropicBody
      )
    }
  ]

  const imageExamples: ProtocolExample[] = [
    {
      id: 'curl',
      label: 'cURL',
      code: buildCurl(`${baseUrl.value}/v1/images/generations`, openAIHeaders, imageBody)
    },
    {
      id: 'python',
      label: 'Python',
      code: buildPython(
        `${baseUrl.value}/v1/images/generations`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        imageBody
      )
    },
    {
      id: 'javascript',
      label: 'JavaScript',
      code: buildJavaScript(
        `${baseUrl.value}/v1/images/generations`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        imageBody
      )
    },
    {
      id: 'java',
      label: 'Java',
      code: buildJava(
        `${baseUrl.value}/v1/images/generations`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        imageBody
      )
    }
  ]

  return [
    {
      id: 'openai-chat',
      badge: t('integrationDocs.sections.openaiChat.badge'),
      title: t('integrationDocs.sections.openaiChat.title'),
      description: t('integrationDocs.sections.openaiChat.description'),
      method: 'POST',
      endpoints: ['/v1/chat/completions'],
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiChat.bullets.item1'),
        t('integrationDocs.sections.openaiChat.bullets.item2'),
        t('integrationDocs.sections.openaiChat.bullets.item3')
      ],
      parameterNotice:
        docsLocale.value === 'zh'
          ? '以下为当前网关稳定支持的 Chat 请求字段，工具定义与消息内容支持嵌套对象。'
          : 'The table below lists the Chat request fields stably supported by the gateway, including nested tool and message structures.',
      params: [
        param('model', 'string', true, text('目标模型 ID。', 'Target model ID.')),
        param('messages', 'array', true, text('对话消息数组。', 'Conversation message array.')),
        param('messages[].role', 'string', true, text('消息角色，可为 system、user、assistant、tool、function。', 'Message role: system, user, assistant, tool, or function.')),
        param(
          'messages[].content',
          'string | array',
          false,
          text('消息正文，可直接传文本，或传多模态内容数组。', 'Message content, either plain text or a multimodal content array.')
        ),
        param(
          'messages[].name',
          'string',
          false,
          text('消息发送者名称，常用于 function / tool 兼容场景。', 'Optional sender name, commonly used in function or tool compatibility flows.')
        ),
        param(
          'messages[].tool_calls',
          'array',
          false,
          text('助手发起工具调用时使用的数组。', 'Assistant tool-call array.')
        ),
        param(
          'messages[].tool_calls[].id',
          'string',
          false,
          text('单次工具调用的唯一 ID。', 'Unique ID for a tool call.')
        ),
        param(
          'messages[].tool_calls[].type',
          'string',
          false,
          text('工具调用类型。', 'Tool call type.'),
          text('function', 'function')
        ),
        param(
          'messages[].tool_calls[].function.name',
          'string',
          false,
          text('被调用的函数名。', 'Invoked function name.')
        ),
        param(
          'messages[].tool_calls[].function.arguments',
          'string',
          false,
          text('函数调用参数 JSON 字符串。', 'JSON string containing function call arguments.')
        ),
        param(
          'messages[].tool_call_id',
          'string',
          false,
          text('tool 角色消息回传工具结果时对应的 call id。', 'Call ID used when a tool-role message returns tool output.')
        ),
        param(
          'messages[].function_call',
          'object',
          false,
          text('旧版 function calling 兼容字段。', 'Legacy function-calling compatibility field.')
        ),
        param(
          'messages[].function_call.name',
          'string',
          false,
          text('旧版 function calling 的函数名。', 'Function name in legacy function calling.')
        ),
        param(
          'messages[].function_call.arguments',
          'string',
          false,
          text('旧版 function calling 的参数 JSON 字符串。', 'JSON string of arguments in legacy function calling.')
        ),
        param(
          'messages[].reasoning_content',
          'string',
          false,
          text('部分兼容客户端会附带 reasoning 内容。', 'Reasoning content used by some compatible clients.')
        ),
        param(
          'messages[].content[].type',
          'string',
          false,
          text('多模态内容类型，常见为 text 或 image_url。', 'Multimodal part type, commonly text or image_url.'),
          text('text, image_url', 'text, image_url')
        ),
        param(
          'messages[].content[].text',
          'string',
          false,
          text('当 type=text 时的文本内容。', 'Text payload when type=text.')
        ),
        param(
          'messages[].content[].image_url.url',
          'string',
          false,
          text('图片 URL 或 data URL。', 'Image URL or data URL.')
        ),
        param(
          'messages[].content[].image_url.detail',
          'string',
          false,
          text('图片理解精度。', 'Image detail level.'),
          text('auto, low, high', 'auto, low, high')
        ),
        param(
          'instructions',
          'string',
          false,
          text('Responses 兼容字段，适合把统一系统指令写在顶层。', 'Responses-compatible top-level instructions field.')
        ),
        param('max_tokens', 'integer', false, text('输出 token 上限。', 'Maximum output tokens.')),
        param(
          'max_completion_tokens',
          'integer',
          false,
          text('部分客户端使用的完成 token 上限别名。', 'Alternate completion token cap used by some clients.')
        ),
        param('temperature', 'number', false, text('采样温度。', 'Sampling temperature.')),
        param('top_p', 'number', false, text('核采样参数。', 'Nucleus sampling parameter.')),
        param('stream', 'boolean', false, text('是否启用流式输出。', 'Enable streaming output.')),
        param(
          'stream_options',
          'object',
          false,
          text('流式输出附加配置。', 'Additional streaming options.')
        ),
        param(
          'stream_options.include_usage',
          'boolean',
          false,
          text('流式响应末尾是否附带 usage。', 'Whether to append usage info at the end of a stream.')
        ),
        param('tools', 'array', false, text('工具定义数组。', 'Tool definition array.')),
        param('tools[].type', 'string', false, text('工具类型。', 'Tool type.'), text('function', 'function')),
        param(
          'tools[].function.name',
          'string',
          false,
          text('函数工具名称。', 'Function tool name.')
        ),
        param(
          'tools[].function.description',
          'string',
          false,
          text('函数工具描述。', 'Function tool description.')
        ),
        param(
          'tools[].function.parameters',
          'object',
          false,
          text('JSON Schema 形式的入参定义。', 'JSON Schema definition for the function input.')
        ),
        param(
          'tools[].function.strict',
          'boolean',
          false,
          text('是否要求严格按 schema 生成。', 'Whether schema output must be strict.')
        ),
        param(
          'tool_choice',
          'string | object',
          false,
          text('控制工具调用策略。', 'Controls tool-calling behavior.')
        ),
        param(
          'reasoning_effort',
          'string',
          false,
          text('推理强度，适用于兼容 reasoning 参数的模型。', 'Reasoning effort for compatible models.'),
          text('low, medium, high, xhigh', 'low, medium, high, xhigh')
        ),
        param(
          'service_tier',
          'string',
          false,
          text('服务优先级设置。', 'Service priority tier.'),
          text('auto, default, flex, priority', 'auto, default, flex, priority'),
          text('网关会上游规范化，只保留上游可识别值。', 'The gateway normalizes this field to values accepted upstream.')
        ),
        param(
          'stop',
          'string | string[]',
          false,
          text('停止词，可传单个字符串或字符串数组。', 'Stop sequence as a string or string array.')
        ),
        param(
          'functions',
          'array',
          false,
          text('旧版 function calling 定义数组。', 'Legacy function-calling definition array.')
        ),
        param(
          'functions[].name',
          'string',
          false,
          text('旧版函数名称。', 'Legacy function name.')
        ),
        param(
          'functions[].description',
          'string',
          false,
          text('旧版函数描述。', 'Legacy function description.')
        ),
        param(
          'functions[].parameters',
          'object',
          false,
          text('旧版函数入参 JSON Schema。', 'JSON Schema for legacy function inputs.')
        ),
        param(
          'function_call',
          'string | object',
          false,
          text('旧版 function calling 调度控制字段。', 'Legacy function-calling dispatch control field.')
        )
      ],
      extraParameterGroups: [],
      examples: openAIChatExamples
    },
    {
      id: 'openai-responses',
      badge: t('integrationDocs.sections.openaiResponses.badge'),
      title: t('integrationDocs.sections.openaiResponses.title'),
      description: t('integrationDocs.sections.openaiResponses.description'),
      method: 'POST',
      endpoints: ['/v1/responses'],
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiResponses.bullets.item1'),
        t('integrationDocs.sections.openaiResponses.bullets.item2'),
        t('integrationDocs.sections.openaiResponses.bullets.item3')
      ],
      parameterNotice:
        docsLocale.value === 'zh'
          ? '此处列出当前网关稳定识别的 Responses 顶层字段；续链、WSv2 与工具结果相关字段会单独标注。'
          : 'This table lists the top-level Responses fields stably recognized by the gateway, with continuation and tool-result fields called out separately.',
      params: [
        param('model', 'string', true, text('目标模型 ID。', 'Target model ID.')),
        param('instructions', 'string', false, text('顶层系统指令。', 'Top-level system instructions.')),
        param(
          'input',
          'string | array',
          true,
          text('输入正文，可为字符串或结构化数组。', 'Input payload as a string or structured array.')
        ),
        param(
          'input[].role',
          'string',
          false,
          text('当输入项为消息时的角色。', 'Role when the input item is a message.')
        ),
        param(
          'input[].content',
          'string | array',
          false,
          text('消息内容，可直接传字符串，或传 content parts。', 'Message content as a string or content parts.')
        ),
        param(
          'input[].content[].type',
          'string',
          false,
          text('内容块类型。', 'Content part type.'),
          text('input_text, output_text, input_image', 'input_text, output_text, input_image')
        ),
        param(
          'input[].content[].text',
          'string',
          false,
          text('文本内容。', 'Text content.')
        ),
        param(
          'input[].content[].image_url',
          'string',
          false,
          text('图片输入 URL 或 data URL。', 'Image input URL or data URL.')
        ),
        param(
          'input[].type',
          'string',
          false,
          text('结构化输入项类型。', 'Structured input item type.'),
          text(
            'message-like, function_call, function_call_output, item_reference, tool_call, local_shell_call, tool_search_call, custom_tool_call, mcp_tool_call, tool_search_output, custom_tool_call_output, mcp_tool_call_output',
            'message-like, function_call, function_call_output, item_reference, tool_call, local_shell_call, tool_search_call, custom_tool_call, mcp_tool_call, tool_search_output, custom_tool_call_output, mcp_tool_call_output'
          )
        ),
        param(
          'input[].call_id',
          'string',
          false,
          text('函数调用或函数输出关联的 call id。', 'Call ID used by function calls or function outputs.')
        ),
        param(
          'input[].name',
          'string',
          false,
          text('函数调用名称。', 'Function call name.')
        ),
        param(
          'input[].arguments',
          'string',
          false,
          text('函数调用参数 JSON 字符串。', 'JSON string containing function call arguments.')
        ),
        param(
          'input[].id',
          'string',
          false,
          text('结构化输入项 ID，也用于 item_reference.id。', 'Structured input item ID, also used by item_reference.id.')
        ),
        param(
          'input[].output',
          'string',
          false,
          text('function_call_output 的输出文本。', 'Output text for function_call_output.')
        ),
        param(
          'input[].text',
          'string',
          false,
          text('部分 item_reference 或兼容输入项附带的文本内容。', 'Text content attached by some item_reference or compatible input items.')
        ),
        param(
          'max_output_tokens',
          'integer',
          false,
          text('输出 token 上限。', 'Maximum output tokens.')
        ),
        param('temperature', 'number', false, text('采样温度。', 'Sampling temperature.')),
        param('top_p', 'number', false, text('核采样参数。', 'Nucleus sampling parameter.')),
        param('stream', 'boolean', false, text('是否启用 SSE 流式响应。', 'Enable SSE streaming response.')),
        param('tools', 'array', false, text('工具定义数组。', 'Tool definition array.')),
        param(
          'tools[].type',
          'string',
          false,
          text('工具类型。', 'Tool type.'),
          text('function, web_search, image_generation 等兼容类型', 'Compatible types such as function, web_search, and image_generation')
        ),
        param(
          'tools[].name',
          'string',
          false,
          text('工具名称。', 'Tool name.')
        ),
        param(
          'tools[].description',
          'string',
          false,
          text('工具描述。', 'Tool description.')
        ),
        param(
          'tools[].parameters',
          'object',
          false,
          text('函数工具的 JSON Schema。', 'JSON Schema for function tools.')
        ),
        param(
          'tools[].strict',
          'boolean',
          false,
          text('是否要求严格遵守 schema。', 'Whether schema output must be strict.')
        ),
        param(
          'include',
          'string[]',
          false,
          text('要求上游在响应中补充的附加字段列表。', 'Additional response fields to ask upstream to include.')
        ),
        param(
          'store',
          'boolean',
          false,
          text('是否依赖服务端历史状态。', 'Whether the request should rely on server-side stored history.'),
          undefined,
          text('部分 OAuth / WS 场景下网关会主动规范为 false。', 'In some OAuth or WS flows, the gateway may normalize this field to false.')
        ),
        param(
          'reasoning.effort',
          'string',
          false,
          text('推理强度。', 'Reasoning effort.'),
          text('none, low, medium, high', 'none, low, medium, high'),
          text('网关会把 minimal 归一化为 none。', 'The gateway normalizes minimal to none.')
        ),
        param(
          'reasoning.summary',
          'string',
          false,
          text('推理摘要粒度。', 'Reasoning summary verbosity.'),
          text('auto, concise, detailed', 'auto, concise, detailed')
        ),
        param(
          'tool_choice',
          'string | object',
          false,
          text('控制工具调用策略。', 'Controls tool-calling behavior.')
        ),
        param(
          'service_tier',
          'string',
          false,
          text('服务优先级设置。', 'Service priority tier.'),
          text('auto, default, flex, priority', 'auto, default, flex, priority'),
          text('网关会把值规范为上游支持的 tier。', 'The gateway normalizes the value to an upstream-supported tier.')
        ),
        param(
          'previous_response_id',
          'string',
          false,
          text('续链时引用上一轮 response.id。', 'Continuation anchor referencing the previous response.id.'),
          text('resp_*', 'resp_*'),
          text('不能传 message id；工具输出续链时尤其关键。', 'Do not pass a message ID; this is especially important for tool-output continuation.')
        ),
        param(
          'prompt_cache_key',
          'string',
          false,
          text('会话稳定标识，可用于 prompt cache 或会话亲和。', 'Stable session seed for prompt cache or sticky session routing.')
        ),
        param(
          'metadata.user_id',
          'string',
          false,
          text('部分兼容客户端会在 metadata.user_id 中附带会话标识。', 'Some compatible clients attach a session identifier via metadata.user_id.')
        )
      ],
      extraParameterGroups: [],
      examples: responsesExamples
    },
    {
      id: 'anthropic-messages',
      badge: t('integrationDocs.sections.anthropicMessages.badge'),
      title: t('integrationDocs.sections.anthropicMessages.title'),
      description: t('integrationDocs.sections.anthropicMessages.description'),
      method: 'POST',
      endpoints: ['/v1/messages'],
      headers: anthropicHeaders,
      bullets: [
        t('integrationDocs.sections.anthropicMessages.bullets.item1'),
        t('integrationDocs.sections.anthropicMessages.bullets.item2'),
        t('integrationDocs.sections.anthropicMessages.bullets.item3')
      ],
      parameterNotice:
        docsLocale.value === 'zh'
          ? '以下字段主要对齐 Claude Messages 协议；`messages[].content` 支持纯文本或 block 数组。'
          : 'These fields follow the Claude Messages protocol; `messages[].content` may be plain text or an array of blocks.',
      params: [
        param('model', 'string', true, text('目标模型 ID。', 'Target model ID.')),
        param('max_tokens', 'integer', true, text('最大输出 token 数。', 'Maximum output tokens.')),
        param(
          'system',
          'string | array',
          false,
          text('系统提示词，可为字符串或 block 数组。', 'System prompt as a string or block array.')
        ),
        param('messages', 'array', true, text('对话消息数组。', 'Conversation message array.')),
        param(
          'messages[].role',
          'string',
          true,
          text('消息角色。', 'Message role.'),
          text('user, assistant', 'user, assistant')
        ),
        param(
          'messages[].content',
          'string | array',
          true,
          text('消息内容，可为纯文本或 block 数组。', 'Message content as plain text or a block array.')
        ),
        param(
          'messages[].content[].type',
          'string',
          false,
          text('内容块类型。', 'Content block type.'),
          text('text, image, tool_use, tool_result, thinking', 'text, image, tool_use, tool_result, thinking')
        ),
        param(
          'messages[].content[].text',
          'string',
          false,
          text('文本 block 内容。', 'Text block content.')
        ),
        param(
          'messages[].content[].thinking',
          'string',
          false,
          text('thinking block 的思考文本。', 'Thinking text inside a thinking block.')
        ),
        param(
          'messages[].content[].source.type',
          'string',
          false,
          text('图片来源类型。', 'Image source type.'),
          text('base64', 'base64')
        ),
        param(
          'messages[].content[].source.media_type',
          'string',
          false,
          text('图片 MIME 类型。', 'Image MIME type.')
        ),
        param(
          'messages[].content[].source.data',
          'string',
          false,
          text('base64 编码的图片数据。', 'Base64-encoded image data.')
        ),
        param(
          'messages[].content[].id',
          'string',
          false,
          text('tool_use block 的调用 ID。', 'Call ID for a tool_use block.')
        ),
        param(
          'messages[].content[].name',
          'string',
          false,
          text('tool_use block 的工具名。', 'Tool name for a tool_use block.')
        ),
        param(
          'messages[].content[].input',
          'object',
          false,
          text('tool_use block 的输入对象。', 'Input object for a tool_use block.')
        ),
        param(
          'messages[].content[].tool_use_id',
          'string',
          false,
          text('tool_result 对应的 tool_use_id。', 'tool_use_id referenced by a tool_result block.')
        ),
        param(
          'messages[].content[].content',
          'string | array',
          false,
          text('tool_result 的返回内容，可为字符串或 block 数组。', 'tool_result payload as a string or block array.')
        ),
        param(
          'messages[].content[].is_error',
          'boolean',
          false,
          text('tool_result 是否表示错误结果。', 'Whether a tool_result block represents an error.')
        ),
        param('tools', 'array', false, text('工具定义数组。', 'Tool definition array.')),
        param(
          'tools[].type',
          'string',
          false,
          text('工具类型。', 'Tool type.')
        ),
        param('tools[].name', 'string', false, text('工具名称。', 'Tool name.')),
        param(
          'tools[].description',
          'string',
          false,
          text('工具描述。', 'Tool description.')
        ),
        param(
          'tools[].input_schema',
          'object',
          false,
          text('工具输入 JSON Schema。', 'Tool input JSON Schema.')
        ),
        param('stream', 'boolean', false, text('是否启用流式输出。', 'Enable streaming output.')),
        param('temperature', 'number', false, text('采样温度。', 'Sampling temperature.')),
        param('top_p', 'number', false, text('核采样参数。', 'Nucleus sampling parameter.')),
        param(
          'stop_sequences',
          'string[]',
          false,
          text('停止词数组。', 'Stop sequences array.')
        ),
        param(
          'thinking.type',
          'string',
          false,
          text('思考模式。', 'Thinking mode.'),
          text('enabled, adaptive, disabled', 'enabled, adaptive, disabled')
        ),
        param(
          'thinking.budget_tokens',
          'integer',
          false,
          text('思考预算 token 数。', 'Thinking budget in tokens.')
        ),
        param(
          'tool_choice',
          'string | object',
          false,
          text('工具调度策略。', 'Tool dispatch strategy.')
        ),
        param(
          'output_config.effort',
          'string',
          false,
          text('输出生成强度。', 'Output generation effort.'),
          text('low, medium, high, max', 'low, medium, high, max')
        )
      ],
      extraParameterGroups: [],
      examples: anthropicExamples
    },
    {
      id: 'openai-images',
      badge: t('integrationDocs.sections.openaiImages.badge'),
      title: t('integrationDocs.sections.openaiImages.title'),
      description: t('integrationDocs.sections.openaiImages.description'),
      method: 'POST',
      endpoints: ['/v1/images/generations', '/v1/images/edits'],
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiImages.bullets.item1'),
        t('integrationDocs.sections.openaiImages.bullets.item2'),
        t('integrationDocs.sections.openaiImages.bullets.item3')
      ],
      parameterNotice:
        docsLocale.value === 'zh'
          ? '下表先列文生图与图生图共用字段，再补 `/v1/images/edits` 的专属输入参数。'
          : 'The first table lists fields shared by generations and edits, followed by edit-specific inputs for `/v1/images/edits`.',
      params: [
        param('model', 'string', false, text('生图模型 ID。未传时网关默认 `gpt-image-2`。', 'Image model ID. The gateway defaults to `gpt-image-2` when omitted.')),
        param('prompt', 'string', false, text('图片生成提示词。', 'Image generation prompt.')),
        param('stream', 'boolean', false, text('是否启用流式生图事件。', 'Enable streaming image-generation events.')),
        param('n', 'integer', false, text('生成图片张数。', 'Number of images to generate.'), undefined, text('必须为正整数；默认 1。', 'Must be a positive integer; defaults to 1.')),
        param(
          'size',
          'string',
          false,
          text('输出尺寸。', 'Output size.'),
          text('如 1024x1024、1536x1024、2048x2048、auto', 'For example 1024x1024, 1536x1024, 2048x2048, or auto')
        ),
        param(
          'response_format',
          'string',
          false,
          text('返回格式。', 'Response format.'),
          text('b64_json, url', 'b64_json, url')
        ),
        param(
          'quality',
          'string',
          false,
          text('图片质量档位。', 'Image quality level.')
        ),
        param(
          'background',
          'string',
          false,
          text('背景处理策略。', 'Background handling strategy.')
        ),
        param(
          'output_format',
          'string',
          false,
          text('输出图片格式。', 'Output image format.')
        ),
        param(
          'moderation',
          'string',
          false,
          text('内容审核策略。', 'Moderation strategy.')
        ),
        param(
          'input_fidelity',
          'string',
          false,
          text('图生图时的输入保真度。', 'Input fidelity for image editing.')
        ),
        param(
          'style',
          'string',
          false,
          text('图片风格档位。', 'Image style mode.')
        ),
        param(
          'output_compression',
          'integer',
          false,
          text('输出压缩率。', 'Output compression ratio.')
        ),
        param(
          'partial_images',
          'integer',
          false,
          text('是否允许分阶段返回部分图片。', 'Whether partial images may be returned during generation.')
        )
      ],
      extraParameterGroups: [
        {
          title: text('/v1/images/edits JSON 额外参数', 'Additional JSON params for /v1/images/edits'),
          description: text(
            '当你走 JSON 方式调用图生图时，当前网关接受 URL / data URL 形式的输入图片。',
            'When calling image edits with JSON, the gateway accepts URLs or data URLs as image inputs.'
          ),
          params: [
            param(
              'images[].image_url',
              'string',
              true,
              text('输入原图 URL 或 data URL。', 'Source image URL or data URL.')
            ),
            param(
              'mask.image_url',
              'string',
              false,
              text('蒙版图片 URL 或 data URL。', 'Mask image URL or data URL.')
            ),
            param(
              'images[].file_id',
              'string',
              false,
              text('当前网关不支持 file_id 方式。', 'The gateway does not support file_id input.'),
              undefined,
              text('请改用 `images[].image_url`。', 'Use `images[].image_url` instead.')
            ),
            param(
              'mask.file_id',
              'string',
              false,
              text('当前网关不支持 mask.file_id。', 'The gateway does not support mask.file_id.'),
              undefined,
              text('请改用 `mask.image_url`。', 'Use `mask.image_url` instead.')
            )
          ]
        },
        {
          title: text('/v1/images/edits multipart 额外参数', 'Additional multipart params for /v1/images/edits'),
          description: text(
            '当你使用 multipart/form-data 上传文件时，网关会读取下列字段。',
            'When using multipart/form-data uploads, the gateway reads the fields below.'
          ),
          params: [
            param(
              'image',
              'file | file[]',
              true,
              text('上传一张或多张原图文件。', 'Upload one or more source image files.')
            ),
            param(
              'mask',
              'file',
              false,
              text('上传蒙版文件。', 'Upload a mask file.')
            )
          ]
        }
      ],
      examples: imageExamples
    }
  ]
})

const compatibilityChips = computed(() => [
  'OpenAI Chat',
  'OpenAI Responses',
  'Anthropic Messages',
  'OpenAI Images'
])

const totalDocumentedParams = computed(() =>
  protocols.value.reduce((sum, section) => {
    const extraCount = section.extraParameterGroups.reduce((groupSum, group) => groupSum + group.params.length, 0)
    return sum + section.params.length + extraCount
  }, 0)
)

const heroMetrics = computed(() => [
  {
    value: String(protocols.value.length),
    label: localizeText(text('兼容协议', 'Protocols'))
  },
  {
    value: String(standardPaths.value.length),
    label: localizeText(text('标准路径', 'Standard Paths'))
  },
  {
    value: String(totalDocumentedParams.value),
    label: localizeText(text('文档参数', 'Documented Fields'))
  },
  {
    value: '4',
    label: localizeText(text('示例语言', 'Example Languages'))
  }
])

const heroProtocolCards = computed(() =>
  protocols.value.map((section) => ({
    id: section.id,
    title: section.title,
    endpoint: section.endpoints[0],
    paramCount: localizeText(text(`${section.params.length} 参数`, `${section.params.length} fields`))
  }))
)

function sectionSummaryChips(section: ProtocolSection): string[] {
  const extraParams = section.extraParameterGroups.reduce((sum, group) => sum + group.params.length, 0)
  const chips = [
    localizeText(text(`${section.params.length} 个参数`, `${section.params.length} params`)),
    localizeText(text(`${section.examples.length} 种示例`, `${section.examples.length} examples`)),
    localizeText(text(`${section.endpoints.length} 条路径`, `${section.endpoints.length} endpoints`))
  ]

  if (extraParams > 0) {
    chips.push(localizeText(text(`${extraParams} 个扩展字段`, `${extraParams} extension fields`)))
  }

  return chips
}

const pageNav = computed(() =>
  protocols.value.map((section) => ({
    id: section.id,
    label: section.title,
    meta: localizeText(text(`${section.params.length} 参数`, `${section.params.length} params`))
  }))
)

const navSectionIds = computed(() => pageNav.value.map((item) => item.id))

function cleanupSectionObserver(): void {
  if (sectionObserver) {
    sectionObserver.disconnect()
    sectionObserver = null
  }
}

function initSectionObserver(): void {
  cleanupSectionObserver()

  if (typeof window === 'undefined' || typeof IntersectionObserver === 'undefined') {
    return
  }

  const sectionElements = navSectionIds.value
    .map((id) => document.getElementById(id))
    .filter((element): element is HTMLElement => Boolean(element))

  if (!sectionElements.length) {
    return
  }

  activeSectionId.value = navSectionIds.value[0] || ''

  const visibleSections = new Set<string>()

  sectionObserver = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        const target = entry.target as HTMLElement
        if (entry.isIntersecting) {
          visibleSections.add(target.id)
        } else {
          visibleSections.delete(target.id)
        }
      }

      const orderedVisibleSections = navSectionIds.value.filter((id) => visibleSections.has(id))
      const currentSectionId = orderedVisibleSections[orderedVisibleSections.length - 1]

      if (currentSectionId) {
        activeSectionId.value = currentSectionId
      }
    },
    {
      rootMargin: '-18% 0px -62% 0px',
      threshold: [0, 0.1, 0.25, 0.4]
    }
  )

  for (const element of sectionElements) {
    sectionObserver.observe(element)
  }

  const hashSectionId = decodeURIComponent(window.location.hash.replace(/^#/, '').trim())
  if (hashSectionId && navSectionIds.value.includes(hashSectionId)) {
    activeSectionId.value = hashSectionId
  }
}

onMounted(async () => {
  appStore.fetchPublicSettings()
  await nextTick()
  initSectionObserver()
})

onBeforeUnmount(() => {
  if (copiedResetTimer) {
    clearTimeout(copiedResetTimer)
  }
  cleanupSectionObserver()
})
</script>
