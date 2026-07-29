import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import {
  AdminInstallCommand,
  calculateInstallPlatformMenuStyle,
  installCommandButtonDisabled,
  installCommandCopyOptions,
  installCommandForPlatform,
  installCommandReady,
  isCurrentInstallCommandRequest,
  shouldOpenInstallPlatformPicker,
} from './AdminInstallCommand'

describe('AdminInstallCommand state', () => {
  it('normalizes per-platform commands with Linux as the fallback', () => {
    const state = installCommandReady({
      nodeId: 'node-1',
      command: 'install-linux',
      commands: { windows: 'install-windows' },
    })

    expect(installCommandForPlatform(state, 'linux')).toBe('install-linux')
    expect(installCommandForPlatform(state, 'macos')).toBe('install-linux')
    expect(installCommandForPlatform(state, 'windows')).toBe('install-windows')
  })

  it('disables generation while blocked or loading', () => {
    expect(installCommandButtonDisabled(true, { kind: 'idle' })).toBe(true)
    expect(installCommandButtonDisabled(false, { kind: 'loading' })).toBe(true)
    expect(installCommandButtonDisabled(false, { kind: 'idle' })).toBe(false)
  })

  it('never reopens the picker while blocked and defers manual fallback until after validation', () => {
    expect(shouldOpenInstallPlatformPicker(true, false)).toBe(true)
    expect(shouldOpenInstallPlatformPicker(true, true)).toBe(false)
    expect(shouldOpenInstallPlatformPicker(false, false)).toBe(false)
    expect(installCommandCopyOptions).toEqual({ fallbackToPrompt: false })
  })

  it('accepts only the latest response for the active node', () => {
    expect(isCurrentInstallCommandRequest(3, 3, 'node-1', 'node-1')).toBe(true)
    expect(isCurrentInstallCommandRequest(2, 3, 'node-1', 'node-1')).toBe(false)
    expect(isCurrentInstallCommandRequest(3, 3, 'node-1', 'node-2')).toBe(false)
  })

  it('keeps the platform picker inside a tiny viewport', () => {
    expect(calculateInstallPlatformMenuStyle(
      { top: 70, right: 150, bottom: 94, left: 120, width: 30, height: 24 },
      { width: 160, height: 100 },
    )).toEqual({
      position: 'fixed',
      top: 12,
      left: 12,
      width: 136,
      maxHeight: 76,
      overflowY: 'auto',
    })
  })
})

describe('AdminInstallCommand', () => {
  it('uses the same Agent access section for create and edit flows', () => {
    const html = renderToStaticMarkup(
      <AdminInstallCommand
        nodeId="node-1"
        initialMessage="已添加：Alpha"
        onInstallCommand={vi.fn()}
      />,
    )

    expect(html).toContain('aria-label="Agent 接入"')
    expect(html).toContain('复制安装命令')
    expect(html).toContain('已添加：Alpha')
  })

  it('exposes blocked state through the actual button disabled attribute', () => {
    const html = renderToStaticMarkup(
      <AdminInstallCommand nodeId="node-1" blocked onInstallCommand={vi.fn()} />,
    )
    expect(html).toContain('disabled=""')
    expect(html).toContain('aria-expanded="false"')
  })
})
