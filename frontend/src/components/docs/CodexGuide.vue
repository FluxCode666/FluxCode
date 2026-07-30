<template>
  <article class="space-y-6" aria-labelledby="codex-title">
    <header class="overflow-hidden rounded-[28px] border border-sky-900/15 bg-[radial-gradient(circle_at_top_right,_rgba(14,165,233,0.18),_transparent_34%),linear-gradient(135deg,_#eff6ff,_#ffffff_58%,_#f0fdfa)] p-6 dark:border-sky-400/10 dark:bg-[radial-gradient(circle_at_top_right,_rgba(14,165,233,0.17),_transparent_34%),linear-gradient(135deg,_rgba(12,74,110,0.35),_rgba(17,24,39,0.72))] sm:p-8">
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-full border border-sky-700/15 bg-white/75 px-3 py-1 text-xs font-semibold tracking-wide text-sky-800 dark:border-sky-200/15 dark:bg-sky-950/50 dark:text-sky-200">OPENAI CODEX</span>
        <span class="rounded-full border border-teal-700/15 bg-teal-50/75 px-3 py-1 text-xs font-medium text-teal-800 dark:border-teal-200/15 dark:bg-teal-950/40 dark:text-teal-200">Responses API</span>
      </div>
      <h2 id="codex-title" class="mt-5 text-3xl font-semibold tracking-tight text-slate-950 dark:text-white sm:text-4xl">将 Codex CLI 接入 {{ siteName }}</h2>
      <p class="mt-3 max-w-2xl text-sm leading-7 text-slate-600 dark:text-slate-300 sm:text-base">
        在本地写入 Codex 的 <code class="rounded bg-sky-950/5 px-1.5 py-0.5 font-mono text-[0.9em] text-sky-900 dark:bg-white/10 dark:text-sky-100">config.toml</code> 与 <code class="rounded bg-sky-950/5 px-1.5 py-0.5 font-mono text-[0.9em] text-sky-900 dark:bg-white/10 dark:text-sky-100">auth.json</code>，即可让 Codex 使用当前站点的网关与 API Key。
      </p>

      <div class="mt-6 grid gap-3 sm:grid-cols-2">
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">Base URL</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">{{ gatewayBaseUrl }}</code>
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
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">安装 Node.js 与 Codex CLI</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">请先安装 Node.js LTS 版本。安装完成后重新打开终端，再执行下列命令。</p>
          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" :code="nodeVersionCommand" />
            <CodeBlock label="terminal" code="npm install -g @openai/codex" />
          </div>
          <div class="mt-4"><CodeBlock label="terminal" code="codex --version" /></div>
          <p class="mt-4 text-sm leading-6 text-slate-500 dark:text-slate-400">
            未安装 Node.js 时，请从 <a class="font-medium underline underline-offset-4" href="https://nodejs.org/" target="_blank" rel="noopener noreferrer">Node.js 官网</a> 安装 LTS 版本。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">02</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">打开 Codex 配置目录</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">首次执行一次 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">codex</code> 后，配置目录通常会自动创建。</p>

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

          <div class="mt-4"><CodeBlock :label="activeOsTab === 'windows' ? 'PowerShell' : 'terminal'" :code="configDirectoryCommand" /></div>
          <p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">可使用 <code class="font-mono">code .</code> 或 <code class="font-mono">cursor .</code> 在编辑器中打开该目录。</p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">03</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">写入网关与 API Key</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">在配置目录中创建或覆盖以下两个文件。请仅在本机保存 API Key，不要提交到 Git 仓库。</p>

          <div class="mt-5 space-y-4">
            <CodeBlock label="config.toml" :code="configTomlContent" />
            <CodeBlock label="auth.json" :code="authJsonContent" />
          </div>

          <ul class="mt-5 list-disc space-y-2 pl-5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            <li><code class="font-mono">model</code> 可改为当前 API Key 有权限调用的任意模型。</li>
            <li><code class="font-mono">model_reasoning_effort</code> 可按模型能力改为 <code class="font-mono">low</code>、<code class="font-mono">medium</code> 或 <code class="font-mono">high</code>。</li>
            <li><code class="font-mono">model_provider</code> 的名称必须与 <code class="font-mono">[model_providers.*]</code> 小节保持一致。</li>
          </ul>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <h3 class="text-lg font-semibold text-slate-950 dark:text-white">常见排查</h3>
      <div class="mt-4 divide-y divide-black/5 dark:divide-white/10">
        <details class="py-4 first:pt-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">找不到 node、npm 或 codex 命令</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">确认 Node.js 已安装，并关闭后重新打开终端；Windows 还可尝试使用管理员权限打开 PowerShell。</p>
        </details>
        <details class="py-4">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">收到 401 / Unauthorized</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">检查 <code class="font-mono">auth.json</code> 中的 Key 是否来自 {{ siteName }} 控制台，以及 <code class="font-mono">base_url</code> 是否为 <code class="font-mono">{{ gatewayBaseUrl }}</code>。</p>
        </details>
        <details class="py-4 last:pb-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">provider 配置不匹配</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">确保 <code class="font-mono">model_provider = "fluxcode"</code> 与 <code class="font-mono">[model_providers.fluxcode]</code> 完全同名。</p>
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

type OsTabId = 'windows' | 'mac' | 'linux'
const osTabs = [
  { id: 'windows' as const, label: 'Windows' },
  { id: 'mac' as const, label: 'macOS' },
  { id: 'linux' as const, label: 'Linux' }
]
const activeOsTab = ref<OsTabId>('windows')
const nodeVersionCommand = `node --version
npm --version`

const configDirectoryCommand = computed(() => {
  if (activeOsTab.value === 'windows') return 'cd $env:USERPROFILE\\.codex'
  return 'cd ~/.codex'
})

const configTomlContent = computed(() => `model_provider = "fluxcode"
model = "${props.suggestedModelId}"
model_reasoning_effort = "medium"

[model_providers.fluxcode]
name = "${props.siteName}"
base_url = "${props.gatewayBaseUrl}"
wire_api = "responses"
requires_openai_auth = true`)

const authJsonContent = JSON.stringify(
  {
    OPENAI_API_KEY: '粘贴你的 API 密钥'
  },
  null,
  2
)
</script>
