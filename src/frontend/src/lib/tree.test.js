import { describe, expect, it } from 'vitest'
import fixtures from '../../../internal/widget/testdata/tree_fixtures.json'
import { find, layout, resolve } from './tree.js'

// The same fake measurer the Lua spec uses with these fixtures.
const measure = (text, size) => Math.floor((text.length * size) / 2)

describe('layout matches the shared fixtures', () => {
  for (const fixture of fixtures) {
    it(fixture.name, () => {
      const box = { x: 0, y: 0, w: fixture.box.w, h: fixture.box.h }
      const placed = layout(fixture.tree, box, measure, 0).filter((b) => b.node.id)
      expect(placed.map((b) => ({ id: b.node.id, x: b.x, y: b.y, w: b.w, h: b.h }))).toEqual(fixture.expect)
      for (const b of layout(fixture.tree, box, measure, 0)) {
        expect(b.x >= 0 && b.y >= 0 && b.x + b.w <= box.w && b.y + b.h <= box.h).toBe(true)
      }
    })
  }
})

const now = 1794400000
const clock = () => ({ weekday: 1, hour: 9, minute: 5, day: 13, month: 11, year: 2026 })

describe('layout fills in time on the device', () => {
  it('draws clocks and countdowns', () => {
    const boxes = layout(
      {
        kind: 'col',
        children: [
          { kind: 'clock', format: 'time', size: 20 },
          { kind: 'clock', format: 'date', size: 20 },
          { kind: 'countdown', to: now + 90, size: 20 },
          { kind: 'countdown', to: now - 1, size: 20 },
        ],
      },
      { x: 0, y: 0, w: 400, h: 200 },
      measure,
      now,
      clock,
    )
    expect(boxes.map((b) => b.text)).toEqual(['09:05', 'Monday, 13/11/2026', 'resets in 2m', ''])
  })

  it('switches to the "at" values once their time has passed', () => {
    const node = { kind: 'text', text: '74% used', size: 10, at: { time: now, set: { text: '0% used' } } }
    const box = { x: 0, y: 0, w: 100, h: 20 }
    expect(layout(node, box, measure, now - 1)[0].text).toBe('74% used')
    expect(layout(node, box, measure, now)[0].text).toBe('0% used')
    expect(node.text).toBe('74% used')
    expect(resolve({ kind: 'bar', percent: 74, at: { time: now, set: { percent: 0 } } }, now + 5).percent).toBe(0)
  })

  it('treats missing values as defaults and a missing root as nothing', () => {
    const boxes = layout(
      { kind: 'col', gap: null, children: [{ kind: 'text', text: null, size: null }, null, { kind: 'bar', percent: 10, height: 4 }] },
      { x: 0, y: 0, w: 100, h: 100 },
      measure,
      now,
    )
    expect(boxes.length).toBe(2)
    expect(boxes[0].text).toBe('')
    expect(layout(null, { x: 0, y: 0, w: 10, h: 10 }, measure, now)).toEqual([])
  })

  it('offsets boxes by the root box position', () => {
    const boxes = layout({ kind: 'text', id: 't', text: 'Hi', size: 10 }, { x: 5, y: 7, w: 50, h: 20 }, measure, 0)
    expect(boxes[0]).toMatchObject({ x: 5, y: 7, w: 50, h: 20, text: 'Hi' })
  })
})

describe('find', () => {
  it('finds a node by id anywhere in the tree', () => {
    const root = { kind: 'row', children: [{ kind: 'col', children: [{ kind: 'button', id: 'pp', size: 10 }] }] }
    expect(find(root, 'pp').size).toBe(10)
    expect(find(root, 'nope')).toBeNull()
  })
})
