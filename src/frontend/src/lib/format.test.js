import { describe, expect, it } from 'vitest'
import {
  agoText,
  focusFromDrag,
  zoomFromWheel,
  mediaHeading,
  mediaLine,
  kindleStatus,
  percentOf,
  refreshText,
  timeText,
  tokenLabel,
  usageSections,
} from './format.js'

const now = 1794400020
// A fixed clock reading, so the tests do not depend on the machine's time zone.
const monday9 = () => ({ weekday: 1, hour: 9, minute: 0 })

describe('percentOf', () => {
  it('reads the used percentage', () => {
    expect(percentOf({ used_percent: 42, resets_at: now + 60 }, now)).toBe(42)
    expect(percentOf({ used_percent: 42 }, now)).toBe(42)
  })
  it('is zero once the reset time has passed', () => {
    expect(percentOf({ used_percent: 42, resets_at: now }, now)).toBe(0)
    expect(percentOf({ used_percent: 42, resets_at: now - 999 }, now)).toBe(0)
  })
  it('is null without a usable window', () => {
    expect(percentOf(null, now)).toBeNull()
    expect(percentOf(undefined, now)).toBeNull()
    expect(percentOf({ resets_at: now + 60 }, now)).toBeNull()
  })
})

describe('refreshText', () => {
  it('counts down under a day', () => {
    expect(refreshText({ used_percent: 1, resets_at: now + 30 }, now)).toBe('resets in 1m')
    expect(refreshText({ used_percent: 1, resets_at: now + 14 * 60 }, now)).toBe('resets in 14m')
    expect(refreshText({ used_percent: 1, resets_at: now + 3600 }, now)).toBe('resets in 1h 0m')
    expect(refreshText({ used_percent: 1, resets_at: now + 2 * 3600 + 14 * 60 }, now)).toBe('resets in 2h 14m')
    expect(refreshText({ used_percent: 1, resets_at: now + 86400 - 60 }, now)).toBe('resets in 23h 59m')
  })
  it('names the weekday and time from a day out', () => {
    expect(refreshText({ used_percent: 1, resets_at: now + 86400 }, now, monday9)).toBe('resets Mon 09:00')
    expect(refreshText({ used_percent: 1, resets_at: now + 3 * 86400 }, now, () => ({ weekday: 0, hour: 3, minute: 5 }))).toBe(
      'resets Sun 03:05',
    )
  })
  it('is null when there is nothing ahead to wait for', () => {
    expect(refreshText(null, now)).toBeNull()
    expect(refreshText({ used_percent: 1 }, now)).toBeNull()
    expect(refreshText({ used_percent: 1, resets_at: now }, now)).toBeNull()
    expect(refreshText({ used_percent: 1, resets_at: now - 5 }, now)).toBeNull()
  })
})

describe('usageSections', () => {
  const week = now + 3 * 86400

  it('groups the weekly limits and shares one refresh time', () => {
    const sections = usageSections(
      { seven_day: { used_percent: 28, resets_at: week + 1 }, models: [{ name: 'Fable', used_percent: 6, resets_at: week }] },
      now,
      monday9,
    )
    expect(sections).toEqual([
      {
        title: 'Claude · weekly limits',
        refresh: 'resets Mon 09:00',
        rows: [
          { label: 'All models', percent: 28, used: '28% used', refresh: null },
          { label: 'Fable only', percent: 6, used: '6% used', refresh: null },
        ],
      },
    ])
  })

  it('keeps separate refresh times on the rows when they differ', () => {
    const [section] = usageSections(
      { seven_day: { used_percent: 28, resets_at: now + 3600 }, models: [{ name: 'Fable', used_percent: 6, resets_at: now + 7200 }] },
      now,
    )
    expect(section.refresh).toBeNull()
    expect(section.rows.map((r) => r.refresh)).toEqual(['resets in 1h 0m', 'resets in 2h 0m'])
  })

  it('puts a five hour window in its own section, first', () => {
    const sections = usageSections(
      { five_hour: { used_percent: 42, resets_at: now + 14 * 60 }, seven_day: { used_percent: 28, resets_at: week } },
      now,
      monday9,
    )
    expect(sections.map((s) => s.title)).toEqual(['Claude · 5-hour limit', 'Claude · weekly limits'])
    expect(sections[0].refresh).toBe('resets in 14m')
    expect(sections[0].rows).toEqual([{ label: 'All models', percent: 42, used: '42% used', refresh: null }])
  })

  it('shows a window as empty once its reset has passed', () => {
    const [section] = usageSections({ seven_day: { used_percent: 80, resets_at: now - 10 } }, now)
    expect(section.rows[0]).toEqual({ label: 'All models', percent: 0, used: '0% used', refresh: null })
    expect(section.refresh).toBeNull()
  })

  it('never throws on missing or odd data', () => {
    expect(usageSections(null, now)).toEqual([])
    expect(usageSections({ stale: true, error: 'x' }, now)).toEqual([])
    expect(usageSections({ five_hour: null, seven_day: null, models: null }, now)).toEqual([])
    const [section] = usageSections(
      { seven_day: { used_percent: 1 }, models: [{ name: '', used_percent: 6 }, { name: 'NoPercent' }, null] },
      now,
    )
    expect(section.rows).toHaveLength(1)
  })
})

describe('tokenLabel', () => {
  const clock = () => ({ weekday: 3, hour: 18, minute: 0 })
  it('describes each token state', () => {
    expect(tokenLabel({ status: 'valid', expires_at: now + 3600 }, clock)).toEqual({
      text: 'Token valid until 18:00',
      tone: 'ok',
    })
    expect(tokenLabel({ status: 'expired', expires_at: now - 60 }, clock)).toEqual({
      text: 'Token expired, renews on the next poll',
      tone: 'warn',
    })
    expect(tokenLabel({ status: 'login_required' }, clock)).toEqual({ text: 'Sign in to Claude Code again', tone: 'error' })
    expect(tokenLabel({ status: 'unknown' }, clock)).toEqual({ text: 'Cannot read the token', tone: 'warn' })
    expect(tokenLabel(null, clock)).toEqual({ text: 'Cannot read the token', tone: 'warn' })
  })
})

describe('kindleStatus', () => {
  const clock = () => ({ weekday: 3, hour: 14, minute: 34 })
  it('describes a connected Kindle', () => {
    expect(kindleStatus({ seen: true, connected: true, ip: '10.0.0.114', last_seen: now - 3 }, now, clock)).toEqual({
      text: 'Connected · 10.0.0.114',
      detail: 'Last request 3s ago',
      tone: 'ok',
    })
  })
  it('describes a Kindle that went quiet', () => {
    expect(kindleStatus({ seen: true, connected: false, ip: '10.0.0.114', last_seen: now - 5000 }, now, clock)).toEqual({
      text: 'Disconnected since 14:34',
      detail: '10.0.0.114',
      tone: 'warn',
    })
  })
  it('describes having seen nothing yet', () => {
    const none = { text: 'No device has connected yet', detail: null, tone: 'idle' }
    expect(kindleStatus({ seen: false, connected: false }, now, clock)).toEqual(none)
    expect(kindleStatus(null, now, clock)).toEqual(none)
  })
})

describe('agoText', () => {
  it('uses the largest sensible unit', () => {
    expect(agoText(0)).toBe('just now')
    expect(agoText(1)).toBe('1s ago')
    expect(agoText(59)).toBe('59s ago')
    expect(agoText(60)).toBe('1m ago')
    expect(agoText(3599)).toBe('59m ago')
    expect(agoText(3600)).toBe('1h ago')
    expect(agoText(-5)).toBe('just now')
  })
})

describe('timeText', () => {
  it('pads hours and minutes', () => {
    expect(timeText(now, () => ({ weekday: 0, hour: 4, minute: 7 }))).toBe('04:07')
  })
})

describe('focusFromDrag', () => {
  // A 2000x1000 picture in a 300x400 frame is scaled to 800x400: 500 px of slack sideways, none vertically.
  const image = { w: 2000, h: 1000 }
  const frame = { w: 300, h: 400 }

  it('moves the focus against the drag', () => {
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, 250, 0, image, frame)).toEqual({ x: 0, y: 0.5 })
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, -250, 0, image, frame)).toEqual({ x: 1, y: 0.5 })
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, -50, 0, image, frame).x).toBeCloseTo(0.6)
  })
  it('clamps at the edges', () => {
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, 9999, 0, image, frame).x).toBe(0)
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, -9999, 0, image, frame).x).toBe(1)
  })
  it('ignores a drag along an axis with no slack', () => {
    expect(focusFromDrag({ x: 0.5, y: 0.5 }, 0, 120, image, frame)).toEqual({ x: 0.5, y: 0.5 })
  })
  it('works for a tall picture too', () => {
    // 600x2400 in 300x400 is scaled to 300x1200: 800 px of slack vertically.
    const moved = focusFromDrag({ x: 0.5, y: 0.5 }, 30, -200, { w: 600, h: 2400 }, frame)
    expect(moved.x).toBe(0.5)
    expect(moved.y).toBeCloseTo(0.75)
  })
  it('has room on both axes once zoomed in', () => {
    // At zoom 2 the picture is 1600x800 in the 300x400 frame: 1300 px of slack sideways, 400 vertically.
    const moved = focusFromDrag({ x: 0.5, y: 0.5 }, -130, 100, image, frame, 2)
    expect(moved.x).toBeCloseTo(0.6)
    expect(moved.y).toBeCloseTo(0.25)
  })
  it('survives a picture that has not loaded yet', () => {
    expect(focusFromDrag({ x: 0.3, y: 0.7 }, 40, 40, { w: 0, h: 0 }, frame)).toEqual({ x: 0.3, y: 0.7 })
  })
})

describe('mediaHeading', () => {
  it('names the state and the app', () => {
    expect(mediaHeading({ active: true, playing: true, app: 'Spotify' })).toBe('Playing · Spotify')
    expect(mediaHeading({ active: true, playing: false, app: 'Chrome' })).toBe('Paused · Chrome')
    expect(mediaHeading({ active: true, app: 'Chrome' })).toBe('Paused · Chrome')
    expect(mediaHeading({ active: true, playing: true })).toBe('Playing')
  })
  it('falls back when nothing is playing', () => {
    expect(mediaHeading({ active: false })).toBe('Music')
    expect(mediaHeading(null)).toBe('Music')
  })
})

describe('mediaLine', () => {
  it('joins what is known', () => {
    expect(mediaLine({ artist: 'Ava Max', volume: 22, muted: false })).toBe('Ava Max · volume 22')
    expect(mediaLine({ artist: 'Ava Max' })).toBe('Ava Max')
    expect(mediaLine({ volume: 0 })).toBe('volume 0')
    expect(mediaLine({ artist: 'Ava Max', volume: 22, muted: true })).toBe('Ava Max · muted')
    expect(mediaLine({})).toBe('')
    expect(mediaLine(null)).toBe('')
  })
})

describe('zoomFromWheel', () => {
  it('zooms in when the wheel rolls up and out when it rolls down', () => {
    expect(zoomFromWheel(1, -100)).toBe(1.1)
    expect(zoomFromWheel(2, 100)).toBe(1.82)
  })
  it('stays between 100% and 500%', () => {
    expect(zoomFromWheel(1, 100)).toBe(1)
    expect(zoomFromWheel(4.9, -100)).toBe(5)
    expect(zoomFromWheel(5, -100)).toBe(5)
  })
  it('ignores a sideways scroll', () => {
    expect(zoomFromWheel(2.5, 0)).toBe(2.5)
  })
})
