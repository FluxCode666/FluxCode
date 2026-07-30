<template>
  <article class="space-y-6" aria-labelledby="pi-agent-title">
    <header class="overflow-hidden rounded-[28px] border border-emerald-900/15 bg-[radial-gradient(circle_at_top_right,_rgba(16,185,129,0.16),_transparent_34%),linear-gradient(135deg,_#f0fdf4,_#ffffff_58%,_#ecfeff)] p-6 dark:border-emerald-400/10 dark:bg-[radial-gradient(circle_at_top_right,_rgba(16,185,129,0.17),_transparent_34%),linear-gradient(135deg,_rgba(6,78,59,0.38),_rgba(17,24,39,0.72))] sm:p-8">
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-full border border-emerald-700/15 bg-white/75 px-3 py-1 text-xs font-semibold tracking-wide text-emerald-800 dark:border-emerald-200/15 dark:bg-emerald-950/50 dark:text-emerald-200">PI AGENT</span>
        <span class="rounded-full border border-cyan-700/15 bg-cyan-50/75 px-3 py-1 text-xs font-medium text-cyan-800 dark:border-cyan-200/15 dark:bg-cyan-950/40 dark:text-cyan-200">OpenAI Chat Completions</span>
      </div>
      <h2 id="pi-agent-title" class="mt-5 text-3xl font-semibold tracking-tight text-slate-950 dark:text-white sm:text-4xl">将 Pi Agent 接入 {{ siteName }}</h2>
      <p class="mt-3 max-w-2xl text-sm leading-7 text-slate-600 dark:text-slate-300 sm:text-base">
        Pi 是运行在终端中的轻量 Coding Agent。本指南采用环境变量保存 API Key，并通过 <code class="rounded bg-emerald-950/5 px-1.5 py-0.5 font-mono text-[0.9em] text-emerald-900 dark:bg-white/10 dark:text-emerald-100">models.json</code> 接入当前站点的 OpenAI 兼容接口。
      </p>

      <div class="mt-6 grid gap-3 sm:grid-cols-3">
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">配置文件</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">~/.pi/agent/models.json</code>
        </div>
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">Base URL</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">{{ apiEndpoint }}</code>
        </div>
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">默认模型</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">{{ suggestedModelId }}</code>
        </div>
      </div>
    </header>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">01</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">准备 Node.js 并安装 Pi</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">Pi 当前推荐使用 Node.js 22.19.0 或更高版本。先在终端确认 Node.js 版本，再安装 CLI。</p>

          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" :code="nodeVersionCommand" />
            <CodeBlock label="terminal" :code="installCommand" />
          </div>
          <div class="mt-4">
            <CodeBlock label="terminal" code="pi --version" />
          </div>

          <p class="mt-4 rounded-xl bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900 dark:bg-amber-400/10 dark:text-amber-100">
            如果当前机器还没有 Node.js，请从 <a class="font-medium underline underline-offset-4" href="https://nodejs.org/" target="_blank" rel="noopener noreferrer">Node.js 官网</a> 安装最新 LTS 版本，然后重新打开终端。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">02</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">设置 API Key 环境变量</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            先在控制台创建 API Key，再把它保存为当前终端的 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">FLUXCODE_API_KEY</code>。这样 Key 不会以明文写入配置文件，也不容易误提交到仓库。
          </p>

          <div class="mt-5 flex flex-wrap gap-2" role="tablist" aria-label="选择操作系统">
            <button
              v-for="tab in osTabs"
              :key="tab.id"
              type="button"
              role="tab"
              :aria-selected="activeOsTab === tab.id"
              :class="[
                'rounded-full px-3.5 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400',
                activeOsTab === tab.id
                  ? 'bg-slate-900 text-white shadow-sm dark:bg-white dark:text-slate-900'
                  : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-white/5 dark:text-slate-300 dark:hover:bg-white/10'
              ]"
              @click="activeOsTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>

          <div class="mt-4">
            <CodeBlock :label="activeOsTab === 'windows' ? 'PowerShell' : 'terminal'" :code="environmentCommand" />
          </div>

          <p v-if="activeOsTab === 'windows'" class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">
            需要持久化到后续终端时，执行 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">setx FLUXCODE_API_KEY "sk-..."</code>，然后关闭并重新打开终端。
          </p>
          <p v-else class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">
            上述命令仅对当前终端会话生效。若需长期保存，请按你使用的 shell（例如 zsh 或 bash）写入对应的配置文件，并妥善保护该文件。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">03</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">创建 <code class="font-mono text-[0.92em]">models.json</code></h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">Pi 会从用户目录的配置文件读取自定义模型供应商。</p>

          <div class="mt-5 grid gap-4 lg:grid-cols-2">
            <div class="rounded-2xl border border-black/5 bg-slate-50 p-4 dark:border-white/10 dark:bg-dark-950/35">
              <p class="text-sm font-semibold text-slate-900 dark:text-white">Windows</p>
              <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">在文件资源管理器地址栏打开：</p>
              <code class="mt-3 block break-all rounded-lg bg-white px-3 py-2 font-mono text-xs text-slate-800 ring-1 ring-black/5 dark:bg-white/5 dark:text-slate-100 dark:ring-white/10">C:\\Users\\&lt;你的用户名&gt;\\.pi\\agent\\</code>
              <p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">新建 <code class="font-mono">models.json</code>；请确认文件名没有保留 <code class="font-mono">.txt</code> 后缀。</p>
            </div>
            <div class="rounded-2xl border border-black/5 bg-slate-50 p-4 dark:border-white/10 dark:bg-dark-950/35">
              <p class="text-sm font-semibold text-slate-900 dark:text-white">macOS / Linux</p>
              <div class="mt-3">
                <CodeBlock label="terminal" :code="createConfigCommand" />
              </div>
            </div>
          </div>

          <div class="mt-5">
            <CodeBlock label="~/.pi/agent/models.json" :code="modelsJson" />
          </div>

          <p class="mt-4 rounded-xl bg-cyan-50 px-4 py-3 text-sm leading-6 text-cyan-900 dark:bg-cyan-400/10 dark:text-cyan-100">
            模型 ID 必须是当前 API Key 可用的模型。模板只提供最小字段；Pi 默认按文本输入、128K 上下文和 16K 最大输出处理。若要声明图片、推理或更准确的 token 上限，请根据该模型的实际能力补充字段。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">04</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">选择模型并开始使用</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">打开 Pi 后，使用 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">/model</code> 选择刚刚配置的模型。</p>
          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" code="pi" />
            <CodeBlock label="Pi 内部命令" code="/model" />
          </div>
          <ol class="mt-5 list-decimal space-y-2 pl-5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            <li>在模型列表中选择 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">{{ suggestedModelId }}</code> 或你账户中其他可用模型。</li>
            <li>按回车确认；模型 ID 会显示在 Pi 的状态栏中。</li>
            <li>直接描述任务即可开始编码。修改 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">models.json</code> 后再次打开 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">/model</code>，Pi 会重新读取配置，无需重启。</li>
          </ol>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <h3 class="text-lg font-semibold text-slate-950 dark:text-white">常见排查</h3>
      <div class="mt-4 divide-y divide-black/5 dark:divide-white/10">
        <details class="group py-4 first:pt-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">在 <code class="font-mono text-[0.92em]">/model</code> 中看不到模型</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">确认环境变量已在当前终端设置、JSON 格式合法，并且模型 ID 对应当前 API Key 可用的模型。重新进入 <code class="font-mono">/model</code> 会触发配置重载。</p>
        </details>
        <details class="group py-4">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">返回 401 或 Unauthorized</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">检查 <code class="font-mono">FLUXCODE_API_KEY</code> 的值、终端是否已重开，以及 <code class="font-mono">baseUrl</code> 是否为 <code class="font-mono">{{ apiEndpoint }}</code>。</p>
        </details>
        <details class="group py-4 last:pb-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">网关不兼容 developer role 或 reasoning 参数</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">仅当服务端明确报出相关兼容错误时，才在 provider 内增加 <code class="font-mono">compat.supportsDeveloperRole</code> 或 <code class="font-mono">compat.supportsReasoningEffort</code> 配置；不要默认开启，以免错误限制具备完整能力的模型。</p>
        </details>
      </div>
    </section>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import CodeBlock from './CodeBlock.vue'

const props = defineProps<{
  siteName: string
  gatewayBaseUrl: string
  apiEndpoint: string
  suggestedModelId: string
}>()

type OsTabId = 'windows' | 'unix'

const osTabs = [
  { id: 'windows' as const, label: 'Windows' },
  { id: 'unix' as const, label: 'macOS / Linux' }
]
const activeOsTab = ref<OsTabId>('windows')

const installCommand = 'npm install -g --ignore-scripts @earendil-works/pi-coding-agent'
const nodeVersionCommand = `node --version
npm --version`
const createConfigCommand = `mkdir -p ~/.pi/agent
touch ~/.pi/agent/models.json`

const environmentCommand = computed(() =>
  activeOsTab.value === 'windows'
    ? "$env:FLUXCODE_API_KEY = 'sk-粘贴你的 API Key'"
    : "export FLUXCODE_API_KEY='sk-粘贴你的 API Key'"
)

const modelsJson = computed(() =>
  JSON.stringify(
    {
      providers: {
        fluxcode: {
          name: props.siteName,
          baseUrl: props.apiEndpoint,
          api: 'openai-completions',
          apiKey: '$FLUXCODE_API_KEY',
          models: [
            {
              id: props.suggestedModelId,
              name: props.suggestedModelId
            }
          ]
        }
      }
    },
    null,
    2
  )
)
</script>
