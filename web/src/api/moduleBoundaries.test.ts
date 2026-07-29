// @ts-nocheck
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const apiDirectory = dirname(fileURLToPath(import.meta.url))
const sourceDirectory = join(apiDirectory, '..')
const readSource = (path: string) => readFileSync(join(sourceDirectory, path), 'utf8')

describe('API module boundaries', () => {
  it('keeps public application code independent from the admin API client', () => {
    const app = readSource('App.tsx')
    const publicClient = readSource('api/publicClient.ts')
    const publicControllers = [
      readSource('hooks/useSummaryController.ts'),
      readSource('hooks/useNodeDetailController.ts'),
      readSource('hooks/useServiceDetailController.ts'),
    ].join('\n')
    expect(publicControllers).toContain("from '../api/publicClient'")
    expect(publicControllers).not.toContain("from '../api/adminClient'")
    expect(app).not.toContain("from './api/client'")
    expect(app).not.toContain('useAdminController')
    expect(readSource('components/admin/AdminDashboard.tsx')).toContain('useAdminController')
    expect(publicClient).not.toContain('/api/admin/')
    expect(publicClient).not.toContain('adminHeaders')
  })

  it('keeps the compatibility facade free of implementation code', () => {
    const facade = readSource('api/client.ts')
    expect(facade).toMatch(/^export \* from '\.\/publicClient'\nexport \* from '\.\/adminClient'\n$/)
  })

  it('loads admin operations through the admin-only client', () => {
    const controller = readSource('hooks/useAdminController.ts')
    expect(controller).toContain("from '../api/adminClient'")
    expect(controller).not.toContain("from '../api/client'")
  })
})
