import { type CSSProperties, type PointerEvent as ReactPointerEvent, type ReactNode, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { runMaybePromise } from '../../lib/maybePromise'
import { AdminActionFooter, AdminModal } from './AdminPrimitives'
import type { MaybePromise } from './adminOperationalTypes'

type SortableItem = { id: string }

type AdminSortDragState = {
  sourceId: string
  pointerId: number
  startY: number
  currentY: number
  rect: { top: number; left: number; width: number; height: number }
  originIds: string[]
}

export function moveAdminItemInOrder(itemIds: string[], sourceId: string, targetId: string): string[] {
  const sourceIndex = itemIds.indexOf(sourceId)
  const targetIndex = itemIds.indexOf(targetId)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return itemIds
  const nextIds = [...itemIds]
  const [source] = nextIds.splice(sourceIndex, 1)
  nextIds.splice(targetIndex, 0, source)
  return nextIds
}

export function placeAdminItemBesideTarget(itemIds: string[], sourceId: string, targetId: string, afterTarget: boolean): string[] {
  if (sourceId === targetId || !itemIds.includes(sourceId) || !itemIds.includes(targetId)) return itemIds
  const nextIds = itemIds.filter((itemId) => itemId !== sourceId)
  const targetIndex = nextIds.indexOf(targetId)
  nextIds.splice(targetIndex + (afterTarget ? 1 : 0), 0, sourceId)
  return nextIds.every((itemId, index) => itemId === itemIds[index]) ? itemIds : nextIds
}

export function adminSortAutoScrollVelocity(pointerY: number, top: number, bottom: number): number {
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

export interface AdminSortModalProps<T extends SortableItem> {
  items: T[]
  title: string
  intro: string
  countLabel: string
  listLabel: string
  itemLabel: string
  getDisplayName: (item: T) => string
  renderItem: (item: T) => ReactNode
  onSave: (items: T[]) => MaybePromise
  onClose: () => void
}

export function AdminSortModal<T extends SortableItem>({ items, title, intro, countLabel, listLabel, itemLabel, getDisplayName, renderItem, onSave, onClose }: AdminSortModalProps<T>) {
  const [orderedIds, setOrderedIds] = useState(() => items.map((item) => item.id))
  const [dragState, setDragState] = useState<AdminSortDragState | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [sortAnnouncement, setSortAnnouncement] = useState('')
  const dragStateRef = useRef<AdminSortDragState | null>(null)
  const dragFrameRef = useRef<number | null>(null)
  const autoScrollFrameRef = useRef<number | null>(null)
  const autoScrollVelocityRef = useRef(0)
  const pointerPositionRef = useRef<{ x: number; y: number } | null>(null)
  const sortListRef = useRef<HTMLDivElement | null>(null)
  const orderedIdsRef = useRef(orderedIds)
  const itemByIdRef = useRef(new Map(items.map((item) => [item.id, item])))
  const getDisplayNameRef = useRef(getDisplayName)
  orderedIdsRef.current = orderedIds
  itemByIdRef.current = new Map(items.map((item) => [item.id, item]))
  getDisplayNameRef.current = getDisplayName
  const itemById = itemByIdRef.current

  useEffect(() => {
    setOrderedIds((currentIds) => {
      const availableIds = new Set(items.map((item) => item.id))
      const retainedIds = currentIds.filter((itemId) => availableIds.has(itemId))
      const retainedSet = new Set(retainedIds)
      const appendedIds = items.map((item) => item.id).filter((itemId) => !retainedSet.has(itemId))
      const nextIds = appendedIds.length === 0 && retainedIds.length === currentIds.length ? currentIds : [...retainedIds, ...appendedIds]
      orderedIdsRef.current = nextIds
      return nextIds
    })
  }, [items])

  const orderedItems = orderedIds.map((itemId) => itemById.get(itemId)).filter((item): item is T => Boolean(item))
  const hasChanges = orderedItems.some((item, index) => item.id !== items[index]?.id)
  const activeDragItem = dragState ? itemById.get(dragState.sourceId) : undefined
  const activeDragIndex = dragState ? orderedIds.indexOf(dragState.sourceId) : -1
  const setItemOrder = (updater: (currentIds: string[]) => string[]) => {
    setOrderedIds((currentIds) => {
      const nextIds = updater(currentIds)
      orderedIdsRef.current = nextIds
      return nextIds
    })
  }
  const moveItem = (sourceId: string, targetId: string) => setItemOrder((currentIds) => moveAdminItemInOrder(currentIds, sourceId, targetId))
  const moveItemByStep = (itemId: string, step: -1 | 1) => {
    if (submitting) return
    const sourceIndex = orderedIdsRef.current.indexOf(itemId)
    const targetIndex = sourceIndex + step
    const targetId = orderedIdsRef.current[targetIndex]
    const item = itemById.get(itemId)
    if (!targetId || !item) return
    moveItem(itemId, targetId)
    setSortAnnouncement(`${getDisplayName(item)} 已调整为第 ${targetIndex + 1} 位`)
  }
  const beginPointerDrag = (event: ReactPointerEvent<HTMLButtonElement>, itemId: string) => {
    if (submitting || !event.isPrimary || event.button !== 0) return
    const row = event.currentTarget.closest<HTMLElement>('.admin-sort-row')
    if (!row) return
    event.preventDefault()
    const bounds = row.getBoundingClientRect()
    const nextDrag: AdminSortDragState = {
      sourceId: itemId,
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
    const updateDropTarget = (clientX: number, clientY: number, currentDrag: AdminSortDragState) => {
      const targetRow = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>('.admin-sort-row:not(.is-placeholder)')
      const targetId = targetRow?.dataset.sortItemId
      if (!targetId || targetId === currentDrag.sourceId) return
      const targetBounds = targetRow.getBoundingClientRect()
      const afterTarget = clientY >= targetBounds.top + targetBounds.height / 2
      setItemOrder((currentIds) => placeAdminItemBesideTarget(currentIds, currentDrag.sourceId, targetId, afterTarget))
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
        const item = itemByIdRef.current.get(currentDrag.sourceId)
        const finalIndex = orderedIdsRef.current.indexOf(currentDrag.sourceId)
        if (item && finalIndex >= 0) setSortAnnouncement(`${getDisplayNameRef.current(item)} 已调整为第 ${finalIndex + 1} 位`)
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
      autoScrollVelocityRef.current = listBounds ? adminSortAutoScrollVelocity(event.clientY, listBounds.top, listBounds.bottom) : 0
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
  }, [dragState !== null])

  const closeModal = () => {
    if (!submitting) onClose()
  }
  const saveOrder = () => {
    if (submitting) return
    setSubmitting(true)
    setFormError(null)
    runMaybePromise(() => onSave(orderedItems))
      .catch((error: unknown) => setFormError(error instanceof Error ? error.message : '保存失败'))
      .finally(() => setSubmitting(false))
  }

  return (
    <AdminModal title={title} className="admin-sort-modal" closeDisabled={submitting || dragState !== null} onClose={closeModal}>
      <section className="admin-sort-workspace" aria-label={`调整${itemLabel}顺序`} aria-busy={submitting} inert={submitting ? true : undefined}>
        <header className="admin-sort-intro">
          <p>{intro}</p>
          <span>{items.length} {countLabel}</span>
        </header>
        <div ref={sortListRef} className="admin-sort-list" role="list" aria-label={listLabel}>
          {orderedItems.map((item, index) => {
            const isFirst = index === 0
            const isLast = index === orderedItems.length - 1
            const isPlaceholder = dragState?.sourceId === item.id
            return (
              <article
                className={`admin-sort-row${isPlaceholder ? ' is-placeholder' : ''}`}
                role="listitem"
                key={item.id}
                data-sort-item-id={item.id}
              >
                <span className="admin-sort-index" aria-label={`第 ${index + 1} 位`}>{String(index + 1).padStart(2, '0')}</span>
                {renderItem(item)}
                <div className="admin-sort-controls" aria-label={`${getDisplayName(item)} 的排序操作`}>
                  <button type="button" aria-label={`将 ${getDisplayName(item)} 上移`} title="上移" disabled={submitting || isFirst} onClick={() => moveItemByStep(item.id, -1)}><AdminSortArrow direction="up" /></button>
                  <button type="button" aria-label={`将 ${getDisplayName(item)} 下移`} title="下移" disabled={submitting || isLast} onClick={() => moveItemByStep(item.id, 1)}><AdminSortArrow direction="down" /></button>
                </div>
                <button
                  className="admin-drag-handle"
                  type="button"
                  disabled={submitting}
                  aria-label={`拖动 ${getDisplayName(item)} 调整顺序`}
                  title="拖动整行"
                  onPointerDown={(event) => beginPointerDrag(event, item.id)}
                ><AdminSortGrip /></button>
              </article>
            )
          })}
        </div>
        <p className="sr-only" aria-live="polite" aria-atomic="true">{sortAnnouncement}</p>
      </section>
      <AdminActionFooter className="admin-sort-actions" error={formError}>
        <button type="button" onClick={closeModal} disabled={submitting}>取消</button>
        <button className="admin-primary-action" type="button" onClick={saveOrder} disabled={submitting || !hasChanges}>{submitting ? '保存中…' : '保存排序'}</button>
      </AdminActionFooter>
      {dragState && activeDragItem && typeof document !== 'undefined' && createPortal(
        <article
          className="zeno-overlay-surface admin-sort-row is-drag-preview"
          aria-hidden="true"
          style={{
            '--admin-sort-drag-y': `${dragState.currentY - dragState.startY}px`,
            top: dragState.rect.top,
            left: dragState.rect.left,
            width: dragState.rect.width,
            height: dragState.rect.height,
          } as CSSProperties}
        >
          <span className="admin-sort-index">{String(activeDragIndex + 1).padStart(2, '0')}</span>
          {renderItem(activeDragItem)}
          <span className="admin-sort-dragging-label">移动中</span>
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
