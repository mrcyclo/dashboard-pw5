// Lays out a widget's component tree exactly like server/internal/tree/layout.go,
// so the window's preview places everything where the Kindle does. Both are
// checked against server/internal/widget/testdata/tree_fixtures.json; the rules
// are written out at the top of layout.go.
import { countdownText, dateText, timeText } from './format.js'

const KNOWN = new Set(['col', 'row', 'space', 'rule', 'text', 'clock', 'countdown', 'bar', 'icon', 'image', 'button'])
const TEXTUAL = new Set(['text', 'clock', 'countdown'])
const LINE_HEIGHT = 1.3

const num = (value, fallback) => (typeof value === 'number' ? value : fallback)
const str = (value) => (typeof value === 'string' ? value : '')
const known = (node) => !!node && typeof node === 'object' && KNOWN.has(node.kind)
const growOf = (node) => Math.max(num(node.grow, 0), 0)

export function resolve(node, now) {
  const at = node.at
  if (!at || typeof at !== 'object' || !at.set || typeof at.set !== 'object') return node
  const time = num(at.time, null)
  if (time === null || now < time) return node
  return { ...node, ...at.set }
}

function textOf(node, ctx) {
  if (node.kind === 'clock') {
    return node.format === 'date' ? dateText(ctx.now, ctx.clock) : timeText(ctx.now, ctx.clock)
  }
  if (node.kind === 'countdown') return countdownText(node.to, ctx.now, ctx.clock) ?? ''
  return str(node.text)
}

function childrenOf(node, ctx) {
  if (!Array.isArray(node.children)) return []
  return node.children.filter(known).map((child) => resolve(child, ctx.now))
}

function size(node, axis, ctx) {
  const kind = node.kind
  if (TEXTUAL.has(kind)) {
    const font = num(node.size, 20)
    return [ctx.measure(textOf(node, ctx), font, node.bold === true), Math.ceil(font * LINE_HEIGHT)]
  }
  if (kind === 'icon' || kind === 'button') {
    const side = num(node.size, 0)
    return [side, side]
  }
  if (kind === 'image') return [num(node.w, 0), num(node.h, 0)]
  if (kind === 'bar') return [0, num(node.height, 8)]
  if (kind === 'rule' || kind === 'space') {
    const amount = kind === 'rule' ? num(node.thickness, 2) : num(node.size, 0)
    return axis === 'row' ? [amount, 0] : [0, amount]
  }

  const kids = childrenOf(node, ctx)
  const gaps = Math.max(kids.length - 1, 0) * num(node.gap, 0)
  let w = 0
  let h = 0
  for (const child of kids) {
    const [cw, ch] = size(child, kind, ctx)
    if (kind === 'col') {
      w = Math.max(w, cw)
      h += ch
    } else {
      w += cw
      h = Math.max(h, ch)
    }
  }
  return kind === 'col' ? [w, h + gaps] : [w + gaps, h]
}

function placeCol(node, x, y, w, h, ctx) {
  const kids = childrenOf(node, ctx)
  const gap = num(node.gap, 0)
  const align = str(node.align) || 'stretch'
  const sizes = kids.map((child) => size(child, 'col', ctx))
  const used = sizes.reduce((sum, [, ch]) => sum + ch, 0)
  const weights = kids.reduce((sum, child) => sum + growOf(child), 0)
  const leftover = Math.max(h - used - Math.max(kids.length - 1, 0) * gap, 0)

  let cy = y
  kids.forEach((child, i) => {
    let ch = sizes[i][1]
    if (weights > 0) {
      ch += (leftover * growOf(child)) / weights
    }
    let cw = w
    let cx = x
    const stretches = align === 'stretch' || child.kind === 'bar' || child.kind === 'rule'
    if (!stretches) {
      cw = Math.min(sizes[i][0], w)
      if (align === 'center') {
        cx = x + (w - cw) / 2
      } else if (align === 'end') {
        cx = x + w - cw
      }
    }
    place(child, cx, cy, cw, ch, ctx)
    cy += ch + gap
  })
}

function placeRow(node, x, y, w, h, ctx) {
  const kids = childrenOf(node, ctx)
  const gap = num(node.gap, 0)
  const align = str(node.align) || 'center'
  const sizes = kids.map((child) => {
    const [cw, ch] = size(child, 'row', ctx)
    return [growOf(child) > 0 ? 0 : cw, ch]
  })
  const used = sizes.reduce((sum, [cw]) => sum + cw, 0)
  const weights = kids.reduce((sum, child) => sum + growOf(child), 0)
  const leftover = Math.max(w - used - Math.max(kids.length - 1, 0) * gap, 0)

  let cx = x
  kids.forEach((child, i) => {
    let cw = sizes[i][0]
    if (weights > 0) {
      cw += (leftover * growOf(child)) / weights
    }
    let ch = h
    let cy = y
    if (child.kind !== 'col' && child.kind !== 'rule') {
      ch = sizes[i][1]
      if (align === 'center') {
        cy = y + (h - ch) / 2
      } else if (align === 'end') {
        cy = y + h - ch
      }
    }
    place(child, cx, cy, cw, ch, ctx)
    cx += cw + gap
  })
}

function emit(node, x, y, w, h, ctx) {
  const box = ctx.root
  const x0 = Math.floor(x)
  const y0 = Math.floor(y)
  const left = Math.max(x0, box.x)
  const top = Math.max(y0, box.y)
  const right = Math.min(x0 + Math.floor(w), box.x + box.w)
  const bottom = Math.min(y0 + Math.floor(h), box.y + box.h)
  if (right <= left || bottom <= top) return
  if (TEXTUAL.has(node.kind) && bottom - top < Math.ceil(num(node.size, 20) * LINE_HEIGHT)) return

  const out = { node, x: left, y: top, w: right - left, h: bottom - top }
  if (TEXTUAL.has(node.kind)) {
    out.text = textOf(node, ctx)
  }
  ctx.boxes.push(out)
}

function place(node, x, y, w, h, ctx) {
  if (node.kind === 'col') {
    placeCol(node, x, y, w, h, ctx)
  } else if (node.kind === 'row') {
    placeRow(node, x, y, w, h, ctx)
  } else if (node.kind !== 'space') {
    emit(node, x, y, w, h, ctx)
  }
}

// Returns the drawable boxes in paint order: { node, x, y, w, h, text }.
// measure(text, size, bold) gives a text's width in pixels.
export function layout(root, box, measure, now, clock) {
  const ctx = { boxes: [], root: box, measure, now: now ?? Date.now() / 1000, clock }
  if (!known(root)) return ctx.boxes
  place(resolve(root, ctx.now), box.x, box.y, box.w, box.h, ctx)
  return ctx.boxes
}

export function find(node, id) {
  if (!node || typeof node !== 'object') return null
  if (node.id === id) return node
  for (const child of Array.isArray(node.children) ? node.children : []) {
    const found = find(child, id)
    if (found) return found
  }
  return null
}
