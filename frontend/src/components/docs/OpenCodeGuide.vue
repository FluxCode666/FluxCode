<template>
  <article class="space-y-6" aria-labelledby="opencode-title">
    <header class="overflow-hidden rounded-[28px] border border-violet-900/15 bg-[radial-gradient(circle_at_top_right,_rgba(139,92,246,0.18),_transparent_34%),linear-gradient(135deg,_#f5f3ff,_#ffffff_58%,_#eef2ff)] p-6 dark:border-violet-400/10 dark:bg-[radial-gradient(circle_at_top_right,_rgba(139,92,246,0.17),_transparent_34%),linear-gradient(135deg,_rgba(76,29,149,0.36),_rgba(17,24,39,0.72))] sm:p-8">
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-full border border-violet-700/15 bg-white/75 px-3 py-1 text-xs font-semibold tracking-wide text-violet-800 dark:border-violet-200/15 dark:bg-violet-950/50 dark:text-violet-200">OPENCODE</span>
        <span class="rounded-full border border-indigo-700/15 bg-indigo-50/75 px-3 py-1 text-xs font-medium text-indigo-800 dark:border-indigo-200/15 dark:bg-indigo-950/40 dark:text-indigo-200">OpenAI Compatible</span>
      </div>
      <h2 id="opencode-title" class="mt-5 text-3xl font-semibold tracking-tight text-slate-950 dark:text-white sm:text-4xl">将 OpenCode 接入 {{ siteName }}</h2>
      <p class="mt-3 max-w-2xl text-sm leading-7 text-slate-600 dark:text-slate-300 sm:text-base">
        在 OpenCode 的全局配置中添加 OpenAI Compatible provider，即可通过当前站点的 API 网关使用 Coding Agent。本教程使用环境变量保存 API Key，不会将密钥写入 <code class="rounded bg-violet-950/5 px-1.5 py-0.5 font-mono text-[0.9em] text-violet-900 dark:bg-white/10 dark:text-violet-100">opencode.json</code>。
      </p>

      <div class="mt-6 grid gap-3 sm:grid-cols-3">
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">配置文件</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">opencode.json</code>
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
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">安装 OpenCode</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">请先安装 Node.js LTS，然后在终端中安装 OpenCode CLI。</p>
          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" :code="nodeVersionCommand" />
            <CodeBlock label="terminal" code="npm install -g opencode-ai@latest" />
          </div>
          <div class="mt-4"><CodeBlock label="terminal" code="opencode --version" /></div>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">02</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">设置 API Key 环境变量</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">将控制台创建的 API Key 保存到 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">FLUXCODE_API_KEY</code>，配置文件会在运行时引用它。</p>

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

          <div class="mt-4"><CodeBlock :label="activeOsTab === 'windows' ? 'PowerShell' : 'terminal'" :code="environmentCommand" /></div>
          <p v-if="activeOsTab === 'windows'" class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">上述命令仅对当前 PowerShell 生效。若要长期保存，可执行 <code class="font-mono">setx FLUXCODE_API_KEY &quot;sk-...&quot;</code>，然后重新打开终端。</p>
          <p v-else class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">若要长期保存，请按所用 shell（如 zsh 或 bash）写入对应的配置文件，并妥善保护该文件。</p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">03</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">创建 OpenCode 全局配置</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">推荐使用全局配置，这样所有项目都会默认使用 {{ siteName }}。也可以将同样内容保存为项目根目录的 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">opencode.json</code>。</p>

          <div class="mt-5"><CodeBlock :label="activeOsTab === 'windows' ? 'PowerShell' : 'terminal'" :code="createConfigCommand" /></div>
          <div class="mt-4"><CodeBlock :label="configPath" :code="opencodeConfig" /></div>
          <p class="mt-4 rounded-xl bg-indigo-50 px-4 py-3 text-sm leading-6 text-indigo-900 dark:bg-indigo-400/10 dark:text-indigo-100">
            <code class="font-mono">baseURL</code> 必须包含 <code class="font-mono">/v1</code>，即 <code class="font-mono">{{ apiEndpoint }}</code>。这是 OpenAI Compatible provider 的接口根路径。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">04</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">启动 OpenCode</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">进入任意代码仓库后运行以下命令。OpenCode 会加载刚刚配置的默认模型。</p>
          <div class="mt-5"><CodeBlock label="terminal" code="opencode" /></div>
          <ul class="mt-5 list-disc space-y-2 pl-5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            <li>默认模型为 <code class="font-mono">openai/{{ suggestedModelId }}</code>，可在 OpenCode 的模型选择器中切换为当前 API Key 有权限调用的其他模型。</li>
            <li>本教程使用 OpenAI Compatible 接口；请使用可调用该接口的 API Key。</li>
            <li>若 API Key 仅支持 Anthropic Messages，请使用 Claude Code 教程，或在密钥页的 OpenCode 配置生成器中选择对应平台。</li>
          </ul>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <h3 class="text-lg font-semibold text-slate-950 dark:text-white">常见排查</h3>
      <div class="mt-4 divide-y divide-black/5 dark:divide-white/10">
        <details class="py-4 first:pt-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">返回 401 或 Unauthorized</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">确认启动 OpenCode 的终端已设置 <code class="font-mono">FLUXCODE_API_KEY</code>；使用 <code class="font-mono">setx</code> 后需要关闭并重新打开终端。</p>
        </details>
        <details class="py-4">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">看不到默认模型或模型不可用</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">检查模型 ID 是否属于当前 API Key 的可用模型，并在 OpenCode 中选择其他已授权模型。配置文件中的 provider ID 与模型前缀都应保持 <code class="font-mono">openai</code>。</p>
        </details>
        <details class="py-4 last:pb-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">请求地址包含重复的 /v1</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">请将 <code class="font-mono">baseURL</code> 恢复为 <code class="font-mono">{{ apiEndpoint }}</code>，不要写成 <code class="font-mono">{{ apiEndpoint }}/v1</code>。</p>
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

type OsTabId = 'unix' | 'windows'

const osTabs = [
  { id: 'unix' as const, label: 'macOS / Linux' },
  { id: 'windows' as const, label: 'Windows' }
]
const activeOsTab = ref<OsTabId>('unix')
const nodeVersionCommand = `node --version
npm --version`

const environmentCommand = computed(() =>
  activeOsTab.value === 'windows'
    ? '$env:FLUXCODE_API_KEY = "sk-粘贴你的 API Key"'
    : "export FLUXCODE_API_KEY='sk-粘贴你的 API Key'"
)

const configPath = computed(() => activeOsTab.value === 'windows' ? '%USERPROFILE%\\.config\\opencode\\opencode.json' : '~/.config/opencode/opencode.json')

const createConfigCommand = computed(() =>
  activeOsTab.value === 'windows'
    ? "$configDir = Join-Path $env:USERPROFILE '.config\\opencode'\nNew-Item -ItemType Directory -Force -Path $configDir | Out-Null\nNew-Item -ItemType File -Force -Path (Join-Path $configDir 'opencode.json') | Out-Null"
    : 'mkdir -p ~/.config/opencode\ntouch ~/.config/opencode/opencode.json'
)

const opencodeConfig = computed(() =>
  JSON.stringify(
    {
      $schema: 'https://opencode.ai/config.json',
      model: `openai/${props.suggestedModelId}`,
      provider: {
        openai: {
          options: {
            baseURL: props.apiEndpoint,
            apiKey: '{env:FLUXCODE_API_KEY}'
          },
          models: {
            [props.suggestedModelId]: {
              name: props.suggestedModelId,
              options: {
                store: false
              }
            }
          }
        }
      }
    },
    null,
    2
  )
)
</script>
