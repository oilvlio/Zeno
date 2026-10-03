import { useEffect, useRef, useState, type MouseEvent } from 'react'
import { copyTextToClipboard } from './AdminInstallCommand'

// monitor-style fallback for plain-http pages: navigator.clipboard only exists
// in a secure context, while document.execCommand('copy') still works from a
// click event with the payload provided via the copy event.
type FallbackDocument = {
  addEventListener: (type: string, listener: (event: ClipboardEvent) => void) => void
  removeEventListener: (type: string, listener: (event: ClipboardEvent) => void) => void
  execCommand: (command: string) => boolean
}

export function copyIpAddressFallback(text: string, doc?: FallbackDocument): boolean {
  const target: FallbackDocument | undefined = doc ?? (
    typeof document === 'undefined' || typeof document.execCommand !== 'function'
      ? undefined
      : document as unknown as FallbackDocument
  )
  if (!target) return false
  let ok = false
  const put = (event: ClipboardEvent) => {
    event.clipboardData?.setData('text/plain', text)
    event.preventDefault()
  }
  target.addEventListener('copy', put)
  try {
    ok = target.execCommand('copy')
  } catch {
    ok = false
  }
  target.removeEventListener('copy', put)
  return ok
}

export function markIpCopied(setCopied: (value: boolean) => void, timerRef: { current: ReturnType<typeof setTimeout> | null }): void {
  setCopied(true)
  if (timerRef.current !== null) clearTimeout(timerRef.current)
  timerRef.current = setTimeout(() => {
    timerRef.current = null
    setCopied(false)
  }, 1500)
}

export function AdminIpCopyButton({ ip }: { ip: string }) {
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timerRef.current !== null) clearTimeout(timerRef.current)
  }, [])

  const handleCopy = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    void copyTextToClipboard(ip).then((nativeCopied) => {
      if (nativeCopied) {
        markIpCopied(setCopied, timerRef)
        return
      }
      if (copyIpAddressFallback(ip)) {
        markIpCopied(setCopied, timerRef)
        return
      }
      if (typeof window !== 'undefined' && typeof window.prompt === 'function') {
        window.prompt('自动复制失败，请手动复制：', ip)
      }
    })
  }

  return (
    <button
      type="button"
      className={`admin-ip-copy${copied ? ' is-copied' : ''}`}
      title={copied ? `已复制 ${ip}` : `点击复制 ${ip}`}
      aria-label={copied ? `已复制 ${ip}` : `复制 ${ip}`}
      onClick={handleCopy}
    >
      <span>{ip}</span>
    </button>
  )
}
