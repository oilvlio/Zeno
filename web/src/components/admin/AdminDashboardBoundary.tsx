import { Component, type ReactNode } from 'react'

type AdminModuleErrorBoundaryState = { failed: boolean }

export function AdminDashboardLoadError({ onRetry = () => window.location.reload() }: { onRetry?: () => void }) {
  return (
    <section className="state-panel is-error" role="alert">
      <p>后台加载失败，请刷新后重试。</p>
      <button type="button" onClick={onRetry}>刷新重试</button>
    </section>
  )
}

export function AdminOperationalWorkspaceLoadError({ onRetry = () => window.location.reload() }: { onRetry?: () => void }) {
  return (
    <div className="admin-state-card is-error" role="alert">
      <p>运营后台加载失败，请刷新后重试。</p>
      <button type="button" onClick={onRetry}>刷新重试</button>
    </div>
  )
}

export class AdminModuleErrorBoundary extends Component<{ children: ReactNode; fallback: ReactNode }, AdminModuleErrorBoundaryState> {
  state: AdminModuleErrorBoundaryState = { failed: false }

  static getDerivedStateFromError(): AdminModuleErrorBoundaryState {
    return { failed: true }
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}
