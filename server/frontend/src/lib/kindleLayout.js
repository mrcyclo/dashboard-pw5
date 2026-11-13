// The Paperwhite 5 screen. Every number below mirrors claudedash.koplugin/cd_layout.lua
// at that size, so the preview places things where the Kindle does.
export const KINDLE = { width: 1236, height: 1648 }

const EDGE_MARGIN = 40
const PANEL_PADDING = 24
const EXIT_SIDE = 150
const BUTTON_SIDE = 150
const ROW_GAP = 10
const HEADER_HEIGHT = 70
const COVER_SIDE = 130
const TEXT_GAP = 20
const VOLUME_VALUE_W = 130

const rect = (x, y, w, h) => ({ x, y, w, h })

export function kindleZones() {
  const { width, height } = KINDLE
  const panelX = EDGE_MARGIN
  const panelW = width - 2 * EDGE_MARGIN
  const innerX = panelX + PANEL_PADDING
  const innerW = panelW - 2 * PANEL_PADDING
  const innerRight = innerX + innerW

  const volumeY = height - EDGE_MARGIN - PANEL_PADDING - BUTTON_SIDE
  const playbackY = volumeY - ROW_GAP - BUTTON_SIDE
  const headerY = playbackY - HEADER_HEIGHT
  const mediaPanelY = headerY - PANEL_PADDING

  const nextX = innerRight - BUTTON_SIDE
  const previousX = nextX - 2 * BUTTON_SIDE
  const textX = innerX + COVER_SIDE + TEXT_GAP
  const volumeUpX = innerRight - VOLUME_VALUE_W - BUTTON_SIDE
  const volumeBarX = innerX + BUTTON_SIDE
  const exitX = width - EXIT_SIDE

  return {
    padding: PANEL_PADDING,
    exit: rect(exitX, 0, EXIT_SIDE, EXIT_SIDE),
    refresh: rect(exitX - EXIT_SIDE, 0, EXIT_SIDE, EXIT_SIDE),
    previous: rect(previousX, playbackY, BUTTON_SIDE, BUTTON_SIDE),
    play_pause: rect(nextX - BUTTON_SIDE, playbackY, BUTTON_SIDE, BUTTON_SIDE),
    next: rect(nextX, playbackY, BUTTON_SIDE, BUTTON_SIDE),
    volume_down: rect(innerX, volumeY, BUTTON_SIDE, BUTTON_SIDE),
    volume_up: rect(volumeUpX, volumeY, BUTTON_SIDE, BUTTON_SIDE),
    // The top panel grows with its content, so it has no fixed height here.
    top_panel: rect(panelX, EDGE_MARGIN, panelW, 0),
    media_panel: rect(panelX, mediaPanelY, panelW, height - EDGE_MARGIN - mediaPanelY),
    media_header: rect(innerX, headerY, innerW, HEADER_HEIGHT),
    cover: rect(innerX, playbackY + (BUTTON_SIDE - COVER_SIDE) / 2, COVER_SIDE, COVER_SIDE),
    track_text: rect(textX, playbackY, previousX - TEXT_GAP - textX, BUTTON_SIDE),
    volume_bar: rect(volumeBarX, volumeY, volumeUpX - volumeBarX, BUTTON_SIDE),
    volume_value: rect(innerRight - VOLUME_VALUE_W, volumeY, VOLUME_VALUE_W, BUTTON_SIDE),
  }
}
