/**
 * Lightweight Markdown to HTML renderer
 * Supports: bold, italic, links, code, lists, paragraphs, line breaks
 * All links open in new tab with noopener noreferrer
 */

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function isSafeLink(url: string): boolean {
  const trimmed = url.trim()
  if (!trimmed) return false
  if (trimmed.startsWith('#') || trimmed.startsWith('/')) return true
  try {
    const parsed = new URL(trimmed)
    return ['http:', 'https:', 'mailto:'].includes(parsed.protocol)
  } catch {
    return false
  }
}

function renderInline(text: string): string {
  let result = escapeHtml(text)

  // Bold: **text** or __text__
  result = result.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  result = result.replace(/__(.+?)__/g, '<strong>$1</strong>')

  // Italic: *text* or _text_
  result = result.replace(/\*(.+?)\*/g, '<em>$1</em>')
  result = result.replace(/(?<!\w)_(.+?)_(?!\w)/g, '<em>$1</em>')

  // Inline code: `code`
  result = result.replace(/`(.+?)`/g, '<code>$1</code>')

  // Links: [text](url)
  result = result.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, text, url) => {
    if (!isSafeLink(url)) return text
    return `<a href="${url}" target="_blank" rel="noopener noreferrer">${text}</a>`
  })

  // Auto-linkify bare URLs
  result = result.replace(
    /(?<![="'])(https?:\/\/[^\s<]+)/g,
    '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>'
  )

  return result
}

export function renderMarkdownToHtml(markdown: string): string {
  if (!markdown) return ''

  const lines = markdown.split('\n')
  const htmlParts: string[] = []
  let inList = false
  let listType: 'ul' | 'ol' = 'ul'

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const trimmed = line.trim()

    // Empty line: close list if open, add paragraph break
    if (!trimmed) {
      if (inList) {
        htmlParts.push(listType === 'ul' ? '</ul>' : '</ol>')
        inList = false
      }
      continue
    }

    // Unordered list: - item or * item
    const ulMatch = trimmed.match(/^[-*]\s+(.+)/)
    if (ulMatch) {
      if (!inList || listType !== 'ul') {
        if (inList) htmlParts.push(listType === 'ul' ? '</ul>' : '</ol>')
        htmlParts.push('<ul>')
        inList = true
        listType = 'ul'
      }
      htmlParts.push(`<li>${renderInline(ulMatch[1])}</li>`)
      continue
    }

    // Ordered list: 1. item
    const olMatch = trimmed.match(/^\d+\.\s+(.+)/)
    if (olMatch) {
      if (!inList || listType !== 'ol') {
        if (inList) htmlParts.push(listType === 'ul' ? '</ul>' : '</ol>')
        htmlParts.push('<ol>')
        inList = true
        listType = 'ol'
      }
      htmlParts.push(`<li>${renderInline(olMatch[1])}</li>`)
      continue
    }

    // Close list if we hit a non-list line
    if (inList) {
      htmlParts.push(listType === 'ul' ? '</ul>' : '</ol>')
      inList = false
    }

    // Regular paragraph
    htmlParts.push(`<p>${renderInline(trimmed)}</p>`)
  }

  // Close any open list
  if (inList) {
    htmlParts.push(listType === 'ul' ? '</ul>' : '</ol>')
  }

  return htmlParts.join('\n')
}
