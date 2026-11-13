// The Kindle's widget grid, mirroring src/internal/grid/grid.go. The Go side
// is the authority; this copy lets the window disable choices that cannot work
// and snap a dragged widget before asking.

export const SCREEN = { width: 1236, height: 1648 }
export const COLS = 6
export const ROWS = 8
const MARGIN = 40
const GAP = 16
const CELL_W = (SCREEN.width - 2 * MARGIN) / COLS
const CELL_H = (SCREEN.height - 2 * MARGIN) / ROWS

export const NO_ROOM = 'No room for this size. Move or hide another widget first'

export function parseSize(size) {
  const match = /^(\d)x(\d)$/.exec(size ?? '')
  if (!match) return null
  const cols = Number(match[1])
  const rows = Number(match[2])
  if (cols < 1 || cols > COLS || rows < 1 || rows > ROWS) return null
  return { cols, rows }
}

export function cellRect(col, row, size) {
  return {
    x: Math.round(MARGIN + col * CELL_W + GAP / 2),
    y: Math.round(MARGIN + row * CELL_H + GAP / 2),
    w: Math.round(size.cols * CELL_W - GAP),
    h: Math.round(size.rows * CELL_H - GAP),
  }
}

// The grid lines, in reference pixels, for drawing.
export function gridLines() {
  const xs = Array.from({ length: COLS + 1 }, (_, i) => MARGIN + i * CELL_W)
  const ys = Array.from({ length: ROWS + 1 }, (_, i) => MARGIN + i * CELL_H)
  return { xs, ys, left: MARGIN, top: MARGIN, right: SCREEN.width - MARGIN, bottom: SCREEN.height - MARGIN }
}

export const reserved = (col, row) => row === 0 && col >= 3

export function fits(layout, kind, size, col, row) {
  if (col < 0 || row < 0 || col + size.cols > COLS || row + size.rows > ROWS) return false
  for (let c = col; c < col + size.cols; c++) {
    for (let r = row; r < row + size.rows; r++) {
      if (reserved(c, r)) return false
    }
  }
  return layout.every((other) => {
    if (other.kind === kind) return true
    const s = parseSize(other.size)
    if (!s) return true
    return !(col < other.col + s.cols && other.col < col + size.cols && row < other.row + s.rows && other.row < row + size.rows)
  })
}

function show(layout, kind, size) {
  const current = layout.find((p) => p.kind === kind)
  if (current && fits(layout, kind, size, current.col, current.row)) return { col: current.col, row: current.row }
  for (let r = 0; r < ROWS; r++) {
    for (let c = 0; c < COLS; c++) {
      if (fits(layout, kind, size, c, r)) return { col: c, row: r }
    }
  }
  return null
}

function resize(layout, kind, size) {
  const current = layout.find((p) => p.kind === kind) ?? { col: 0, row: 0 }
  if (fits(layout, kind, size, current.col, current.row)) return { col: current.col, row: current.row }
  let best = null
  for (let r = 0; r < ROWS; r++) {
    for (let c = 0; c < COLS; c++) {
      if (!fits(layout, kind, size, c, r)) continue
      const distance = Math.abs(c - current.col) + Math.abs(r - current.row)
      if (!best || distance < best.distance) {
        best = { col: c, row: r, distance }
      }
    }
  }
  return best && { col: best.col, row: best.row }
}

// canShow says whether choosing size for kind can work, and why not.
export function canShow(layout, kind, size) {
  if (size === 'none') return { ok: true }
  const wanted = parseSize(size)
  if (!wanted) return { ok: false, reason: 'Unknown widget or size' }
  const current = layout.find((p) => p.kind === kind)
  const place = current && current.size !== 'none' ? resize : show
  return place(layout, kind, wanted) ? { ok: true } : { ok: false, reason: NO_ROOM }
}

// snap turns a widget's dragged top-left corner, in reference pixels, into the nearest cell.
export function snap(x, y) {
  const col = Math.round((x - MARGIN - GAP / 2) / CELL_W)
  const row = Math.round((y - MARGIN - GAP / 2) / CELL_H)
  return { col: Math.min(Math.max(col, 0), COLS - 1), row: Math.min(Math.max(row, 0), ROWS - 1) }
}
