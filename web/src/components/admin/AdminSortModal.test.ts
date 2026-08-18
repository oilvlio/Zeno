import { describe, expect, it } from 'vitest'
import { adminSortAutoScrollVelocity, moveAdminItemInOrder, placeAdminItemBesideTarget } from './AdminSortModal'

describe('admin ordering helpers', () => {
  it('moves an item one step in either direction without mutating the source array', () => {
    const ids = ['a', 'b', 'c', 'd']

    expect(moveAdminItemInOrder(ids, 'a', 'c')).toEqual(['b', 'c', 'a', 'd'])
    expect(moveAdminItemInOrder(ids, 'd', 'b')).toEqual(['a', 'd', 'b', 'c'])
    expect(ids).toEqual(['a', 'b', 'c', 'd'])
    expect(moveAdminItemInOrder(ids, 'missing', 'b')).toBe(ids)
  })

  it('places the dragged item on the intended half of the target row', () => {
    const ids = ['a', 'b', 'c', 'd']

    expect(placeAdminItemBesideTarget(ids, 'a', 'c', false)).toEqual(['b', 'a', 'c', 'd'])
    expect(placeAdminItemBesideTarget(ids, 'a', 'c', true)).toEqual(['b', 'c', 'a', 'd'])
    expect(placeAdminItemBesideTarget(ids, 'd', 'b', false)).toEqual(['a', 'd', 'b', 'c'])
    expect(placeAdminItemBesideTarget(ids, 'd', 'b', true)).toEqual(['a', 'b', 'd', 'c'])
    expect(placeAdminItemBesideTarget(ids, 'a', 'a', false)).toBe(ids)
    expect(placeAdminItemBesideTarget(ids, 'missing', 'b', false)).toBe(ids)
  })

  it('auto-scrolls only near list edges and accelerates toward the boundary', () => {
    expect(adminSortAutoScrollVelocity(150, 100, 300)).toBe(0)
    expect(adminSortAutoScrollVelocity(120, 100, 300)).toBeLessThan(0)
    expect(adminSortAutoScrollVelocity(95, 100, 300)).toBe(-14)
    expect(adminSortAutoScrollVelocity(280, 100, 300)).toBeGreaterThan(0)
    expect(adminSortAutoScrollVelocity(305, 100, 300)).toBe(14)
    expect(adminSortAutoScrollVelocity(100, 100, 100)).toBe(0)
  })
})
