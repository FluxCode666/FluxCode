<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="pt-24">
      <section class="mx-auto max-w-6xl px-6 py-16">
        <div class="max-w-3xl">
          <h1 class="text-4xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-5xl">
            {{ t('home.sections.docsTitle') }}
          </h1>
          <p class="mt-4 text-base leading-relaxed text-gray-600 dark:text-dark-300">
            {{ t('home.sections.docsSubtitle') }}
          </p>
        </div>

        <div class="mt-12 grid gap-6 text-base">
          <div class="rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <div class="border-b border-black/5 dark:border-white/10">
              <nav class="-mb-px flex space-x-4" aria-label="Tabs">
                <button
                  v-for="tab in osTabs"
                  :key="tab.id"
                  type="button"
                  @click="activeOsTab = tab.id"
                  :class="[
                    'whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-base transition-colors',
                    activeOsTab === tab.id
                      ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                      : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
                  ]"
                >
                  {{ tab.label }}
                </button>
              </nav>
            </div>

            <p class="mt-4 text-gray-700 dark:text-dark-300">以下步骤会根据你的操作系统展示对应文档。</p>
          </div>

          <div
            class="rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40"
          >
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">1. 安装 Node.js</h3>
            <ul class="mt-4 list-disc space-y-2 pl-5 text-gray-700 dark:text-dark-300">
              <li>
                打开浏览器访问
                <a
                  class="underline underline-offset-4 hover:text-gray-900 dark:hover:text-white"
                  href="https://nodejs.org/"
                  target="_blank"
                  rel="noopener noreferrer"
                  >https://nodejs.org/</a
                >
              </li>
              <li>推荐下载 LTS 版本</li>
              <template v-if="activeOsTab === 'windows'">
                <li>下载完成后双击 .msi 文件</li>
                <li>按照安装向导完成安装，保持默认设置即可</li>
                <li>如果遇到权限问题，尝试以管理员身份运行</li>
                <li>某些杀毒软件可能会误报，需要添加白名单</li>
              </template>
              <template v-else-if="activeOsTab === 'mac'">
                <li>可直接下载 macOS 安装包并按向导安装（保持默认设置即可）</li>
                <li>也可使用 Homebrew 安装（如果你已经在用 brew 管理开发环境）</li>
              </template>
              <template v-else>
                <li>可使用系统包管理器或 nvm 安装（建议 LTS）</li>
                <li>如果遇到权限问题，检查是否需要 sudo/管理员权限</li>
              </template>
            </ul>

            <div class="mt-5 space-y-3">
              <div class="rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
                <pre class="overflow-x-auto font-mono leading-relaxed"><code>node --version</code></pre>
              </div>
              <div class="rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
                <pre class="overflow-x-auto font-mono leading-relaxed"><code>npm --version</code></pre>
              </div>
            </div>
          </div>

          <div class="rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">2. 安装 Codex CLI</h3>
            <div class="mt-4 rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
              <pre class="overflow-x-auto font-mono leading-relaxed"><code>npm install -g @openai/codex</code></pre>
            </div>
            <p class="mt-3 text-gray-700 dark:text-dark-300">
              <span v-if="activeOsTab === 'windows'">可在 CMD 或 PowerShell 中执行。安装完成后，可通过以下命令验证安装是否成功：</span>
              <span v-else>可在终端中执行。安装完成后，可通过以下命令验证安装是否成功：</span>
            </p>
            <div class="mt-4 rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
              <pre class="overflow-x-auto font-mono leading-relaxed"><code>codex --version</code></pre>
            </div>
          </div>

          <div class="rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">3. 配置 FluxCode 中转</h3>
            <div class="mt-4 rounded-2xl bg-neutral-50 p-4 text-gray-700 dark:bg-dark-950/40 dark:text-dark-300">
              <div class="font-medium text-gray-900 dark:text-white">配置路径与打开方式</div>

              <ol class="mt-3 list-decimal space-y-2 pl-5">
                <template v-if="activeOsTab === 'windows'">
                  <li>
                    切换到配置目录：终端执行
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cd %USERPROFILE%\.codex</code>
                    （如果提示不存在，可先运行一下 codex，然后新开终端即可）
                    <div class="mt-2 text-sm text-gray-600 dark:text-dark-400">
                      PowerShell 也可使用：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10"
                        >cd $env:USERPROFILE\.codex</code
                      >
                    </div>
                  </li>
                </template>
                <template v-else>
                  <li>
                    切换到配置目录：终端执行
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cd ~/.codex</code>
                    （如果提示不存在，可先运行一下 codex，然后新开终端即可）
                  </li>
                </template>

                <li>
                  打开目录：有 VS Code 执行
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">code .</code>
                  （注意空格）；有 Cursor 执行
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cursor .</code>
                  会自动打开该文件夹
                </li>
                <li>
                  编辑
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">config.toml</code>
                  与
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">auth.json</code>
                  ：复制下方内容（文件不存在就新建）
                </li>
              </ol>

              <div
                class="mt-4 rounded-2xl border border-black/5 bg-white/70 p-4 text-gray-700 dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-300"
              >
                <div class="font-medium text-gray-900 dark:text-white">Codex 中文配置（可选）</div>
                <p class="mt-2 text-gray-600 dark:text-dark-400">
                  让 Codex CLI 始终输出中文提示：在 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">.codex</code>
                  目录下创建 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">AGENTS.md</code>，写入
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">Always respond in Chinese-simplified</code>。
                </p>

                <template v-if="activeOsTab === 'windows'">
                  <ol class="mt-3 list-decimal space-y-2 pl-5">
                    <li>
                      进入配置目录：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cd %USERPROFILE%\.codex</code>
                    </li>
                    <li>
                      创建/覆盖文件：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10"
                        >echo Always respond in Chinese-simplified &gt; AGENTS.md</code
                      >
                    </li>
                  </ol>
                </template>
                <template v-else-if="activeOsTab === 'mac'">
                  <ol class="mt-3 list-decimal space-y-2 pl-5">
                    <li>
                      进入配置目录：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cd ~/.codex</code>
                    </li>
                    <li>
                      写入文件：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10"
                        >printf "Always respond in Chinese-simplified\n" &gt; AGENTS.md</code
                      >
                    </li>
                  </ol>
                </template>
                <template v-else>
                  <ol class="mt-3 list-decimal space-y-2 pl-5">
                    <li>
                      进入配置目录：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">cd ~/.codex</code>
                    </li>
                    <li>
                      如果系统未启用中文 locale，可先执行
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">sudo locale-gen zh_CN.UTF-8</code>
                    </li>
                    <li>
                      创建文件：
                      <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10"
                        >printf "Always respond in Chinese-simplified\n" &gt; AGENTS.md</code
                      >
                    </li>
                  </ol>
                </template>

                <div class="mt-3 rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
                  <pre class="overflow-x-auto font-mono leading-relaxed"><code>Always respond in Chinese-simplified</code></pre>
                </div>
                <p class="mt-3 text-sm text-gray-600 dark:text-dark-400">
                  保存后重新打开 Codex CLI 会读取该文件，始终以简体中文回应；若要恢复默认，可删除
                  <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">AGENTS.md</code>。
                </p>
              </div>
            </div>

            <div class="mt-5 space-y-4">
              <div>
                <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">config.toml 文件</div>
                <div class="rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
                  <pre class="overflow-x-auto font-mono leading-relaxed"><code v-text="configTomlContent"></code></pre>
                </div>
              </div>

              <div>
                <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">auth.json 文件</div>
                <div class="rounded-2xl bg-slate-900 p-4 text-[15px] text-slate-100">
                  <pre class="overflow-x-auto font-mono leading-relaxed"><code v-text="authJsonContent"></code></pre>
                </div>
                <p class="mt-3 text-sm text-gray-600 dark:text-dark-400">
                  提示：这里填写的是 FluxCode 的 API Key（在控制台创建/复制），字段名保持为 OPENAI_API_KEY 以兼容 Codex。
                </p>
              </div>
            </div>

            <ul class="mt-5 list-disc space-y-2 pl-5 text-gray-700 dark:text-dark-300">
              <li>model 为使用的模型，可在 CLI 或插件中再自定义</li>
              <li>model_reasoning_effort 表示推理强度，可选 high/medium/low</li>
              <li>model_provider 与 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">[model_providers.*]</code> 需与中转服务一致</li>
            </ul>
          </div>

          <div class="rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">4. 常见问题 / 排错</h3>

            <div class="mt-4 space-y-3 text-gray-700 dark:text-dark-300">
              <details class="rounded-2xl border border-black/5 bg-white/60 p-4 dark:border-white/10 dark:bg-dark-950/30">
                <summary class="cursor-pointer select-none font-medium text-gray-900 dark:text-white">
                  提示找不到 node/npm（node: command not found）
                </summary>
                <div class="mt-3 space-y-2">
                  <p>确认已安装 Node.js，并重新打开终端/命令行。</p>
                  <p>Windows 建议重启终端；macOS/Linux 检查是否把 Node 安装到了 PATH。</p>
                </div>
              </details>

              <details class="rounded-2xl border border-black/5 bg-white/60 p-4 dark:border-white/10 dark:bg-dark-950/30">
                <summary class="cursor-pointer select-none font-medium text-gray-900 dark:text-white">
                  Windows 下 npm 全局安装失败（权限/EPERM/EACCES）
                </summary>
                <div class="mt-3 space-y-2">
                  <p>优先尝试以管理员权限打开 PowerShell/CMD 后重试。</p>
                  <p>如果你不希望用管理员权限，可将 npm 全局目录配置到用户目录后再安装。</p>
                </div>
              </details>

              <details class="rounded-2xl border border-black/5 bg-white/60 p-4 dark:border-white/10 dark:bg-dark-950/30">
                <summary class="cursor-pointer select-none font-medium text-gray-900 dark:text-white">
                  找不到配置目录/文件（~/.codex 不存在）
                </summary>
                <div class="mt-3 space-y-2">
                  <p>
                    先运行一次 codex（例如
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">codex --version</code>），再重新打开终端。
                  </p>
                  <p>文件不存在可以手动创建：<code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">config.toml</code> 与 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">auth.json</code>。</p>
                </div>
              </details>

              <details class="rounded-2xl border border-black/5 bg-white/60 p-4 dark:border-white/10 dark:bg-dark-950/30">
                <summary class="cursor-pointer select-none font-medium text-gray-900 dark:text-white">报 401/Unauthorized</summary>
                <div class="mt-3 space-y-2">
                  <p>检查
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">auth.json</code> 
                    中的 key 是否为 FluxCode 控制台生成的 API Key。
                  </p>
                  <p>检查
                    <code  class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">config.toml</code>
                     中的 
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">base_url</code> 
                    是否指向 FluxCode 官网地址
                  </p>
                </div>
              </details>

              <details class="rounded-2xl border border-black/5 bg-white/60 p-4 dark:border-white/10 dark:bg-dark-950/30">
                <summary class="cursor-pointer select-none font-medium text-gray-900 dark:text-white">
                  model_provider / [model_providers.*] 不匹配
                </summary>
                <div class="mt-3 space-y-2">
                  <p>
                    确保 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">model_provider</code> 与
                    <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">[model_providers.&lt;同名&gt;]</code>
                    的名字一致（这里建议使用 <code class="rounded bg-black/5 px-1 py-0.5 font-mono text-[0.9em] dark:bg-white/10">fluxcode</code>）。
                  </p>
                </div>
              </details>
            </div>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import { resolveOpenAIUseKeyModelId } from '@/utils/openaiUseKeyModel'

const { t } = useI18n()

const appStore = useAppStore()

// Site settings
const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')

type OsTabId = 'windows' | 'mac' | 'linux'
const osTabs = [
  { id: 'windows' as const, label: 'Windows' },
  { id: 'mac' as const, label: 'macOS' },
  { id: 'linux' as const, label: 'Linux' }
]
const activeOsTab = ref<OsTabId>('windows')

const suggestedBaseUrl = computed(() => {
  const raw = (appStore.apiBaseUrl || '').trim()
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const base = (raw || origin).replace(/\/api\/v1\/?$/, '')
  return base.replace(/\/+$/, '')
})

const suggestedModelId = computed(() => resolveOpenAIUseKeyModelId(appStore.openaiUseKeyModelId))

const configTomlContent = computed(() => `model_provider = "fluxcode"
model = "${suggestedModelId.value}"
model_reasoning_effort = "medium"

[model_providers.fluxcode]
name = "fluxcode"
base_url = "${suggestedBaseUrl.value}"
wire_api = "responses"
requires_openai_auth = true`)

const authJsonContent = `{
  "OPENAI_API_KEY": "粘贴你的 API 密钥"
}`

onMounted(() => {
  appStore.fetchPublicSettings()
  const ua = navigator.userAgent || ''
  if (/Macintosh|Mac OS X/i.test(ua)) activeOsTab.value = 'mac'
  else if (/Linux/i.test(ua)) activeOsTab.value = 'linux'
  else if (/Windows/i.test(ua)) activeOsTab.value = 'windows'
})
</script>
