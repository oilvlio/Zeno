import { type DragEvent, type FormEvent, useState } from 'react'
import type { AdminNodeCreateInput, AdminNodeUpdateInput } from '../../api/adminClient'
import { sortAdminNodes, sortAdminProbeTargets } from '../../lib/adminCollections'
import type { AdminNode, AdminNodeInstallCommand, AdminProbeTarget } from '../../types'
import { ServerFlag } from '../ServerFlag'
import { AdminDateField, AdminExpandedCheckList, AdminSegmentedField } from './AdminFields'
import { AdminInstallCommand } from './AdminInstallCommand'
import { AdminDeleteConfirmModal, AdminFormSection, AdminModal, AdminModalActions, AdminRowActions } from './AdminPrimitives'
import { billingCycleOptions, billingModeOptions, formatQuotaValue, formatRenewalAmountInput, normalizeBillingCycle, parseMonthlyResetDay, parseQuota, parseRenewalAmount, quotaUnitForBytes, quotaUnitOptions, renewalCurrencyOptions } from './adminOperationalModel'
import type { AdminNodeWorkspaceProps, MaybePromise } from './adminOperationalTypes'

export function AdminNodeWorkspace({ nodes, targets, onCreate, onUpdate, onDelete, onInstallCommand }: AdminNodeWorkspaceProps) {
  const [creatingNode, setCreatingNode] = useState(false)
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null)
  const [sortingNodes, setSortingNodes] = useState(false)
  const editingNode = editingNodeId ? nodes.find((node) => node.id === editingNodeId) : undefined
  const orderedNodes = sortAdminNodes(nodes)
  const applyOrderPatches = (orderedNodes: AdminNode[]) => {
    const patches = buildAdminNodeOrderPatches(orderedNodes)
    return Promise.all(patches.map((patch) => Promise.resolve(onUpdate(patch.nodeId, { displayOrder: patch.displayOrder })))).then(() => undefined)
  }

  return (
    <section className="admin-node-section admin-workspace-panel" aria-label="admin node list">
      <header className="admin-section-heading">
        <div>
          <h3>服务器列表</h3>
        </div>
        <div className="admin-section-actions">
          <button className="admin-primary-action" type="button" onClick={() => setSortingNodes(true)}>服务器排序</button>
          <button className="admin-primary-action" type="button" onClick={() => setCreatingNode(true)}>添加服务器</button>
        </div>
      </header>

      {nodes.length === 0 && <div className="admin-state-card">还没有节点。</div>}
      {nodes.length > 0 && <AdminNodeList nodes={orderedNodes} onEdit={setEditingNodeId} onDelete={onDelete} />}

      {creatingNode && (
        <AdminNodeCreateModal
          onClose={() => setCreatingNode(false)}
          onCreate={onCreate}
          onInstallCommand={onInstallCommand}
        />
      )}

      {editingNode && (
        <AdminNodeEditModal
          key={editingNode.id}
          node={editingNode}
          targets={targets}
          onClose={() => setEditingNodeId(null)}
          onUpdate={onUpdate}
          onInstallCommand={onInstallCommand}
        />
      )}

      {sortingNodes && (
        <AdminNodeSortModal
          nodes={orderedNodes}
          onClose={() => setSortingNodes(false)}
          onSave={async (nextNodes) => {
            await applyOrderPatches(nextNodes)
            setSortingNodes(false)
          }}
        />
      )}
    </section>
  )
}

type AdminNodeOrderPatch = { nodeId: string; displayOrder: number }

function AdminNodeList({ nodes, onEdit, onDelete }: { nodes: AdminNode[]; onEdit: (nodeId: string) => void; onDelete: (nodeId: string) => MaybePromise }) {
  const [pendingDelete, setPendingDelete] = useState<AdminNode | null>(null)

  return (
    <>
    <div className="admin-list" role="list" aria-label="服务器列表">
      <div className="admin-list-head" aria-hidden="true">
        <span>服务器</span>
        <span>公网 IP</span>
        <span>Agent 版本</span>
        <span>操作</span>
      </div>
      {nodes.map((node) => (
        <article className="admin-list-row" role="listitem" key={node.id}>
          <div className="admin-list-main">
            <strong className="admin-node-title"><ServerFlag countryCode={node.countryCode} className="admin-list-flag" /><span>{node.displayName}</span></strong>
          </div>
          <span data-label="公网 IP" className={`admin-ip-stack${node.publicIPv6 ? '' : ' is-single'}`}>
            {node.publicIPv4 && <span>{node.publicIPv4}</span>}
            {node.publicIPv6 && <span>{node.publicIPv6}</span>}
            {!node.publicIPv4 && !node.publicIPv6 && <span>—</span>}
          </span>
          <span data-label="Agent 版本">{node.agentVersion || '—'}</span>
          <AdminRowActions
            entityLabel="服务器"
            name={node.displayName}
            onEdit={() => onEdit(node.id)}
            onDelete={() => setPendingDelete(node)}
          />
        </article>
      ))}
    </div>
    {pendingDelete && (
      <AdminDeleteConfirmModal
        title="删除服务器"
        subjectName={pendingDelete.displayName}
        confirmLabel="删除服务器"
        onClose={() => setPendingDelete(null)}
        onConfirm={() => onDelete(pendingDelete.id)}
      />
    )}
    </>
  )
}

function buildAdminNodeOrderPatches(nodes: AdminNode[]): AdminNodeOrderPatch[] {
  const orderedNodes = [...nodes]
  return orderedNodes
    .map((node, index) => ({ nodeId: node.id, displayOrder: (index + 1) * 10 }))
    .filter((patch) => orderedNodes.find((node) => node.id === patch.nodeId)?.displayOrder !== patch.displayOrder)
}

function moveAdminNodeInOrder(nodeIds: string[], sourceId: string, targetId: string): string[] {
  const sourceIndex = nodeIds.indexOf(sourceId)
  const targetIndex = nodeIds.indexOf(targetId)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return nodeIds
  const nextIds = [...nodeIds]
  const [source] = nextIds.splice(sourceIndex, 1)
  nextIds.splice(targetIndex, 0, source)
  return nextIds
}

function AdminNodeSortModal({ nodes, onSave, onClose }: { nodes: AdminNode[]; onSave: (nodes: AdminNode[]) => MaybePromise; onClose: () => void }) {
  const [orderedIds, setOrderedIds] = useState(() => nodes.map((node) => node.id))
  const [draggedNodeId, setDraggedNodeId] = useState<string | null>(null)
  const [dropTargetNodeId, setDropTargetNodeId] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [sortAnnouncement, setSortAnnouncement] = useState('')
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const orderedNodes = orderedIds.map((nodeId) => nodeById.get(nodeId)).filter((node): node is AdminNode => Boolean(node))
  const hasChanges = orderedNodes.some((node, index) => node.id !== nodes[index]?.id)
  const moveNode = (sourceId: string, targetId: string) => {
    setOrderedIds((currentIds) => moveAdminNodeInOrder(currentIds, sourceId, targetId))
  }
  const moveNodeByStep = (nodeId: string, step: -1 | 1) => {
    const sourceIndex = orderedIds.indexOf(nodeId)
    const targetIndex = sourceIndex + step
    const targetId = orderedIds[targetIndex]
    const node = nodeById.get(nodeId)
    if (!targetId || !node) return
    moveNode(nodeId, targetId)
    setSortAnnouncement(`${node.displayName} 已调整为第 ${targetIndex + 1} 位`)
  }
  const handleDragStart = (event: DragEvent<HTMLElement>, nodeId: string) => {
    setDraggedNodeId(nodeId)
    setDropTargetNodeId(null)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', nodeId)
  }
  const handleDragOver = (event: DragEvent<HTMLElement>, targetId: string) => {
    event.preventDefault()
    const sourceId = draggedNodeId || event.dataTransfer.getData('text/plain')
    if (!sourceId || sourceId === targetId) return
    event.dataTransfer.dropEffect = 'move'
    setDropTargetNodeId(targetId)
  }
  const handleDrop = (event: DragEvent<HTMLElement>, targetId: string) => {
    event.preventDefault()
    const sourceId = draggedNodeId || event.dataTransfer.getData('text/plain')
    if (sourceId && sourceId !== targetId) {
      moveNode(sourceId, targetId)
      const node = nodeById.get(sourceId)
      const targetIndex = orderedIds.indexOf(targetId)
      if (node && targetIndex >= 0) setSortAnnouncement(`${node.displayName} 已调整为第 ${targetIndex + 1} 位`)
    }
    setDropTargetNodeId(null)
  }
  const saveOrder = () => {
    setSubmitting(true)
    setFormError(null)
    Promise.resolve(onSave(orderedNodes))
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title="服务器排序" className="admin-server-sort-modal" onClose={onClose}>
      <div className="admin-server-sort-layout">
        <section className="admin-server-sort-workspace" aria-label="调整服务器顺序">
          <header className="admin-server-sort-intro">
            <p>拖动服务器，或使用上下按钮调整顺序。</p>
          </header>
          <div className="admin-server-sort-list" role="list" aria-label="服务器排序列表">
            {orderedNodes.map((node, index) => {
              const isFirst = index === 0
              const isLast = index === orderedNodes.length - 1
              return (
                <article
                  className={`admin-server-sort-item${draggedNodeId === node.id ? ' is-dragging' : ''}${dropTargetNodeId === node.id ? ' is-drop-target' : ''}`}
                  role="listitem"
                  draggable
                  key={node.id}
                  aria-grabbed={draggedNodeId === node.id}
                  onDragStart={(event) => handleDragStart(event, node.id)}
                  onDragOver={(event) => handleDragOver(event, node.id)}
                  onDrop={(event) => handleDrop(event, node.id)}
                  onDragEnd={() => { setDraggedNodeId(null); setDropTargetNodeId(null) }}
                >
                  <span className="admin-server-sort-index" aria-label={`第 ${index + 1} 位`}>{index + 1}</span>
                  <span className="admin-server-sort-server">
                    <ServerFlag countryCode={node.countryCode} className="admin-server-sort-flag" />
                    <strong>{node.displayName}</strong>
                  </span>
                  <div className="admin-server-sort-controls" aria-label={`${node.displayName} 的排序操作`}>
                    <button type="button" aria-label={`将 ${node.displayName} 上移`} title="上移" disabled={isFirst} onClick={() => moveNodeByStep(node.id, -1)}>↑</button>
                    <button type="button" aria-label={`将 ${node.displayName} 下移`} title="下移" disabled={isLast} onClick={() => moveNodeByStep(node.id, 1)}>↓</button>
                  </div>
                  <span className="admin-drag-handle" aria-hidden="true">⠿</span>
                </article>
              )
            })}
          </div>
          <p className="sr-only" aria-live="polite" aria-atomic="true">{sortAnnouncement}</p>
        </section>
      </div>
      <AdminModalActions className="admin-server-sort-actions" error={formError}>
        <button type="button" onClick={onClose} disabled={submitting}>取消</button>
        <button className="admin-primary-action" type="button" onClick={saveOrder} disabled={submitting || !hasChanges}>{submitting ? '保存中…' : '保存排序'}</button>
      </AdminModalActions>
    </AdminModal>
  )
}

function AdminNodeCreateModal({ onCreate, onInstallCommand, onClose }: { onCreate: (input: AdminNodeCreateInput) => Promise<AdminNode | void>; onInstallCommand: (nodeId: string) => Promise<AdminNodeInstallCommand>; onClose: () => void }) {
  const [createdNode, setCreatedNode] = useState<AdminNode | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const nodeInputFromForm = (form: HTMLFormElement): AdminNodeCreateInput | null => {
    const formData = new FormData(form)
    const displayName = String(formData.get('new-display-name') ?? '').trim()
    if (displayName === '') return null
    return {
      displayName,
      expiryDate: String(formData.get('new-expiry-date') ?? '').trim(),
      expiryPermanent: formData.get('new-expiry-permanent') === '1',
      billingCycle: String(formData.get('new-billing-cycle') ?? '').trim(),
      renewalAmount: parseRenewalAmount(String(formData.get('new-renewal-amount') ?? '')),
      renewalCurrency: String(formData.get('new-renewal-currency') ?? 'CNY'),
      billingMode: String(formData.get('new-billing-mode') ?? 'both'),
      monthlyResetDay: parseMonthlyResetDay(String(formData.get('new-monthly-reset-day') ?? '')) ?? 1,
      monthlyQuotaBytes: parseQuota(String(formData.get('new-monthly-quota') ?? ''), String(formData.get('new-monthly-quota-unit') ?? 'GB')),
    }
  }

  const createNodeFromForm = (form: HTMLFormElement): Promise<AdminNode | null> => {
    const input = nodeInputFromForm(form)
    if (!input) {
      setFormError('请先填写服务器名称。')
      return Promise.resolve(null)
    }
    setSubmitting(true)
    setFormError(null)
    return onCreate(input)
      .then((node) => {
        if (node) setCreatedNode(node)
        return node ?? null
      })
      .catch((error: unknown) => {
        setFormError(error instanceof Error ? error.message : '添加服务器失败')
        return null
      })
      .finally(() => setSubmitting(false))
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    createNodeFromForm(event.currentTarget)
  }

  return (
    <AdminModal title="添加服务器" onClose={onClose}>
      <form className="admin-node-create-form admin-node-edit-form is-sectioned" aria-label="添加服务器" onSubmit={handleSubmit}>
        <AdminFormSection title="服务器名称">
          <div className="admin-form-grid">
            <label>
              <span>服务器名称</span>
              <input name="new-display-name" autoComplete="off" placeholder="New Server" disabled={Boolean(createdNode)} />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="账单与流量">
          <div className="admin-billing-grid">
            <div className="admin-billing-row admin-billing-row--cycle">
              <AdminDateField className="admin-billing-control admin-billing-control--expiry" name="new-expiry-date" label="到期日" permanentLabel="设为永久" disabled={Boolean(createdNode)} />
              <label className="admin-billing-control admin-billing-control--amount">
                <span>续费金额</span>
                <input name="new-renewal-amount" type="number" min="0" max="1000000000" step="0.01" inputMode="decimal" disabled={Boolean(createdNode)} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--currency" name="new-renewal-currency" label="币种" defaultValue="CNY" options={renewalCurrencyOptions} disabled={Boolean(createdNode)} />
              <AdminSegmentedField className="admin-billing-control admin-billing-control--cycle" name="new-billing-cycle" label="账单周期" defaultValue="月" options={billingCycleOptions} disabled={Boolean(createdNode)} />
            </div>
            <div className="admin-billing-row admin-billing-row--traffic">
              <label className="admin-billing-control admin-billing-control--reset">
                <span>月流量重置日</span>
                <input name="new-monthly-reset-day" type="number" min="1" max="31" step="1" defaultValue="1" disabled={Boolean(createdNode)} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--mode" name="new-billing-mode" label="流量计费口径" defaultValue="both" options={billingModeOptions} disabled={Boolean(createdNode)} />
              <label className="admin-billing-control admin-billing-control--quota">
                <span>月配额</span>
                <input name="new-monthly-quota" type="number" min="0" step="0.01" disabled={Boolean(createdNode)} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--unit" name="new-monthly-quota-unit" label="配额单位" defaultValue="GB" options={quotaUnitOptions} disabled={Boolean(createdNode)} />
            </div>
          </div>
        </AdminFormSection>
        {createdNode && (
          <AdminInstallCommand
            nodeId={createdNode.id}
            initialMessage={<>已添加：{createdNode.displayName}</>}
            blocked={submitting}
            onInstallCommand={onInstallCommand}
          />
        )}
        <AdminModalActions error={formError}>
          <button type="submit" disabled={submitting || Boolean(createdNode)}>{submitting ? '添加中…' : createdNode ? '服务器已添加' : '添加服务器'}</button>
        </AdminModalActions>
      </form>
    </AdminModal>
  )
}

function AdminNodeEditModal({ node, targets, onUpdate, onInstallCommand, onClose }: { node: AdminNode; targets: AdminProbeTarget[]; onUpdate: (nodeId: string, input: AdminNodeUpdateInput) => MaybePromise; onInstallCommand: (nodeId: string) => Promise<AdminNodeInstallCommand>; onClose: () => void }) {
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const sortedTargets = sortAdminProbeTargets(targets)
  const initialSelectedTargetIds = sortedTargets.filter((target) => target.assignments.some((assignment) => assignment.nodeId === node.id && assignment.enabled)).map((target) => target.id)
  const [selectedTargetIds, setSelectedTargetIds] = useState<string[]>(initialSelectedTargetIds)
  const [homeTargetId, setHomeTargetId] = useState<string>(node.homeProbeTargetId && initialSelectedTargetIds.includes(node.homeProbeTargetId) ? node.homeProbeTargetId : '')

  const updateSelectedTargetIds = (nextTargetIds: string[]) => {
    setSelectedTargetIds(nextTargetIds)
    if (homeTargetId !== '' && !nextTargetIds.includes(homeTargetId)) {
      setHomeTargetId('')
    }
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    const displayName = String(formData.get('display-name') ?? '').trim()
    const selectedTargets = new Set(selectedTargetIds)
    setSubmitting(true)
    setFormError(null)
    Promise.resolve(onUpdate(node.id, {
      displayName: displayName || node.displayName,
      homeProbeTargetId: selectedTargets.has(homeTargetId) ? homeTargetId : '',
      expiryDate: String(formData.get('expiry-date') ?? '').trim(),
      expiryPermanent: formData.get('expiry-permanent') === '1',
      billingCycle: String(formData.get('billing-cycle') ?? '').trim(),
      renewalAmount: parseRenewalAmount(String(formData.get('renewal-amount') ?? '')),
      renewalCurrency: String(formData.get('renewal-currency') ?? node.renewalCurrency ?? 'CNY'),
      billingMode: String(formData.get('billing-mode') ?? node.billingMode),
      monthlyResetDay: parseMonthlyResetDay(String(formData.get('monthly-reset-day') ?? '')) ?? node.monthlyResetDay,
      monthlyQuotaBytes: parseQuota(String(formData.get('monthly-quota') ?? ''), String(formData.get('monthly-quota-unit') ?? quotaUnitForBytes(node.monthlyQuotaBytes))),
      probeTargetIds: [...selectedTargets],
    }))
      .then(() => onClose())
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title={`编辑服务器 · ${node.displayName}`} onClose={onClose}>
      <form className="admin-node-edit-form is-sectioned" aria-label={`${node.displayName} 节点编辑`} onSubmit={handleSubmit}>
        <AdminFormSection title="服务器名称">
          <div className="admin-form-grid">
            <label className="admin-label-without-caption">
              <input name="display-name" defaultValue={node.displayName} autoComplete="off" aria-label="服务器名称" />
            </label>
          </div>
        </AdminFormSection>
        <AdminFormSection title="关联延迟监控">
          {sortedTargets.length === 0 ? (
            <div className="admin-state-card is-compact">暂无延迟监控。</div>
          ) : (
            <AdminExpandedCheckList
              title="已选延迟监控"
              emptyText="暂无延迟监控"
              options={sortedTargets.map((target) => ({ value: target.id, label: target.name }))}
              value={selectedTargetIds}
              onChange={updateSelectedTargetIds}
              renderRight={(option, checked) => (
                <label className="admin-home-monitor-radio">
                  <input
                    type="radio"
                    name={`home-monitor-${node.id}`}
                    checked={homeTargetId === option.value}
                    onChange={() => {
                      if (!checked) updateSelectedTargetIds([...selectedTargetIds, option.value])
                      setHomeTargetId(option.value)
                    }}
                  />
                  <span>首页展示</span>
                </label>
              )}
            />
          )}
        </AdminFormSection>
        <AdminFormSection title="账单与流量">
          <div className="admin-billing-grid">
            <div className="admin-billing-row admin-billing-row--cycle">
              <AdminDateField className="admin-billing-control admin-billing-control--expiry" name="expiry-date" label="到期日" defaultValue={node.expiryDate ?? ''} defaultPermanent={node.expiryPermanent} permanentLabel="设为永久" />
              <label className="admin-billing-control admin-billing-control--amount">
                <span>续费金额</span>
                <input name="renewal-amount" type="number" min="0" max="1000000000" step="0.01" inputMode="decimal" defaultValue={formatRenewalAmountInput(node.renewalAmount)} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--currency" name="renewal-currency" label="币种" defaultValue={node.renewalCurrency || 'CNY'} options={renewalCurrencyOptions} />
              <AdminSegmentedField className="admin-billing-control admin-billing-control--cycle" name="billing-cycle" label="账单周期" defaultValue={normalizeBillingCycle(node.billingCycle)} options={billingCycleOptions} />
            </div>
            <div className="admin-billing-row admin-billing-row--traffic">
              <label className="admin-billing-control admin-billing-control--reset">
                <span>月流量重置日</span>
                <input name="monthly-reset-day" type="number" min="1" max="31" step="1" defaultValue={node.monthlyResetDay || 1} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--mode" name="billing-mode" label="流量计费口径" defaultValue={node.billingMode || 'both'} options={billingModeOptions} />
              <label className="admin-billing-control admin-billing-control--quota">
                <span>月配额</span>
                <input name="monthly-quota" type="number" min="0" step="0.01" defaultValue={formatQuotaValue(node.monthlyQuotaBytes)} />
              </label>
              <AdminSegmentedField className="admin-billing-control admin-billing-control--unit" name="monthly-quota-unit" label="配额单位" defaultValue={quotaUnitForBytes(node.monthlyQuotaBytes)} options={quotaUnitOptions} />
            </div>
          </div>
        </AdminFormSection>
        <AdminInstallCommand nodeId={node.id} onInstallCommand={onInstallCommand} />
        <AdminModalActions error={formError}>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存服务器'}</button>
        </AdminModalActions>
      </form>
    </AdminModal>
  )
}


export default AdminNodeWorkspace
