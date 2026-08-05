import { describe, expect, it } from 'vitest'
import type { AdminNode } from '../../types'
import { applyAdminNodeOrderPatches, buildAdminNodeOrderPatches, moveAdminNodeInOrder, placeAdminNodeBesideTarget } from './AdminNodeSortModal'
import { backupNode, exampleNodeANode } from './adminTestFixtures'

function node(id: string, displayOrder: number): AdminNode {
  return { ...exampleNodeANode, id, displayName: id, displayOrder }
}

describe('server ordering', () => {
  it('moves a server one step in either direction without mutating the source array', () => {
    const ids = ['a', 'b', 'c', 'd']

    expect(moveAdminNodeInOrder(ids, 'a', 'c')).toEqual(['b', 'c', 'a', 'd'])
    expect(moveAdminNodeInOrder(ids, 'd', 'b')).toEqual(['a', 'd', 'b', 'c'])
    expect(ids).toEqual(['a', 'b', 'c', 'd'])
    expect(moveAdminNodeInOrder(ids, 'missing', 'b')).toBe(ids)
  })

  it('places the dragged server on the intended half of the target row', () => {
    const ids = ['a', 'b', 'c', 'd']

    expect(placeAdminNodeBesideTarget(ids, 'a', 'c', false)).toEqual(['b', 'a', 'c', 'd'])
    expect(placeAdminNodeBesideTarget(ids, 'a', 'c', true)).toEqual(['b', 'c', 'a', 'd'])
    expect(placeAdminNodeBesideTarget(ids, 'd', 'b', false)).toEqual(['a', 'd', 'b', 'c'])
    expect(placeAdminNodeBesideTarget(ids, 'd', 'b', true)).toEqual(['a', 'b', 'd', 'c'])
    expect(placeAdminNodeBesideTarget(ids, 'a', 'a', false)).toBe(ids)
    expect(placeAdminNodeBesideTarget(ids, 'missing', 'b', false)).toBe(ids)
  })

  it('builds only the canonical order patches affected by the new order', () => {
    const first = node('first', 10)
    const second = node('second', 20)
    const third = node('third', 30)

    expect(buildAdminNodeOrderPatches([first, second, third])).toEqual([])
    expect(buildAdminNodeOrderPatches([second, first, third])).toEqual([
      { nodeId: 'second', displayOrder: 10 },
      { nodeId: 'first', displayOrder: 20 },
    ])
  })

  it('persists order patches sequentially so responses cannot race or overwrite each other', async () => {
    const ordered = [{ ...backupNode, displayOrder: 20 }, { ...exampleNodeANode, displayOrder: 10 }]
    const calls: Array<{ nodeId: string; displayOrder: number }> = []
    let active = 0
    let maxActive = 0

    await applyAdminNodeOrderPatches(ordered, async (nodeId, input) => {
      active += 1
      maxActive = Math.max(maxActive, active)
      await new Promise((resolve) => setTimeout(resolve, 0))
      calls.push({ nodeId, displayOrder: input.displayOrder ?? -1 })
      active -= 1
    })

    expect(maxActive).toBe(1)
    expect(calls).toEqual([
      { nodeId: 'backup', displayOrder: 10 },
      { nodeId: 'example-node-a', displayOrder: 20 },
    ])
  })

  it('stops after the first failed patch instead of issuing more partial updates concurrently', async () => {
    const ordered = [node('second', 20), node('first', 10), node('third', 30)]
    const calls: string[] = []

    await expect(applyAdminNodeOrderPatches(ordered, async (nodeId) => {
      calls.push(nodeId)
      throw new Error('save failed')
    })).rejects.toThrow('save failed')

    expect(calls).toEqual(['second'])
  })
})
