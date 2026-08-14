import { afterEach, describe, expect, it, vi } from 'vitest'
import { loginAdmin, updateAdminAccount } from './adminSession'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin password requests', () => {
  it('preserves surrounding password whitespace for login and account updates', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ username: 'admin' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ username: 'admin', token: 'rotated' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))

    const rawPassword = ' new-admin-pass '
    await loginAdmin('admin', rawPassword)
    await updateAdminAccount('session-token', 'admin', rawPassword, rawPassword)

    const loginBody = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body)) as { password: string }
    const updateBody = JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body)) as { current_password: string; new_password: string }
    expect(loginBody.password).toBe(rawPassword)
    expect(updateBody.current_password).toBe(rawPassword)
    expect(updateBody.new_password).toBe(rawPassword)
  })
})
