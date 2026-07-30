<template>
  <article class="space-y-6" aria-labelledby="claude-code-title">
    <header class="overflow-hidden rounded-[28px] border border-orange-900/15 bg-[radial-gradient(circle_at_top_right,_rgba(249,115,22,0.16),_transparent_34%),linear-gradient(135deg,_#fff7ed,_#ffffff_58%,_#fefce8)] p-6 dark:border-orange-400/10 dark:bg-[radial-gradient(circle_at_top_right,_rgba(249,115,22,0.17),_transparent_34%),linear-gradient(135deg,_rgba(124,45,18,0.36),_rgba(17,24,39,0.72))] sm:p-8">
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-full border border-orange-700/15 bg-white/75 px-3 py-1 text-xs font-semibold tracking-wide text-orange-800 dark:border-orange-200/15 dark:bg-orange-950/50 dark:text-orange-200">CLAUDE CODE</span>
        <span class="rounded-full border border-amber-700/15 bg-amber-50/75 px-3 py-1 text-xs font-medium text-amber-800 dark:border-amber-200/15 dark:bg-amber-950/40 dark:text-amber-200">Anthropic Messages</span>
      </div>
      <h2 id="claude-code-title" class="mt-5 text-3xl font-semibold tracking-tight text-slate-950 dark:text-white sm:text-4xl">将 Claude Code CLI 接入 {{ siteName }}</h2>
      <p class="mt-3 max-w-2xl text-sm leading-7 text-slate-600 dark:text-slate-300 sm:text-base">
        通过 Claude Code 的环境变量配置网关与 API Key。CLI 会将请求发送到 <code class="rounded bg-orange-950/5 px-1.5 py-0.5 font-mono text-[0.9em] text-orange-900 dark:bg-white/10 dark:text-orange-100">/v1/messages</code>，无需登录 Anthropic 账号。
      </p>

      <div class="mt-6 grid gap-3 sm:grid-cols-2">
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">网关根地址</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">{{ gatewayBaseUrl }}</code>
        </div>
        <div class="rounded-2xl border border-black/5 bg-white/70 p-4 dark:border-white/10 dark:bg-slate-950/30">
          <div class="text-xs font-medium uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">实际请求路径</div>
          <code class="mt-2 block break-all font-mono text-sm font-semibold text-slate-900 dark:text-white">{{ apiEndpoint }}/messages</code>
        </div>
      </div>
    </header>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">01</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">安装 Claude Code CLI</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">请先安装 Node.js LTS。安装完成后重新打开终端，再安装 Claude Code。</p>
          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" :code="nodeVersionCommand" />
            <CodeBlock label="terminal" code="npm install -g @anthropic-ai/claude-code" />
          </div>
          <div class="mt-4"><CodeBlock label="terminal" code="claude --version" /></div>
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
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">在当前终端设置网关与 API Key</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            先在 {{ siteName }} 控制台创建 API Key，再执行对应系统的命令。命令只对当前终端会话生效，关闭终端后需要重新设置或继续下一步写入配置文件。
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

          <div class="mt-4"><CodeBlock :label="activeOsTab === 'powershell' ? 'PowerShell' : activeOsTab === 'cmd' ? 'Command Prompt' : 'terminal'" :code="environmentCommand" /></div>
          <p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">
            请将示例中的 <code class="font-mono">粘贴你的 API Key</code> 替换为真实密钥；不要把真实 Key 写入项目文件或提交到 Git。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">03</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">可选：保存为 Claude Code 全局配置</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">
            希望后续终端自动生效时，将以下 <code class="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[0.9em] dark:bg-white/10">env</code> 字段合并到 {{ settingsPath }}。若文件已有其他设置，请保留原有内容。
          </p>
          <div class="mt-5"><CodeBlock :label="settingsPath" :code="settingsJson" /></div>
          <p class="mt-4 rounded-xl bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900 dark:bg-amber-400/10 dark:text-amber-100">
            <code class="font-mono">ANTHROPIC_BASE_URL</code> 必须使用上方的网关根地址，不能额外加 <code class="font-mono">/v1</code>；Claude Code 会自行补全 <code class="font-mono">/v1/messages</code>。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <div class="flex items-start gap-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-900 text-sm font-bold text-white dark:bg-white dark:text-slate-900">04</div>
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-slate-950 dark:text-white">启动并确认连接</h3>
          <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">在任意代码仓库中运行 Claude Code。首次启动后直接输入任务即可开始使用。</p>
          <div class="mt-5 grid gap-4 xl:grid-cols-2">
            <CodeBlock label="terminal" code="claude" />
            <CodeBlock label="terminal" code="claude -p &quot;请用一句话确认当前连接可用。&quot;" />
          </div>
          <p class="mt-4 text-sm leading-6 text-slate-500 dark:text-slate-400">
            若需指定模型，可在启动时追加 <code class="font-mono">--model &lt;模型 ID&gt;</code>。该模型必须属于当前 API Key 可调用的 Anthropic Messages 模型；不要使用页面中用于 OpenAI 客户端的默认模型 ID。
          </p>
        </div>
      </div>
    </section>

    <section class="rounded-[24px] border border-black/5 bg-white/75 p-6 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
      <h3 class="text-lg font-semibold text-slate-950 dark:text-white">常见排查</h3>
      <div class="mt-4 divide-y divide-black/5 dark:divide-white/10">
        <details class="py-4 first:pt-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">出现 401、Unauthorized 或要求登录</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">确认当前终端已设置 <code class="font-mono">ANTHROPIC_AUTH_TOKEN</code>，并检查 Key 是否来自 {{ siteName }}。保存配置后请重新打开终端再试。</p>
        </details>
        <details class="py-4">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">返回 /v1/messages 不可用或分组不支持</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">Claude Code 只使用 <code class="font-mono">/v1/messages</code>。请使用 Claude/Anthropic 分组的 API Key；若使用 OpenAI 分组，管理员需要启用该分组的 Messages 调度能力。</p>
        </details>
        <details class="py-4 last:pb-0">
          <summary class="cursor-pointer list-none pr-8 text-sm font-medium text-slate-900 marker:hidden dark:text-white">请求地址变成 /v1/v1/messages</summary>
          <p class="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">说明 <code class="font-mono">ANTHROPIC_BASE_URL</code> 误填了 <code class="font-mono">/v1</code>。请改为 <code class="font-mono">{{ gatewayBaseUrl }}</code>。</p>
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

type OsTabId = 'unix' | 'cmd' | 'powershell'

const osTabs = [
  { id: 'unix' as const, label: 'macOS / Linux' },
  { id: 'cmd' as const, label: 'Windows CMD' },
  { id: 'powershell' as const, label: 'PowerShell' }
]
const activeOsTab = ref<OsTabId>('unix')
const nodeVersionCommand = `node --version
npm --version`

const environmentCommand = computed(() => {
  if (activeOsTab.value === 'cmd') {
    return `set ANTHROPIC_BASE_URL=${props.gatewayBaseUrl}
set ANTHROPIC_AUTH_TOKEN=粘贴你的 API Key
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
  }

  if (activeOsTab.value === 'powershell') {
    return `$env:ANTHROPIC_BASE_URL="${props.gatewayBaseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="粘贴你的 API Key"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
  }

  return `export ANTHROPIC_BASE_URL="${props.gatewayBaseUrl}"
export ANTHROPIC_AUTH_TOKEN="粘贴你的 API Key"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
})

const settingsPath = computed(() => activeOsTab.value === 'unix' ? '~/.claude/settings.json' : '%USERPROFILE%\\.claude\\settings.json')

const settingsJson = computed(() =>
  JSON.stringify(
    {
      env: {
        ANTHROPIC_BASE_URL: props.gatewayBaseUrl,
        ANTHROPIC_AUTH_TOKEN: '粘贴你的 API Key',
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
        CLAUDE_CODE_ATTRIBUTION_HEADER: '0'
      }
    },
    null,
    2
  )
)
</script>
