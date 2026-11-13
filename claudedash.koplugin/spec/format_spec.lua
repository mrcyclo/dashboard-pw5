local format = require("cd_format")

local now = 1794400000

local function fake_date(fmt, epoch)
    assert(fmt == "*t", "format must ask for a date table")
    assert(type(epoch) == "number", "format must pass the epoch")
    return { wday = 2, hour = 9, min = 0 }
end

local function eq(actual, expected)
    if actual ~= expected then
        error(string.format("expected %s, got %s", tostring(expected), tostring(actual)), 2)
    end
end

-- KOReader's JSON decoder turns JSON null into a function value.
local json_null = function() end

eq(format.clock({ hour = 4, min = 7 }), "04:07")
eq(format.clock({ hour = 14, min = 32 }), "14:32")

eq(format.date_text({ wday = 4, day = 11, month = 11, year = 2026 }), "Wednesday, 11/11/2026")
eq(format.date_text({ wday = 1, day = 1, month = 3, year = 2027 }), "Sunday, 01/03/2027")
eq(format.date_text({ wday = 7, day = 9, month = 1, year = 2027 }), "Saturday, 09/01/2027")

eq(format.countdown_text(nil, now), nil)
eq(format.countdown_text(nil, now), nil)
eq(format.countdown_text(json_null, now), nil)
eq(format.countdown_text(now - 5, now), nil)
eq(format.countdown_text(now, now), nil)
eq(format.countdown_text(now + 30, now), "resets in 1m")
eq(format.countdown_text(now + 14 * 60, now), "resets in 14m")
eq(format.countdown_text(now + 3600, now), "resets in 1h 0m")
eq(format.countdown_text(now + 2 * 3600 + 14 * 60, now), "resets in 2h 14m")
eq(format.countdown_text(now + 86400 - 60, now), "resets in 23h 59m")
eq(format.countdown_text(now + 86400, now, fake_date), "resets Mon 09:00")
-- A picture that failed to load is not requested again on every poll.
local failed = format.note_failure(nil, "h1", 1000)
eq(failed.hash, "h1")
eq(format.retry_blocked(failed, "h1", 1000), true)
eq(format.retry_blocked(failed, "h1", 1059), true)
eq(format.retry_blocked(failed, "h1", 1060), false)
-- A different picture is always worth a try at once.
eq(format.retry_blocked(failed, "h2", 1000), false)
eq(format.retry_blocked(nil, "h1", 1000), false)
-- Each further failure of the same picture waits longer, up to 15 minutes.
failed = format.note_failure(failed, "h1", 1060)
eq(format.retry_blocked(failed, "h1", 1060 + 299), true)
eq(format.retry_blocked(failed, "h1", 1060 + 300), false)
failed = format.note_failure(failed, "h1", 2000)
eq(format.retry_blocked(failed, "h1", 2000 + 899), true)
eq(format.retry_blocked(failed, "h1", 2000 + 900), false)
failed = format.note_failure(failed, "h1", 5000)
eq(format.retry_blocked(failed, "h1", 5000 + 900), false)
-- A new picture starts over at the short delay.
failed = format.note_failure(failed, "h9", 9000)
eq(format.retry_blocked(failed, "h9", 9060), false)
eq(format.retry_blocked(failed, "h9", 9059), true)

-- Display settings come from the PC; anything unusable falls back to the defaults.
local defaults = { panel_lighten = 0.7, full_refresh_minutes = 0, media_interval_seconds = 5 }
local d = format.display_settings({ panel_lighten = 0.85, full_refresh_minutes = 30, media_interval_seconds = 10 }, defaults)
eq(d.panel_lighten, 0.85)
eq(d.full_refresh_minutes, 30)
eq(d.media_interval_seconds, 10)

d = format.display_settings(nil, defaults)
eq(d.panel_lighten, 0.7)
eq(d.full_refresh_minutes, 0)
eq(d.media_interval_seconds, 5)

-- Each field is judged on its own.
d = format.display_settings({ panel_lighten = "bright", full_refresh_minutes = json_null, media_interval_seconds = 30 }, defaults)
eq(d.panel_lighten, 0.7)
eq(d.full_refresh_minutes, 0)
eq(d.media_interval_seconds, 30)

-- Values are clamped to what the dashboard can actually use.
d = format.display_settings({ panel_lighten = 4, full_refresh_minutes = -5, media_interval_seconds = 0 }, defaults)
eq(d.panel_lighten, 1)
eq(d.full_refresh_minutes, 0)
eq(d.media_interval_seconds, 1)
d = format.display_settings({ panel_lighten = -1, full_refresh_minutes = 99999, media_interval_seconds = 99999 }, defaults)
eq(d.panel_lighten, 0)
eq(d.full_refresh_minutes, 1440)
eq(d.media_interval_seconds, 300)
d = format.display_settings({ full_refresh_minutes = 15.7, media_interval_seconds = 3.2 }, defaults)
eq(d.full_refresh_minutes, 15)
eq(d.media_interval_seconds, 3)

-- The result is a new table, and the defaults are left alone.
d = format.display_settings({ panel_lighten = 0.2 }, defaults)
d.panel_lighten = 0.9
eq(defaults.panel_lighten, 0.7)

-- Light and warmth are levels 0..24; -1 leaves the Kindle's own setting alone.
d = format.display_settings(nil, defaults)
eq(d.frontlight, -1)
eq(d.warmth, -1)
d = format.display_settings({ frontlight = 10, warmth = 23.9 }, defaults)
eq(d.frontlight, 10)
eq(d.warmth, 23)
d = format.display_settings({ frontlight = 250, warmth = -9 }, d)
eq(d.frontlight, 24)
eq(d.warmth, -1)
d = format.display_settings({ frontlight = "on", warmth = json_null }, { frontlight = 7, warmth = 15 })
eq(d.frontlight, 7)
eq(d.warmth, 15)

-- A level is used as it is, kept inside what the device has.
eq(format.light_level(0, 0, 24), 0)
eq(format.light_level(24, 0, 24), 24)
eq(format.light_level(12, 0, 24), 12)
eq(format.light_level(24, 0, 11), 11)
eq(format.light_level(0, 1, 11), 1)

eq(format.warmth_percent(0), 0)
eq(format.warmth_percent(24), 100)
eq(format.warmth_percent(12), 50)
eq(format.warmth_percent(1), 4)
eq(format.warmth_percent(99), 100)

-- The first reply only records the token: it was meant for an earlier session.
local ask, seen = format.refresh_requested(nil, "t1")
eq(ask, false)
eq(seen, "t1")
ask, seen = format.refresh_requested(seen, "t1")
eq(ask, false)
ask, seen = format.refresh_requested(seen, "t2")
eq(ask, true)
eq(seen, "t2")
-- A restarted PC has no token; that is not a request.
ask, seen = format.refresh_requested(seen, nil)
eq(ask, false)
eq(seen, "")
ask, seen = format.refresh_requested(seen, "t3")
eq(ask, true)

-- KOReader's JSON null and empty strings are not text.
eq(format.text_or_nil(nil), nil)
eq(format.text_or_nil(json_null), nil)
eq(format.text_or_nil(""), nil)
eq(format.text_or_nil("Song"), "Song")

-- A PC that stopped answering is polled less often.
eq(format.next_media_delay(0, { media_interval_seconds = 5 }), 5)
eq(format.next_media_delay(2, { media_interval_seconds = 5 }), 5)
eq(format.next_media_delay(3, { media_interval_seconds = 5 }), 30)
eq(format.next_media_delay(9, { media_interval_seconds = 5 }), 30)
eq(format.next_media_delay(0, {}), 5)
eq(format.next_media_delay(0, { media_interval_seconds = 60 }), 60)
