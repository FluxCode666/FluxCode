import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { i18n } from '@/i18n'

const { t } = i18n.global

/**
 * 检测是否支持 Clipboard API（需要安全上下文：HTTPS/localhost）
 */
function isClipboardSupported(): boolean {
  return !!(navigator.clipboard && window.isSecureContext)
}

/**
 * 降级方案：使用 textarea + execCommand
 * 使用 textarea 而非 input，以正确处理多行文本
 */
function fallbackCopy(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.cssText = 'position:fixed;left:-9999px;top:-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

interface ClipboardOptions {
  /**
   * Whether the legacy `execCommand` fallback may be used. Sensitive values
   * must keep this disabled so their plaintext never gets mounted in a DOM node.
   */
  allowFallback?: boolean
}

export function useClipboard() {
  const appStore = useAppStore()
  const copied = ref(false)

  const copyToClipboard = async (
    text: string,
    successMessage?: string,
    options: ClipboardOptions = {}
  ): Promise<boolean> => {
    if (!text) return false

    let success = false
    const allowFallback = options.allowFallback !== false

    if (isClipboardSupported()) {
      try {
        await navigator.clipboard.writeText(text)
        success = true
      } catch {
        success = allowFallback ? fallbackCopy(text) : false
      }
    } else {
      success = allowFallback ? fallbackCopy(text) : false
    }

    if (success) {
      copied.value = true
      appStore.showSuccess(successMessage || t('common.copiedToClipboard'))
      setTimeout(() => {
        copied.value = false
      }, 2000)
    } else {
      appStore.showError(t('common.copyFailed'))
    }

    return success
  }

  /**
   * Copies credentials without the textarea fallback used for ordinary text.
   * This keeps the plaintext outside the DOM even when Clipboard API access
   * is unavailable or rejected by the browser.
   */
  const copySensitiveToClipboard = (
    text: string,
    successMessage?: string
  ): Promise<boolean> => copyToClipboard(text, successMessage, { allowFallback: false })

  return { copied, copyToClipboard, copySensitiveToClipboard }
}
