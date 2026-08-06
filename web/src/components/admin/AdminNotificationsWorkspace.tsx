import { type FormEvent, useState } from 'react'
import type { AdminAlertRuleUpdateInput, AdminNotificationChannelCreateInput, AdminNotificationChannelUpdateInput } from '../../api/adminClient'
import { runMaybePromise } from '../../lib/maybePromise'
import type { AdminAlertRule, AdminNode, AdminNotificationChannel } from '../../types'
import { AdminSegmentedField } from './AdminFields'
import { AdminCredentialField, AdminFormSection, AdminModal, AdminActionFooter, AdminRowActions, AdminWorkspaceHeading } from './AdminPrimitives'
import { formatAlertRuleNote, formatAlertRuleScope, formatRenewalDayOption, normalizeRenewalThreshold, parseNonNegativeInt, parsePercentage, parseRenewalThreshold, renewalDayOptions } from './adminOperationalModel'
import type { AdminNotificationsWorkspaceProps, MaybePromise } from './adminOperationalTypes'

function AdminAlertRulesSection({ rules, nodes, onUpdate }: { rules: AdminAlertRule[]; nodes: AdminNode[]; onUpdate: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise }) {
  const [editingRule, setEditingRule] = useState<AdminAlertRule | null>(null)
  const [addingRule, setAddingRule] = useState(false)
  const addedRules = rules.filter((rule) => rule.enabled)
  const availableRules = rules.filter((rule) => !rule.enabled)

  return (
    <section className="admin-notification-group admin-alert-rule-section" aria-label="通知类型规则">
      <div className="admin-block-heading">
        <h4>通知类型</h4>
        <button className="admin-primary-action" type="button" onClick={() => setAddingRule(true)}>添加通知类型</button>
      </div>
      {addedRules.length === 0 && <div className="admin-state-card">还没有添加通知类型。</div>}
      {addedRules.length > 0 && <AdminAlertRuleList rules={addedRules} onEdit={setEditingRule} onUpdate={onUpdate} />}

      {addingRule && (
        <AdminAlertRuleAddModal
          rules={availableRules}
          nodes={nodes}
          onClose={() => setAddingRule(false)}
          onAdd={onUpdate}
        />
      )}

      {editingRule && (
        <AdminAlertRuleEditModal
          rule={editingRule}
          nodes={nodes}
          onClose={() => setEditingRule(null)}
          onUpdate={onUpdate}
        />
      )}
    </section>
  )
}

function AdminAlertRuleList({ rules, onEdit, onUpdate }: { rules: AdminAlertRule[]; onEdit: (rule: AdminAlertRule) => void; onUpdate: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise }) {
  const confirmDelete = (rule: AdminAlertRule) => {
    const ok = typeof window === 'undefined' ? true : window.confirm(`确认删除通知类型「${rule.name}」？`)
    if (ok) onUpdate(rule.id, { enabled: false })
  }

  return (
    <div className="admin-list admin-alert-rule-list" role="list" aria-label="通知类型列表">
      <div className="admin-list-head" aria-hidden="true">
        <span>通知类型</span>
        <span>状态</span>
        <span>操作</span>
      </div>
      {rules.map((rule) => (
        <article className="admin-list-row" role="listitem" key={rule.id}>
          <div className="admin-list-main">
            <strong>{rule.name}</strong>
            {formatAlertRuleNote(rule) && <small>{formatAlertRuleNote(rule)}</small>}
          </div>
          <AdminStatusBadge label={rule.enabled ? '启用中' : '已停用'} status={rule.enabled ? 'online' : 'disabled'} dataLabel="状态" />
          <AdminRowActions
            entityLabel="通知类型"
            name={rule.name}
            onEdit={() => onEdit(rule)}
            onDelete={() => confirmDelete(rule)}
          />
        </article>
      ))}
    </div>
  )
}

function AdminAlertRuleAddModal({ rules, nodes, onAdd, onClose }: { rules: AdminAlertRule[]; nodes: AdminNode[]; onAdd: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise; onClose: () => void }) {
  const [submittingRuleId, setSubmittingRuleId] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const addRule = (ruleId: string) => {
    if (submittingRuleId !== null) return
    setSubmittingRuleId(ruleId)
    setFormError(null)
    runMaybePromise(() => onAdd(ruleId, { enabled: true }))
      .then(() => onClose())
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '添加失败'))
      .finally(() => setSubmittingRuleId(null))
  }
  return (
    <AdminModal title="添加通知类型" closeDisabled={submittingRuleId !== null} onClose={onClose}>
      <div className="admin-alert-rule-add-form admin-node-edit-form is-sectioned" aria-label="添加通知类型" aria-busy={submittingRuleId !== null} inert={submittingRuleId !== null ? true : undefined}>
        <AdminFormSection title="通知类型">
          <div className="admin-rule-picker" role="list" aria-label="可添加通知类型">
            {rules.length === 0 && <div className="admin-state-card">所有通知类型都已添加。</div>}
            {rules.map((rule) => (
              <article className="admin-rule-picker-row" role="listitem" key={rule.id}>
                <div className="admin-list-main">
                  <strong>{rule.name}</strong>
                  <small>{formatAlertRuleNote(rule) || formatAlertRuleScope(rule, nodes)}</small>
                </div>
                <button className="admin-primary-action" type="button" onClick={() => addRule(rule.id)} disabled={submittingRuleId !== null}>{submittingRuleId === rule.id ? '添加中…' : '添加'}</button>
              </article>
            ))}
          </div>
        </AdminFormSection>
        {formError && <div className="admin-install-error">{formError}</div>}
      </div>
    </AdminModal>
  )
}

function AdminAlertRuleEditModal({ rule, nodes, onUpdate, onClose }: { rule: AdminAlertRule; nodes: AdminNode[]; onUpdate: (ruleId: string, input: AdminAlertRuleUpdateInput) => MaybePromise; onClose: () => void }) {
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const initialScopeNodeIds = rule.scopeNodeIds.length === 0 ? nodes.map((node) => node.id) : rule.scopeNodeIds
  const isRenewalRule = rule.metric === 'expiry_days'
  const isResourceRule = rule.category === 'resource' && rule.thresholdUnit === '%'
  const supportsDuration = !isRenewalRule
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (submitting) return
    const formData = new FormData(event.currentTarget)
    const scopeNodeIds = nodes.filter((node) => formData.get(`rule-scope-${node.id}`) === 'on').map((node) => node.id)
    const renewalThreshold = isRenewalRule ? parseRenewalThreshold(String(formData.get('rule-renewal-days') ?? '')) : null
    const resourceThreshold = isResourceRule ? parsePercentage(String(formData.get('rule-threshold-percent') ?? '')) : null
    const durationSec = supportsDuration ? parseNonNegativeInt(String(formData.get('rule-duration-sec') ?? '')) : null
    setSubmitting(true)
    setFormError(null)
    runMaybePromise(() => onUpdate(rule.id, {
      enabled: formData.get('rule-enabled') === 'on',
      ...(isRenewalRule && renewalThreshold !== null ? { threshold: renewalThreshold } : {}),
      ...(isResourceRule && resourceThreshold !== null ? { threshold: resourceThreshold } : {}),
      ...(supportsDuration && durationSec !== null ? { durationSec } : {}),
      scopeNodeIds,
    }))
      .then(() => onClose())
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title={`编辑通知类型 · ${rule.name}`} closeDisabled={submitting} onClose={onClose}>
      <form className="admin-alert-rule-edit-form admin-node-edit-form is-sectioned" aria-label={`${rule.name} 通知类型编辑`} aria-busy={submitting} inert={submitting ? true : undefined} onSubmit={handleSubmit}>
        <AdminFormSection title="通知设置">
          <div className="admin-form-grid admin-alert-rule-settings-grid">
            {isRenewalRule && (
              <AdminSegmentedField
                name="rule-renewal-days"
                label="提前提醒"
                defaultValue={String(normalizeRenewalThreshold(rule.threshold))}
                options={renewalDayOptions.map((days) => ({ value: String(days), label: formatRenewalDayOption(days) }))}
              />
            )}
            {supportsDuration && (
              <label>
                <span>{isResourceRule ? '统计窗口 s' : '确认时间 s'}</span>
                <input name="rule-duration-sec" type="number" min="0" defaultValue={rule.durationSec} />
              </label>
            )}
            {isResourceRule && (
              <label>
                <span>平均超过 %</span>
                <input name="rule-threshold-percent" type="number" min="0" max="100" step="0.1" defaultValue={rule.threshold} />
              </label>
            )}
          </div>
        </AdminFormSection>
        <AdminFormSection title="通知状态">
          <label className="admin-node-toggle admin-alert-rule-enabled-toggle">
            <input name="rule-enabled" type="checkbox" defaultChecked={rule.enabled} />
            <span>启用通知类型</span>
          </label>
        </AdminFormSection>
        {nodes.length > 0 && (
          <AdminFormSection title="作用服务器">
            <div className="admin-rule-scope-list admin-target-assignment-list">
              {nodes.map((node) => (
                <label className="admin-node-toggle admin-target-assignment-toggle" key={node.id}>
                  <input name={`rule-scope-${node.id}`} type="checkbox" defaultChecked={initialScopeNodeIds.includes(node.id)} />
                  <span>{node.displayName || node.id}</span>
                </label>
              ))}
            </div>
          </AdminFormSection>
        )}
        <AdminActionFooter error={formError}>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存通知类型'}</button>
        </AdminActionFooter>
      </form>
    </AdminModal>
  )
}

export function AdminNotificationsWorkspace({ channels, rules, nodes, onChannelCreate, onChannelUpdate, onChannelDelete, onChannelTest, onRuleUpdate }: AdminNotificationsWorkspaceProps) {
  const [creatingChannel, setCreatingChannel] = useState(false)
  const [editingChannel, setEditingChannel] = useState<AdminNotificationChannel | null>(null)

  return (
    <section className="admin-notification-section admin-workspace-panel admin-workspace-panel--grouped" aria-label="admin notification settings">
      <AdminWorkspaceHeading
        title="通知渠道"
        actions={<button className="admin-primary-action" type="button" onClick={() => setCreatingChannel(true)}>添加通知渠道</button>}
      />
      <div className="admin-workspace-card-list">
        <section className="admin-workspace-card" aria-label="通知设置">
          <section className="admin-notification-group" aria-label="通知渠道">
            {channels.length === 0 && <div className="admin-state-card">还没有通知渠道。</div>}
            {channels.length > 0 && <AdminNotificationChannelList channels={channels} onDelete={onChannelDelete} onEdit={setEditingChannel} />}
          </section>
          <AdminAlertRulesSection rules={rules} nodes={nodes} onUpdate={onRuleUpdate} />
        </section>
      </div>

      {creatingChannel && (
        <AdminNotificationChannelCreateModal
          onClose={() => setCreatingChannel(false)}
          onCreate={onChannelCreate}
        />
      )}
      {editingChannel && (
        <AdminNotificationChannelEditModal
          channel={editingChannel}
          onClose={() => setEditingChannel(null)}
          onTest={onChannelTest}
          onUpdate={onChannelUpdate}
        />
      )}
    </section>
  )
}

function AdminNotificationChannelList({ channels, onDelete, onEdit }: { channels: AdminNotificationChannel[]; onDelete: (channelId: string) => void; onEdit: (channel: AdminNotificationChannel) => void }) {
  const confirmDelete = (channel: AdminNotificationChannel) => {
    const ok = typeof window === 'undefined' ? true : window.confirm(`确认删除通知渠道「${channel.name}」？`)
    if (ok) onDelete(channel.id)
  }

  return (
    <div className="admin-list admin-notification-list" role="list" aria-label="通知渠道列表">
      <div className="admin-list-head" aria-hidden="true">
        <span>渠道</span>
        <span>状态</span>
        <span>操作</span>
      </div>
      {channels.map((channel) => (
        <article className="admin-list-row" role="listitem" key={channel.id}>
          <div className="admin-list-main">
            <strong>{channel.name}</strong>
          </div>
          <AdminStatusBadge label={channel.enabled ? '启用中' : '已停用'} status={channel.enabled ? 'online' : 'disabled'} dataLabel="状态" />
          <AdminRowActions
            entityLabel="通知渠道"
            actionEntityLabel="渠道"
            name={channel.name}
            onEdit={() => onEdit(channel)}
            onDelete={() => confirmDelete(channel)}
          />
        </article>
      ))}
    </div>
  )
}

function AdminStatusBadge({ label, status, dataLabel }: { label: string; status: 'online' | 'disabled'; dataLabel?: string }) {
  return <span data-label={dataLabel} className={`admin-node-status admin-status-indicator status-${status}`}><i className="admin-status-dot" aria-hidden="true" />{label}</span>
}

function AdminNotificationChannelEditModal({ channel, onUpdate, onTest, onClose }: { channel: AdminNotificationChannel; onUpdate: (channelId: string, input: AdminNotificationChannelUpdateInput) => MaybePromise; onTest: (channelId: string) => void; onClose: () => void }) {
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (submitting) return
    const formData = new FormData(event.currentTarget)
    const name = String(formData.get('channel-name') ?? '').trim()
    const destination = String(formData.get('channel-destination') ?? '').trim()
    const credential = String(formData.get('channel-credential') ?? '').trim()
    if (name === '') return
    setSubmitting(true)
    setFormError(null)
    runMaybePromise(() => onUpdate(channel.id, {
      name,
      ...(destination !== '' ? { destination } : {}),
      ...(credential !== '' ? { credential } : {}),
      enabled: formData.get('channel-enabled') === 'on',
    }))
      .then(() => onClose())
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title="编辑通知渠道" className="admin-notification-channel-modal" closeDisabled={submitting} onClose={onClose}>
      <form className="admin-notification-edit-form admin-node-edit-form is-sectioned" aria-label="编辑通知渠道" aria-busy={submitting} inert={submitting ? true : undefined} onSubmit={handleSubmit}>
        <AdminFormSection title="渠道配置">
          <div className="admin-form-grid admin-channel-form-grid">
            <label>
              <span>渠道名称</span>
              <input name="channel-name" autoComplete="off" defaultValue={channel.name} />
            </label>
            <label>
              <span>Telegram Chat ID</span>
              <input name="channel-destination" autoComplete="off" defaultValue={channel.destination} />
            </label>
            <AdminCredentialField
              name="channel-credential"
              placeholder={channel.credentialSet ? '留空则保留已保存 Token' : '请输入 Telegram Bot Token'}
            />
            <label className="admin-node-toggle admin-channel-enabled-toggle">
              <input name="channel-enabled" type="checkbox" defaultChecked={channel.enabled} />
              <span>启用渠道</span>
            </label>
          </div>
        </AdminFormSection>
        <AdminActionFooter error={formError}>
          <button type="button" onClick={() => onTest(channel.id)}>测试发送</button>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存通知渠道'}</button>
        </AdminActionFooter>
      </form>
    </AdminModal>
  )
}

function AdminNotificationChannelCreateModal({ onCreate, onClose }: { onCreate: (input: AdminNotificationChannelCreateInput) => MaybePromise; onClose: () => void }) {
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (submitting) return
    const formData = new FormData(event.currentTarget)
    const name = String(formData.get('new-channel-name') ?? '').trim()
    const destination = String(formData.get('new-channel-destination') ?? '').trim()
    const credential = String(formData.get('new-channel-credential') ?? '').trim()
    if (name === '' || destination === '' || credential === '') return
    setSubmitting(true)
    setFormError(null)
    runMaybePromise(() => onCreate({
      name,
      destination,
      credential,
      enabled: formData.get('new-channel-enabled') === 'on',
    }))
      .then(() => onClose())
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '添加失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title="添加通知渠道" className="admin-notification-channel-modal" closeDisabled={submitting} onClose={onClose}>
      <form className="admin-notification-create-form admin-node-edit-form is-sectioned" aria-label="添加通知渠道" aria-busy={submitting} inert={submitting ? true : undefined} onSubmit={handleSubmit}>
        <AdminFormSection title="渠道配置">
          <div className="admin-form-grid admin-channel-form-grid">
            <label>
              <span>渠道名称</span>
              <input name="new-channel-name" autoComplete="off" placeholder="Zeno Telegram" />
            </label>
            <label>
              <span>Telegram Chat ID</span>
              <input name="new-channel-destination" autoComplete="off" placeholder="请输入 Telegram Chat ID" />
            </label>
            <AdminCredentialField name="new-channel-credential" placeholder="请输入 Telegram Bot Token" />
            <label className="admin-node-toggle admin-channel-enabled-toggle">
              <input name="new-channel-enabled" type="checkbox" defaultChecked />
              <span>创建后启用渠道</span>
            </label>
          </div>
        </AdminFormSection>
        <AdminActionFooter error={formError}>
          <button type="submit" disabled={submitting}>{submitting ? '保存中…' : '保存通知渠道'}</button>
        </AdminActionFooter>
      </form>
    </AdminModal>
  )
}



export default AdminNotificationsWorkspace
