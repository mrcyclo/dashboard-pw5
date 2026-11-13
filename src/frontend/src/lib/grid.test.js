import { describe, expect, it } from 'vitest'
import { canShow, cellRect, fits, NO_ROOM, parseSize, snap } from './grid.js'

const defaults = () => [
  { kind: 'clock', size: '3x1', col: 0, row: 0 },
  { kind: 'claude', size: '6x2', col: 0, row: 1 },
  { kind: 'media', size: '6x2', col: 0, row: 6 },
]

describe('grid geometry matches grid.go', () => {
  it('places cells like CellRect', () => {
    expect(cellRect(0, 0, { cols: 3, rows: 1 })).toEqual({ x: 48, y: 48, w: 562, h: 180 })
    expect(cellRect(0, 6, { cols: 6, rows: 2 })).toEqual({ x: 48, y: 1224, w: 1140, h: 376 })
    expect(cellRect(0, 1, { cols: 6, rows: 2 })).toEqual({ x: 48, y: 244, w: 1140, h: 376 })
  })
  it('parses sizes', () => {
    expect(parseSize('3x1')).toEqual({ cols: 3, rows: 1 })
    expect(parseSize('none')).toBeNull()
    expect(parseSize('7x1')).toBeNull()
  })
  it('refuses overlaps, the reserved corner and the outside', () => {
    expect(fits(defaults(), 'media', { cols: 6, rows: 2 }, 0, 2)).toBe(false)
    expect(fits(defaults(), 'media', { cols: 3, rows: 1 }, 3, 0)).toBe(false)
    expect(fits(defaults(), 'media', { cols: 6, rows: 2 }, 0, 7)).toBe(false)
    expect(fits(defaults(), 'media', { cols: 6, rows: 2 }, 0, 5)).toBe(true)
  })
})

describe('canShow', () => {
  it('allows hiding and sizes that fit', () => {
    expect(canShow(defaults(), 'media', 'none')).toEqual({ ok: true })
    expect(canShow(defaults(), 'clock', '6x1')).toEqual({ ok: true })
  })
  it('says why a size has no room, with the same words as the PC', () => {
    // Claude 3x3 at (0,2), media 6x1 at (0,5), clock at (3,6): no two free full rows touch.
    const layout = [
      { kind: 'clock', size: '3x1', col: 3, row: 6 },
      { kind: 'claude', size: '3x3', col: 0, row: 2 },
      { kind: 'media', size: '6x1', col: 0, row: 5 },
    ]
    expect(canShow(layout, 'media', '6x2')).toEqual({ ok: false, reason: NO_ROOM })
    expect(NO_ROOM).toBe('No room for this size. Move or hide another widget first')
  })
  it('lets a hidden widget come back where there is room', () => {
    const layout = defaults()
    layout[2].size = 'none'
    expect(canShow(layout, 'media', '6x2')).toEqual({ ok: true })
  })
})

describe('snap', () => {
  it('rounds a dragged corner to the nearest cell', () => {
    expect(snap(48, 48)).toEqual({ col: 0, row: 0 })
    expect(snap(48 + 100, 244 + 90)).toEqual({ col: 1, row: 1 })
    expect(snap(48 + 80, 244 + 120)).toEqual({ col: 0, row: 2 })
  })
  it('stays inside the grid', () => {
    expect(snap(-500, -500)).toEqual({ col: 0, row: 0 })
    expect(snap(5000, 5000)).toEqual({ col: 5, row: 7 })
  })
})
