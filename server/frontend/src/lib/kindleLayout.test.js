import { describe, expect, it } from 'vitest'
import { kindleZones } from './kindleLayout.js'

// The expected numbers are what cd_layout.lua's layout.zones(1236, 1648) returns.
describe('kindleZones', () => {
  const zones = kindleZones()

  it('puts the corner buttons where the plugin does', () => {
    expect(zones.exit).toEqual({ x: 1086, y: 0, w: 150, h: 150 })
    expect(zones.refresh).toEqual({ x: 936, y: 0, w: 150, h: 150 })
  })
  it('matches the media panel', () => {
    expect(zones.media_panel).toEqual({ x: 40, y: 1180, w: 1156, h: 428 })
    expect(zones.media_header).toEqual({ x: 64, y: 1204, w: 1108, h: 70 })
    expect(zones.cover).toEqual({ x: 64, y: 1284, w: 130, h: 130 })
    expect(zones.track_text).toEqual({ x: 214, y: 1274, w: 488, h: 150 })
    expect(zones.previous.x).toBe(722)
    expect(zones.play_pause.x).toBe(872)
    expect(zones.next.x).toBe(1022)
  })
  it('matches the volume row', () => {
    expect(zones.volume_down).toEqual({ x: 64, y: 1434, w: 150, h: 150 })
    expect(zones.volume_bar).toEqual({ x: 214, y: 1434, w: 678, h: 150 })
    expect(zones.volume_up.x).toBe(892)
    expect(zones.volume_value).toEqual({ x: 1042, y: 1434, w: 130, h: 150 })
  })
})
