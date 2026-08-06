import { describe, expect, it, vi } from 'vitest'
import { runMaybePromise } from './maybePromise'

describe('runMaybePromise', () => {
  it('turns synchronous throws into promise rejections', async () => {
    const action = vi.fn(() => {
      throw new Error('sync failure')
    })

    await expect(runMaybePromise(action)).rejects.toThrow('sync failure')
    expect(action).toHaveBeenCalledTimes(1)
  })

  it('preserves synchronous values and asynchronous rejections', async () => {
    await expect(runMaybePromise(() => 42)).resolves.toBe(42)
    await expect(runMaybePromise(() => Promise.reject(new Error('async failure')))).rejects.toThrow('async failure')
  })
})
