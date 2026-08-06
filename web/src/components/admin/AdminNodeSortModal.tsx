import { type CSSProperties, type PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { runMaybePromise } from '../../lib/maybePromise'
import type { AdminNode } from '../../types'
import { ServerFlag } from '../ServerFlag'
import { AdminActionFooter, AdminModal } from './AdminPrimitives'
import type { MaybePromise } from './adminOperationalTypes'

type AdminNodeSortDragState = {
  sourceId: string
  pointerId: number
  startY: number
  currentY: number
  rect: { top: number; left: number; width: number; height: number }
  originIds: string[]
}

export function moveAdminNodeInOrder(nodeIds: string[], sourceId: string, targetId: string): string[] {
  const sourceIndex = nodeIds.indexOf(sourceId)
  const targetIndex = nodeIds.indexOf(targetId)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return nodeIds
  const nextIds = [...nodeIds]
  const [source] = nextIds.splice(sourceIndex, 1)
  nextIds.splice(targetIndex, 0, source)
  return nextIds
}

export function placeAdminNodeBesideTarget(nodeIds: string[], sourceId: string, targetId: string, afterTarget: boolean): string[] {
  if (sourceId === targetId || !nodeIds.includes(sourceId) || !nodeIds.includes(targetId)) return nodeIds
  const nextIds = nodeIds.filter((nodeId) => nodeId !== sourceId)
  const targetIndex = nextIds.indexOf(targetId)
  nextIds.splice(targetIndex + (afterTarget ? 1 : 0), 0, sourceId)
  return nextIds.every((nodeId, index) => nodeId === nodeIds[index]) ? nodeIds : nextIds
}

export function adminNodeSortAutoScrollVelocity(pointerY: number, top: number, bottom: number): number {
  if (!Number.isFinite(pointerY) || !Number.isFinite(top) || !Number.isFinite(bottom) || bottom <= top) return 0
  const edgeSize = Math.min(48, (bottom - top) / 2)
  const maxVelocity = 14
  if (pointerY < top + edgeSize) {
    return -Math.max(1, Math.ceil(maxVelocity * Math.min(1, (top + edgeSize - pointerY) / edgeSize)))
  }
  if (pointerY > bottom - edgeSize) {
    return Math.max(1, Math.ceil(maxVelocity * Math.min(1, (pointerY - (bottom - edgeSize)) / edgeSize)))
  }
  return 0
}

export function AdminNodeSortModal({ nodes, onSave, onClose }: { nodes: AdminNode[]; onSave: (nodes: AdminNode[]) => MaybePromise; onClose: () => void }) {
  const [orderedIds, setOrderedIds] = useState(() => nodes.map((node) => node.id))
  const [dragState, setDragState] = useState<AdminNodeSortDragState | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [sortAnnouncement, setSortAnnouncement] = useState('')
  const dragStateRef = useRef<AdminNodeSortDragState | null>(null)
  const dragFrameRef = useRef<number | null>(null)
  const autoScrollFrameRef = useRef<number | null>(null)
  const autoScrollVelocityRef = useRef(0)
  const pointerPositionRef = useRef<{ x: number; y: number } | null>(null)
  const sortListRef = useRef<HTMLDivElement | null>(null)
  const orderedIdsRef = useRef(orderedIds)
  orderedIdsRef.current = orderedIds
  const nodeById = new Map(nodes.map((node) => [node.id, node]))

  useEffect(() => {
    setOrderedIds((currentIds) => {
      const availableIds = new Set(nodes.map((node) => node.id))
      const retainedIds = currentIds.filter((nodeId) => availableIds.has(nodeId))
      const retainedSet = new Set(retainedIds)
      const appendedIds = nodes.map((node) => node.id).filter((nodeId) => !retainedSet.has(nodeId))
      const nextIds = appendedIds.length === 0 && retainedIds.length === currentIds.length ? currentIds : [...retainedIds, ...appendedIds]
      orderedIdsRef.current = nextIds
      return nextIds
    })
  }, [nodes])

  const orderedNodes = orderedIds.map((nodeId) => nodeById.get(nodeId)).filter((node): node is AdminNode => Boolean(node))
  const hasChanges = orderedNodes.some((node, index) => node.id !== nodes[index]?.id)
  const activeDragNode = dragState ? nodeById.get(dragState.sourceId) : undefined
  const activeDragIndex = dragState ? orderedIds.indexOf(dragState.sourceId) : -1
  const setNodeOrder = (updater: (currentIds: string[]) => string[]) => {
    setOrderedIds((currentIds) => {
      const nextIds = updater(currentIds)
      orderedIdsRef.current = nextIds
      return nextIds
    })
  }
  const moveNode = (sourceId: string, targetId: string) => setNodeOrder((currentIds) => moveAdminNodeInOrder(currentIds, sourceId, targetId))
  const moveNodeByStep = (nodeId: string, step: -1 | 1) => {
    if (submitting) return
    const sourceIndex = orderedIdsRef.current.indexOf(nodeId)
    const targetIndex = sourceIndex + step
    const targetId = orderedIdsRef.current[targetIndex]
    const node = nodeById.get(nodeId)
    if (!targetId || !node) return
    moveNode(nodeId, targetId)
    setSortAnnouncement(`${node.displayName} 已调整为第 ${targetIndex + 1} 位`)
  }
  const beginPointerDrag = (event: ReactPointerEvent<HTMLButtonElement>, nodeId: string) => {
    if (submitting || !event.isPrimary || event.button !== 0) return
    const row = event.currentTarget.closest<HTMLElement>('.admin-server-sort-row')
    if (!row) return
    event.preventDefault()
    const bounds = row.getBoundingClientRect()
    const nextDrag: AdminNodeSortDragState = {
      sourceId: nodeId,
      pointerId: event.pointerId,
      startY: event.clientY,
      currentY: event.clientY,
      rect: { top: bounds.top, left: bounds.left, width: bounds.width, height: bounds.height },
      originIds: [...orderedIdsRef.current],
    }
    dragStateRef.current = nextDrag
    setDragState(nextDrag)
  }

  useEffect(() => {
    if (!dragStateRef.current) return undefined
    const publishDragPosition = () => {
      if (dragFrameRef.current !== null) return
      dragFrameRef.current = window.requestAnimationFrame(() => {
        dragFrameRef.current = null
        const currentDrag = dragStateRef.current
        if (currentDrag) setDragState({ ...currentDrag })
      })
    }
    const stopAutoScroll = () => {
      autoScrollVelocityRef.current = 0
      if (autoScrollFrameRef.current !== null) {
        window.cancelAnimationFrame(autoScrollFrameRef.current)
        autoScrollFrameRef.current = null
      }
    }
    const updateDropTarget = (clientX: number, clientY: number, currentDrag: AdminNodeSortDragState) => {
      const targetRow = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>('.admin-server-sort-row:not(.is-placeholder)')
      const targetId = targetRow?.dataset.nodeId
      if (!targetId || targetId === currentDrag.sourceId) return
      const targetBounds = targetRow.getBoundingClientRect()
      const afterTarget = clientY >= targetBounds.top + targetBounds.height / 2
      setNodeOrder((currentIds) => placeAdminNodeBesideTarget(currentIds, currentDrag.sourceId, targetId, afterTarget))
    }
    const startAutoScroll = () => {
      if (autoScrollFrameRef.current !== null || autoScrollVelocityRef.current === 0) return
      const scroll = () => {
        autoScrollFrameRef.current = null
        const currentDrag = dragStateRef.current
        const pointer = pointerPositionRef.current
        const list = sortListRef.current
        const velocity = autoScrollVelocityRef.current
        if (!currentDrag || !pointer || !list || velocity === 0) return
        const previousScrollTop = list.scrollTop
        list.scrollTop += velocity
        if (list.scrollTop !== previousScrollTop) {
          updateDropTarget(pointer.x, pointer.y, currentDrag)
          publishDragPosition()
        }
        autoScrollFrameRef.current = window.requestAnimationFrame(scroll)
      }
      autoScrollFrameRef.current = window.requestAnimationFrame(scroll)
    }
    const finishDrag = (cancelled: boolean) => {
      const currentDrag = dragStateRef.current
      if (!currentDrag) return
      if (dragFrameRef.current !== null) {
        window.cancelAnimationFrame(dragFrameRef.current)
        dragFrameRef.current = null
      }
      stopAutoScroll()
      pointerPositionRef.current = null
      if (cancelled) {
        orderedIdsRef.current = currentDrag.originIds
        setOrderedIds(currentDrag.originIds)
      } else {
        const node = nodeById.get(currentDrag.sourceId)
        const finalIndex = orderedIdsRef.current.indexOf(currentDrag.sourceId)
        if (node && finalIndex >= 0) setSortAnnouncement(`${node.displayName} 已调整为第 ${finalIndex + 1} 位`)
      }
      dragStateRef.current = null
      setDragState(null)
    }
    const handlePointerMove = (event: PointerEvent) => {
      const currentDrag = dragStateRef.current
      if (!currentDrag || currentDrag.pointerId !== event.pointerId) return
      if (event.cancelable) event.preventDefault()
      dragStateRef.current = { ...currentDrag, currentY: event.clientY }
      pointerPositionRef.current = { x: event.clientX, y: event.clientY }
      publishDragPosition()
      updateDropTarget(event.clientX, event.clientY, currentDrag)
      const listBounds = sortListRef.current?.getBoundingClientRect()
      autoScrollVelocityRef.current = listBounds ? adminNodeSortAutoScrollVelocity(event.clientY, listBounds.top, listBounds.bottom) : 0
      if (autoScrollVelocityRef.current === 0) stopAutoScroll()
      else startAutoScroll()
    }
    const handlePointerUp = (event: PointerEvent) => {
      if (dragStateRef.current?.pointerId !== event.pointerId) return
      if (event.cancelable) event.preventDefault()
      finishDrag(false)
    }
    const handlePointerCancel = (event: PointerEvent) => {
      if (dragStateRef.current?.pointerId === event.pointerId) finishDrag(true)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') finishDrag(true)
    }
    const handleWindowBlur = () => finishDrag(true)
    window.addEventListener('pointermove', handlePointerMove, { passive: false })
    window.addEventListener('pointerup', handlePointerUp, { passive: false })
    window.addEventListener('pointercancel', handlePointerCancel)
    window.addEventListener('keydown', handleKeyDown)
    window.addEventListener('blur', handleWindowBlur)
    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', handlePointerUp)
      window.removeEventListener('pointercancel', handlePointerCancel)
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('blur', handleWindowBlur)
      if (dragFrameRef.current !== null) {
        window.cancelAnimationFrame(dragFrameRef.current)
        dragFrameRef.current = null
      }
      stopAutoScroll()
    }
  }, [dragState !== null, nodes])

  const closeModal = () => {
    if (!submitting) onClose()
  }
  const saveOrder = () => {
    if (submitting) return
    setSubmitting(true)
    setFormError(null)
    runMaybePromise(() => onSave(orderedNodes))
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title="服务器排序" className="admin-server-sort-modal" closeDisabled={submitting || dragState !== null} onClose={closeModal}>
      <section className="admin-server-sort-workspace" aria-label="调整服务器顺序" aria-busy={submitting} inert={submitting ? true : undefined}>
        <header className="admin-server-sort-intro">
          <p>按住手柄拖动整行，或使用箭头微调。</p>
          <span>{orderedNodes.length} 台</span>
        </header>
        <div ref={sortListRef} className="admin-server-sort-list" role="list" aria-label="服务器排序列表">
          {orderedNodes.map((node, index) => {
            const isFirst = index === 0
            const isLast = index === orderedNodes.length - 1
            const isPlaceholder = dragState?.sourceId === node.id
            return (
              <article
                className={`admin-server-sort-row${isPlaceholder ? ' is-placeholder' : ''}`}
                role="listitem"
                key={node.id}
                data-node-id={node.id}
              >
                <span className="admin-server-sort-index" aria-label={`第 ${index + 1} 位`}>{String(index + 1).padStart(2, '0')}</span>
                <span className="admin-server-sort-server">
                  <ServerFlag countryCode={node.countryCode} className="admin-server-sort-flag" />
                  <span>
                    <strong>{node.displayName}</strong>
                    <small>{node.publicIPv4 || node.publicIPv6 || '未上报公网 IP'}</small>
                  </span>
                </span>
                <div className="admin-server-sort-controls" aria-label={`${node.displayName} 的排序操作`}>
                  <button type="button" aria-label={`将 ${node.displayName} 上移`} title="上移" disabled={submitting || isFirst} onClick={() => moveNodeByStep(node.id, -1)}><AdminSortArrow direction="up" /></button>
                  <button type="button" aria-label={`将 ${node.displayName} 下移`} title="下移" disabled={submitting || isLast} onClick={() => moveNodeByStep(node.id, 1)}><AdminSortArrow direction="down" /></button>
                </div>
                <button
                  className="admin-drag-handle"
                  type="button"
                  disabled={submitting}
                  aria-label={`拖动 ${node.displayName} 调整顺序`}
                  title="拖动整行"
                  onPointerDown={(event) => beginPointerDrag(event, node.id)}
                ><AdminSortGrip /></button>
              </article>
            )
          })}
        </div>
        <p className="sr-only" aria-live="polite" aria-atomic="true">{sortAnnouncement}</p>
      </section>
      <AdminActionFooter className="admin-server-sort-actions" error={formError}>
        <button type="button" onClick={closeModal} disabled={submitting}>取消</button>
        <button className="admin-primary-action" type="button" onClick={saveOrder} disabled={submitting || !hasChanges}>{submitting ? '保存中…' : '保存排序'}</button>
      </AdminActionFooter>
      {dragState && activeDragNode && typeof document !== 'undefined' && createPortal(
        <article
          className="admin-server-sort-row is-drag-preview"
          aria-hidden="true"
          style={{
            '--admin-sort-drag-y': `${dragState.currentY - dragState.startY}px`,
            top: dragState.rect.top,
            left: dragState.rect.left,
            width: dragState.rect.width,
            height: dragState.rect.height,
          } as CSSProperties}
        >
          <span className="admin-server-sort-index">{String(activeDragIndex + 1).padStart(2, '0')}</span>
          <span className="admin-server-sort-server">
            <ServerFlag countryCode={activeDragNode.countryCode} className="admin-server-sort-flag" />
            <span>
              <strong>{activeDragNode.displayName}</strong>
              <small>{activeDragNode.publicIPv4 || activeDragNode.publicIPv6 || '未上报公网 IP'}</small>
            </span>
          </span>
          <span className="admin-server-sort-dragging-label">移动中</span>
          <span className="admin-drag-handle"><AdminSortGrip /></span>
        </article>,
        document.body,
      )}
    </AdminModal>
  )
}

function AdminSortArrow({ direction }: { direction: 'up' | 'down' }) {
  return <svg viewBox="0 0 20 20" aria-hidden="true"><path d={direction === 'up' ? 'm5.5 12.5 4.5-5 4.5 5' : 'm5.5 7.5 4.5 5 4.5-5'} /></svg>
}

function AdminSortGrip() {
  return (
    <svg viewBox="0 0 20 20" aria-hidden="true">
      <circle cx="7" cy="5" r="1" /><circle cx="13" cy="5" r="1" />
      <circle cx="7" cy="10" r="1" /><circle cx="13" cy="10" r="1" />
      <circle cx="7" cy="15" r="1" /><circle cx="13" cy="15" r="1" />
    </svg>
  )
}
