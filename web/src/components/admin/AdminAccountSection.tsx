import { type FormEvent, useState } from 'react'
import type { AdminAccountData } from '../../api/adminSession'
import { AdminFormSection, AdminActionFooter } from './AdminPrimitives'

export interface AdminAccountSectionProps {
  account: AdminAccountData
  onUpdate: (username: string, currentPassword: string, newPassword: string) => Promise<void>
}

export function validAdminAccountUsername(username: string): boolean {
  return /^[A-Za-z0-9._-]{3,64}$/.test(username.trim())
}

export default function AdminAccountSection({ account, onUpdate }: AdminAccountSectionProps) {
  const [message, setMessage] = useState<{ kind: 'error' | 'success'; text: string } | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    const username = String(formData.get('account-username') ?? '').trim()
    const currentPassword = String(formData.get('current-password') ?? '').trim()
    const newPassword = String(formData.get('new-password') ?? '').trim()
    const confirmPassword = String(formData.get('confirm-password') ?? '').trim()
    if (!validAdminAccountUsername(username)) {
      setMessage({ kind: 'error', text: '账号只能使用 3-64 位字母、数字、点、短横线或下划线。' })
      return
    }
    if (currentPassword === '') {
      setMessage({ kind: 'error', text: '请输入当前密码确认修改。' })
      return
    }
    if (newPassword !== '' && newPassword.length < 8) {
      setMessage({ kind: 'error', text: '新密码至少 8 位；不改密码可留空。' })
      return
    }
    if (newPassword !== confirmPassword) {
      setMessage({ kind: 'error', text: '两次输入的新密码不一致。' })
      return
    }
    setSubmitting(true)
    setMessage(null)
    onUpdate(username, currentPassword, newPassword)
      .then(() => setMessage({ kind: 'success', text: '账户已更新。' }))
      .catch((error: unknown) => setMessage({ kind: 'error', text: error instanceof Error ? error.message : '账户更新失败。' }))
      .finally(() => setSubmitting(false))
  }

  return (
    <section className="admin-account-section admin-workspace-panel" aria-label="账户设置">
      <header className="admin-section-heading">
        <div><h3>账户</h3></div>
      </header>
      <form className="admin-account-form admin-node-edit-form is-sectioned" aria-label="修改账号和密码" onSubmit={handleSubmit}>
        <AdminFormSection title="登录信息">
          <div className="admin-form-grid">
            <label>
              <span>账号</span>
              <input name="account-username" autoComplete="username" defaultValue={account.username} />
            </label>
            <label>
              <span>当前密码</span>
              <input name="current-password" type="password" autoComplete="current-password" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="修改密码">
          <div className="admin-form-grid">
            <label>
              <span>新密码</span>
              <input name="new-password" type="password" autoComplete="new-password" placeholder="留空则不修改" />
            </label>
            <label>
              <span>确认新密码</span>
              <input name="confirm-password" type="password" autoComplete="new-password" placeholder="留空则不修改" />
            </label>
          </div>
        </AdminFormSection>
        <AdminActionFooter>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存账户'}</button>
        </AdminActionFooter>
        {message && <p className={`admin-install-error${message.kind === 'success' ? ' is-success' : ''}`}>{message.text}</p>}
      </form>
    </section>
  )
}
