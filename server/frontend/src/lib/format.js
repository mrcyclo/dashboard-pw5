// Text and numbers shown in the window. The usage wording mirrors
// claudedash.koplugin/cd_format.lua so the window and the Kindle say the same thing.

const WEEKDAYS_SHORT = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const MINUTES_PER_DAY = 24 * 60
// The API reports limits that reset together a second or so apart.
const SAME_MOMENT_SECONDS = 120

// Breaks an epoch into local clock parts. Tests pass their own to stay independent of the time zone.
function localClock(epochSeconds) {
  const date = new Date(epochSeconds * 1000)
  return {
    weekday: date.getDay(),
    hour: date.getHours(),
    minute: date.getMinutes(),
    day: date.getDate(),
    month: date.getMonth() + 1,
    year: date.getFullYear(),
  }
}

function pad(value) {
  return String(value).padStart(2, '0')
}

function isWindow(value) {
  return !!value && typeof value.used_percent === 'number'
}

function resetsAt(window) {
  return typeof window.resets_at === 'number' ? window.resets_at : null
}

// "Friday, 13/11/2026", as the Kindle writes it.
export function dateText(epochSeconds, clock = localClock) {
  const t = clock(epochSeconds)
  return `${WEEKDAYS[t.weekday]}, ${pad(t.day)}/${pad(t.month)}/${t.year}`
}

export function timeText(epochSeconds, clock = localClock) {
  const t = clock(epochSeconds)
  return `${pad(t.hour)}:${pad(t.minute)}`
}

export function percentOf(window, now) {
  if (!isWindow(window)) return null
  const reset = resetsAt(window)
  if (reset !== null && reset <= now) return 0
  return window.used_percent
}

export function refreshText(window, now, clock = localClock) {
  if (!isWindow(window)) return null
  return countdownText(resetsAt(window), now, clock)
}

// "resets in 2h 14m" until the epoch to, or null once it has passed.
export function countdownText(to, now, clock = localClock) {
  const reset = typeof to === 'number' ? to : null
  if (reset === null || reset <= now) return null

  const minutes = Math.ceil((reset - now) / 60)
  if (minutes < 60) return `resets in ${minutes}m`
  if (minutes < MINUTES_PER_DAY) return `resets in ${Math.floor(minutes / 60)}h ${minutes % 60}m`

  const t = clock(reset)
  return `resets ${WEEKDAYS_SHORT[t.weekday]} ${pad(t.hour)}:${pad(t.minute)}`
}

function row(label, window, now, clock) {
  const percent = percentOf(window, now)
  const refresh = refreshText(window, now, clock)
  return { label, percent, used: `${percent}% used`, refresh, resetsAt: refresh ? resetsAt(window) : null }
}

function sameMoment(a, b) {
  if (a === null || b === null) return a === b
  return Math.abs(a - b) < SAME_MOMENT_SECONDS
}

// When every row refreshes at the same moment the time is shown once, on the section.
function section(title, rows) {
  const shared = rows.every((r) => sameMoment(r.resetsAt, rows[0].resetsAt))
  return {
    title,
    refresh: shared ? rows[0].refresh : null,
    rows: rows.map((r) => ({ label: r.label, percent: r.percent, used: r.used, refresh: shared ? null : r.refresh })),
  }
}

export function usageSections(snapshot, now, clock = localClock) {
  if (!snapshot) return []

  const sections = []
  if (isWindow(snapshot.five_hour)) {
    sections.push(section('Claude · 5-hour limit', [row('All models', snapshot.five_hour, now, clock)]))
  }

  const weekly = []
  if (isWindow(snapshot.seven_day)) {
    weekly.push(row('All models', snapshot.seven_day, now, clock))
  }
  for (const model of snapshot.models ?? []) {
    if (!isWindow(model) || !model.name) continue
    weekly.push(row(`${model.name} only`, model, now, clock))
  }
  if (weekly.length > 0) {
    sections.push(section('Claude · weekly limits', weekly))
  }
  return sections
}

export function tokenLabel(token, clock = localClock) {
  if (token?.status === 'valid') {
    return { text: `Token valid until ${timeText(token.expires_at, clock)}`, tone: 'ok' }
  }
  if (token?.status === 'expired') {
    return { text: 'Token expired, renews on the next poll', tone: 'warn' }
  }
  if (token?.status === 'login_required') {
    return { text: 'Sign in to Claude Code again', tone: 'error' }
  }
  return { text: 'Cannot read the token', tone: 'warn' }
}

export function agoText(seconds) {
  if (seconds < 1) return 'just now'
  if (seconds < 60) return `${Math.floor(seconds)}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  return `${Math.floor(seconds / 3600)}h ago`
}

export function kindleStatus(kindle, now, clock = localClock) {
  if (!kindle?.seen) {
    return { text: 'No device has connected yet', detail: null, tone: 'idle' }
  }
  if (kindle.connected) {
    return {
      text: `Connected · ${kindle.ip}`,
      detail: `Last request ${agoText(now - kindle.last_seen)}`,
      tone: 'ok',
    }
  }
  return { text: `Disconnected since ${timeText(kindle.last_seen, clock)}`, detail: kindle.ip, tone: 'warn' }
}

// Heading of the music card: "Playing · Spotify", "Paused · Chrome", or "Music".
export function mediaHeading(media) {
  if (!media?.active) return 'Music'
  const state = media.playing ? 'Playing' : 'Paused'
  return media.app ? `${state} · ${media.app}` : state
}

// Second line of the music card: artist and volume, whichever are known.
export function mediaLine(media) {
  const parts = []
  if (media?.artist) {
    parts.push(media.artist)
  }
  if (typeof media?.volume === 'number') {
    parts.push(media.muted ? 'muted' : `volume ${media.volume}`)
  }
  return parts.join(' · ')
}

function clampUnit(value) {
  return Math.min(Math.max(value, 0), 1)
}

// Turns a drag of the picture inside its frame into a new crop focus. The
// picture is scaled to cover the frame, so it can only slide along the axis
// where it is larger than the frame; dragging it right reveals more of its
// left side, which is a smaller focus.
export function focusFromDrag(start, dx, dy, image, frame, zoom = 1) {
  if (!image.w || !image.h) return { x: start.x, y: start.y }

  const scale = Math.max(frame.w / image.w, frame.h / image.h) * zoom
  const slackX = image.w * scale - frame.w
  const slackY = image.h * scale - frame.h
  return {
    x: slackX > 0.5 ? clampUnit(start.x - dx / slackX) : start.x,
    y: slackY > 0.5 ? clampUnit(start.y - dy / slackY) : start.y,
  }
}

export const MIN_ZOOM = 1
export const MAX_ZOOM = 5

// One notch of the mouse wheel changes the zoom by a tenth, kept to whole percents.
export function zoomFromWheel(zoom, deltaY) {
  if (!deltaY) return zoom
  const next = deltaY < 0 ? zoom * 1.1 : zoom / 1.1
  return Math.min(Math.max(Math.round(next * 100) / 100, MIN_ZOOM), MAX_ZOOM)
}
