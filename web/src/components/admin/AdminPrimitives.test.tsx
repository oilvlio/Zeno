import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { AdminActionFooter, AdminRowActions } from './AdminPrimitives'

describe('AdminRowActions', () => {
  it('keeps shared edit and delete actions visually and accessibly consistent', () => {
    const html = renderToStaticMarkup(
      <AdminRowActions
        entityLabel="服务器"
        name="Alpha"
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    expect(html).toContain('class="admin-row-actions admin-icon-actions"')
    expect(html).toContain('aria-label="编辑服务器 Alpha"')
    expect(html).toContain('title="编辑服务器"')
    expect(html).toContain('class="admin-row-action is-icon is-danger"')
    expect(html).toContain('aria-label="删除服务器 Alpha"')
    expect(html).toContain('title="删除服务器"')
  })
})

describe('AdminActionFooter', () => {
  it('uses one shared footer for errors and actions', () => {
    const html = renderToStaticMarkup(
      <AdminActionFooter error="保存失败" className="extra-actions">
        <button type="submit">保存</button>
      </AdminActionFooter>,
    )

    expect(html).toContain('class="admin-action-footer extra-actions"')
    expect(html).toContain('class="admin-inline-note admin-action-footer-note is-error"')
    expect(html).toContain('保存失败')
    expect(html).toContain('<button type="submit">保存</button>')
  })
})
