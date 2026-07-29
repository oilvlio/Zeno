import { describe, expect, it, vi } from 'vitest'
import { createAdminMutationCoordinator, runAdminMutationLease } from './adminMutationCoordinator'

type SessionIdentity = { generation: number }

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('admin mutation coordinator', () => {
  it('aborts pending work and ignores a late result once logout starts', async () => {
    let currentGeneration = 1
    const coordinator = createAdminMutationCoordinator<SessionIdentity>((identity) => identity.generation === currentGeneration)
    const lease = coordinator.begin({ generation: 1 })
    expect(lease).not.toBeNull()
    if (!lease) return

    const result = deferred<string>()
    const apply = vi.fn((value: string) => `applied:${value}`)
    const pending = runAdminMutationLease(lease, () => result.promise, apply)

    coordinator.beginSessionTransition()
    currentGeneration = 2
    expect(lease.signal.aborted).toBe(true)
    expect(coordinator.begin({ generation: 2 })).toBeNull()

    result.resolve('late')
    await expect(pending).resolves.toBeUndefined()
    expect(apply).not.toHaveBeenCalled()
  })

  it('allows new work after a failed logout reopens the coordinator', () => {
    const coordinator = createAdminMutationCoordinator<SessionIdentity>(() => true)
    coordinator.beginSessionTransition()
    expect(coordinator.begin({ generation: 1 })).toBeNull()
    coordinator.endSessionTransition()
    expect(coordinator.begin({ generation: 1 })).not.toBeNull()
  })

  it('ignores results from an invalidated session even without an abort', async () => {
    let currentGeneration = 1
    const coordinator = createAdminMutationCoordinator<SessionIdentity>((identity) => identity.generation === currentGeneration)
    const lease = coordinator.begin({ generation: 1 })
    expect(lease).not.toBeNull()
    if (!lease) return
    currentGeneration = 2
    const apply = vi.fn()
    await expect(runAdminMutationLease(lease, async () => 'stale', apply)).resolves.toBeUndefined()
    expect(apply).not.toHaveBeenCalled()
  })

  it('keeps mutation state untouched when the request fails and releases the lease', async () => {
    const coordinator = createAdminMutationCoordinator<SessionIdentity>(() => true)
    const lease = coordinator.begin({ generation: 1 })
    expect(lease).not.toBeNull()
    if (!lease) return
    const apply = vi.fn()
    const failure = new Error('save failed')
    await expect(runAdminMutationLease(lease, async () => { throw failure }, apply)).rejects.toBe(failure)
    expect(apply).not.toHaveBeenCalled()
    expect(coordinator.begin({ generation: 1 })).not.toBeNull()
  })
})
