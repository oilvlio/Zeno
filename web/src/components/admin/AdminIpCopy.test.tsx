import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { AdminIpCopyButton, copyIpAddressFallback } from './AdminIpCopy'

describe('copyIpAddressFallback', () => {
  it('returns false when the copy command is unavailable', () => {
    expect(copyIpAddressFallback('198.51.100.8', {
      addEventListener: () => {},
      removeEventListener: () => {},
      execCommand: () => false,
    })).toBe(false)
  })

  it('fills the clipboard from the copy event like the monitor panel', () => {
    const seen: string[] = []
    let listener: ((event: ClipboardEvent) => void) | null = null
    const fakeDocument = {
      addEventListener: vi.fn((type: string, put: (event: ClipboardEvent) => void) => {
        expect(type).toBe('copy')
        listener = put
      }),
      removeEventListener: vi.fn((type: string, put: (event: ClipboardEvent) => void) => {
        expect(type).toBe('copy')
        if (listener === put) listener = null
      }),
      execCommand: vi.fn((command: string) => {
        expect(command).toBe('copy')
        const event = {
          clipboardData: {
            setData: (_type: string, text: string) => {
              seen.push(text)
            },
          },
          preventDefault: () => {},
        } as unknown as ClipboardEvent
        listener?.(event)
        return true
      }),
    }

    expect(copyIpAddressFallback('198.51.100.8', fakeDocument)).toBe(true)
    expect(seen).toEqual(['198.51.100.8'])
    expect(fakeDocument.addEventListener).toHaveBeenCalledTimes(1)
    expect(fakeDocument.removeEventListener).toHaveBeenCalledTimes(1)
    expect(listener).toBeNull()
  })

  it('returns false when the copy command throws', () => {
    const fakeDocument = {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      execCommand: vi.fn(() => {
        throw new Error('denied')
      }),
    }

    expect(copyIpAddressFallback('198.51.100.8', fakeDocument)).toBe(false)
  })
})

describe('AdminIpCopyButton', () => {
  it('keeps the address visible while exposing a copy action', () => {
    const html = renderToStaticMarkup(<AdminIpCopyButton ip="198.51.100.8" />)

    expect(html).toContain('198.51.100.8')
    expect(html).toContain('admin-ip-copy')
    expect(html).toContain('aria-label="复制 198.51.100.8"')
    expect(html).toContain('title="点击复制 198.51.100.8"')
    expect(html).toContain('<button type="button"')
  })
})
