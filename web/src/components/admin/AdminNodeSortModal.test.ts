import { describe, expect, it } from 'vitest'
import { adminNodeSortAutoScrollVelocity, moveAdminNodeInOrder, placeAdminNodeBesideTarget } from './AdminNodeSortModal'

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

  it('auto-scrolls only near list edges and accelerates toward the boundary', () => {
    expect(adminNodeSortAutoScrollVelocity(150, 100, 300)).toBe(0)
    expect(adminNodeSortAutoScrollVelocity(120, 100, 300)).toBeLessThan(0)
    expect(adminNodeSortAutoScrollVelocity(95, 100, 300)).toBe(-14)
    expect(adminNodeSortAutoScrollVelocity(280, 100, 300)).toBeGreaterThan(0)
    expect(adminNodeSortAutoScrollVelocity(305, 100, 300)).toBe(14)
    expect(adminNodeSortAutoScrollVelocity(100, 100, 100)).toBe(0)
  })
})
