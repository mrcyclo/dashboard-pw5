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

eq(format.percent(nil, now), nil)
eq(format.percent({ used_percent = 42, resets_at = now + 60 }, now), 42)
eq(format.percent({ used_percent = 42 }, now), 42)
eq(format.percent({ used_percent = 42, resets_at = now }, now), 0)
eq(format.percent({ used_percent = 42, resets_at = now - 999 }, now), 0)
eq(format.percent({ resets_at = now + 60 }, now), nil)
eq(format.percent({ used_percent = 42, resets_at = json_null }, now), 42)
eq(format.percent({ used_percent = json_null }, now), nil)

eq(format.refresh_text(nil, now), nil)
eq(format.refresh_text({ used_percent = 1 }, now), nil)
eq(format.refresh_text({ used_percent = 1, resets_at = json_null }, now), nil)
eq(format.refresh_text({ used_percent = 1, resets_at = now - 5 }, now), nil)
eq(format.refresh_text({ used_percent = 1, resets_at = now }, now), nil)
eq(format.refresh_text({ used_percent = 1, resets_at = now + 30 }, now), "resets in 1m")
eq(format.refresh_text({ used_percent = 1, resets_at = now + 14 * 60 }, now), "resets in 14m")
eq(format.refresh_text({ used_percent = 1, resets_at = now + 3600 }, now), "resets in 1h 0m")
eq(format.refresh_text({ used_percent = 1, resets_at = now + 2 * 3600 + 14 * 60 }, now), "resets in 2h 14m")
eq(format.refresh_text({ used_percent = 1, resets_at = now + 86400 - 60 }, now), "resets in 23h 59m")
eq(format.refresh_text({ used_percent = 1, resets_at = now + 86400 }, now, fake_date), "resets Mon 09:00")

eq(format.updated_line(nil, false, fake_date), nil)
eq(format.updated_line({}, false, fake_date), nil)
eq(format.updated_line({ fetched_at = json_null }, false, fake_date), nil)
eq(format.updated_line({ fetched_at = now }, false, fake_date), "Updated 09:00")
eq(format.updated_line({ fetched_at = now, stale = true }, false, fake_date), "Updated 09:00 (stale)")
eq(format.updated_line({ fetched_at = now }, true, fake_date), "Updated 09:00 (stale)")

eq(format.has_usage(nil), false)
eq(format.has_usage({ stale = true, error = "x" }), false)
eq(format.has_usage({ five_hour = json_null }), false)
eq(format.has_usage({ five_hour = { used_percent = 1 } }), true)
eq(format.has_usage({ seven_day = { used_percent = 1 } }), true)
eq(format.has_usage({ models = { { name = "Fable", used_percent = 1 } } }), true)

-- Claude sections: the real shape, one weekly limit plus one model, same reset.
local week = now + 3 * 86400
local sections = format.claude_sections({
    seven_day = { used_percent = 28, resets_at = week },
    models = { { name = "Fable", used_percent = 6, resets_at = week + 20 } },
}, now, fake_date)
eq(#sections, 1)
eq(sections[1].title, "Claude · weekly limits")
eq(sections[1].refresh, "resets Mon 09:00")
eq(#sections[1].rows, 2)
eq(sections[1].rows[1].label, "All models")
eq(sections[1].rows[1].percent, 28)
eq(sections[1].rows[1].used, "28% used")
eq(sections[1].rows[1].refresh, nil)
eq(sections[1].rows[2].label, "Fable only")
eq(sections[1].rows[2].used, "6% used")
eq(sections[1].rows[2].refresh, nil)

-- Different reset times: each row carries its own, the header carries none.
sections = format.claude_sections({
    seven_day = { used_percent = 28, resets_at = now + 3600 },
    models = { { name = "Fable", used_percent = 6, resets_at = now + 7200 } },
}, now, fake_date)
eq(sections[1].refresh, nil)
eq(sections[1].rows[1].refresh, "resets in 1h 0m")
eq(sections[1].rows[2].refresh, "resets in 2h 0m")

-- A five hour window gets its own section, listed first.
sections = format.claude_sections({
    five_hour = { used_percent = 42, resets_at = now + 14 * 60 },
    seven_day = { used_percent = 28, resets_at = week },
}, now, fake_date)
eq(#sections, 2)
eq(sections[1].title, "Claude · 5-hour limit")
eq(sections[1].refresh, "resets in 14m")
eq(sections[1].rows[1].label, "All models")
eq(sections[1].rows[1].percent, 42)
eq(sections[2].title, "Claude · weekly limits")

-- A window whose reset has passed shows as empty with no refresh text.
sections = format.claude_sections({ seven_day = { used_percent = 80, resets_at = now - 10 } }, now, fake_date)
eq(sections[1].rows[1].percent, 0)
eq(sections[1].rows[1].used, "0% used")
eq(sections[1].refresh, nil)
eq(sections[1].rows[1].refresh, nil)

-- Models without an overall weekly limit still form the weekly section.
sections = format.claude_sections({ models = { { name = "Opus", used_percent = 9 } } }, now, fake_date)
eq(#sections, 1)
eq(sections[1].rows[1].label, "Opus only")

-- Unusable input never raises.
eq(#format.claude_sections(nil, now), 0)
eq(#format.claude_sections({ stale = true, error = "x" }, now), 0)
eq(#format.claude_sections({ five_hour = json_null, seven_day = json_null, models = json_null }, now), 0)
sections = format.claude_sections({
    seven_day = { used_percent = 1 },
    models = { { name = json_null, used_percent = 6 }, { name = "", used_percent = 6 }, { name = "NoPercent" }, json_null },
}, now, fake_date)
eq(#sections[1].rows, 1)

-- Media header.
eq(format.media_title(nil, false), "Music")
eq(format.media_title({ active = false }, false), "Music")
eq(format.media_title({ active = true, playing = true, app = "Spotify" }, false), "Playing · Spotify")
eq(format.media_title({ active = true, playing = false, app = "Chrome" }, false), "Paused · Chrome")
eq(format.media_title({ active = true, app = "Chrome" }, false), "Paused · Chrome")
eq(format.media_title({ active = true, playing = true }, false), "Playing")
eq(format.media_title({ active = true, playing = true, app = json_null }, false), "Playing")
eq(format.media_title({ active = true, playing = true, app = "Spotify" }, true), "Playing · Spotify (offline)")
eq(format.media_title(nil, true), "Music (offline)")

eq(format.volume_text(nil), nil)
eq(format.volume_text({ active = false }), nil)
eq(format.volume_text({ volume = 60, muted = false }), "60")
eq(format.volume_text({ volume = 0 }), "0")
eq(format.volume_text({ volume = 60, muted = true }), "Mute")

eq(format.volume_fraction(nil), nil)
eq(format.volume_fraction({ volume = json_null }), nil)
eq(format.volume_fraction({ volume = 60 }), 0.6)
eq(format.volume_fraction({ volume = 0 }), 0)
eq(format.volume_fraction({ volume = 100 }), 1)
eq(format.volume_fraction({ volume = 60, muted = true }), 0)
eq(format.volume_text({ volume = json_null }), nil)

eq(format.is_active(nil), false)
eq(format.is_active({ active = json_null }), false)
eq(format.is_active({ active = true }), true)

eq(format.text_or_nil(nil), nil)
eq(format.text_or_nil(json_null), nil)
eq(format.text_or_nil(""), nil)
eq(format.text_or_nil("Song"), "Song")

-- Redraw only when something visible changed.
local base = { active = true, title = "A", artist = "B", app = "C", playing = true, art = "h1", volume = 60, muted = false }
local function with(field, value)
    local copy = {}
    for k, v in pairs(base) do copy[k] = v end
    copy[field] = value
    return copy
end
eq(format.media_changed(base, with("title", "A")), false)
eq(format.media_changed(nil, nil), false)
eq(format.media_changed(nil, base), true)
eq(format.media_changed(base, nil), true)
eq(format.media_changed(base, with("title", "Z")), true)
eq(format.media_changed(base, with("artist", "Z")), true)
eq(format.media_changed(base, with("app", "Z")), true)
eq(format.media_changed(base, with("playing", false)), true)
eq(format.media_changed(base, with("art", "h2")), true)
eq(format.media_changed(base, with("volume", 65)), true)
eq(format.media_changed(base, with("muted", true)), true)
eq(format.media_changed(base, with("active", false)), true)
eq(format.media_changed(with("playing", nil), with("playing", false)), false)

eq(format.next_media_delay(0, { media_interval_seconds = 5 }), 5)
eq(format.next_media_delay(2, { media_interval_seconds = 5 }), 5)
eq(format.next_media_delay(3, { media_interval_seconds = 5 }), 30)
eq(format.next_media_delay(9, { media_interval_seconds = 5 }), 30)
eq(format.next_media_delay(0, {}), 5)
eq(format.next_media_delay(0, { media_interval_seconds = 60 }), 60)
eq(format.next_media_delay(3, { media_interval_seconds = 60 }), 60)

-- Real data: the two weekly limits reset one second apart. Drawn on a minute
-- boundary, per-row countdowns would differ by a minute and split the header.
local on_minute = 1794400020
assert(on_minute % 60 == 0)
local soon = on_minute + 2 * 3600 + 14 * 60
sections = format.claude_sections({
    seven_day = { used_percent = 28, resets_at = soon + 1 },
    models = { { name = "Fable", used_percent = 6, resets_at = soon } },
}, on_minute, fake_date)
eq(sections[1].refresh, "resets in 2h 15m")
eq(sections[1].rows[1].refresh, nil)
eq(sections[1].rows[2].refresh, nil)

-- Two minutes apart is a real difference and stays per row.
sections = format.claude_sections({
    seven_day = { used_percent = 28, resets_at = soon },
    models = { { name = "Fable", used_percent = 6, resets_at = soon + 180 } },
}, on_minute, fake_date)
eq(sections[1].refresh, nil)
eq(sections[1].rows[1].refresh, "resets in 2h 14m")

-- One row already reset while the other has not: they do not share a time.
sections = format.claude_sections({
    seven_day = { used_percent = 28, resets_at = on_minute - 5 },
    models = { { name = "Fable", used_percent = 6, resets_at = soon } },
}, on_minute, fake_date)
eq(sections[1].refresh, nil)
eq(sections[1].rows[1].refresh, nil)
eq(sections[1].rows[2].refresh, "resets in 2h 14m")

-- A tap is shown at once, before the PC answers.
local shown = { active = true, title = "A", playing = true, volume = 60, muted = false }
local guess = format.optimistic(shown, "play-pause")
eq(guess.playing, false)
eq(guess.title, "A")
eq(shown.playing, true)
eq(format.optimistic(guess, "play-pause").playing, true)
eq(format.optimistic({ active = true }, "play-pause").playing, true)

eq(format.optimistic(shown, "volume-up").volume, 65)
eq(format.optimistic(shown, "volume-down").volume, 55)
eq(format.optimistic({ volume = 98 }, "volume-up").volume, 100)
eq(format.optimistic({ volume = 3 }, "volume-down").volume, 0)
eq(format.optimistic({ volume = 20, muted = true }, "volume-up").muted, false)
eq(format.optimistic({ volume = 20, muted = true }, "volume-down").muted, true)
eq(format.optimistic({ volume = json_null }, "volume-up").volume, json_null)

-- Nothing can be predicted for a track change, or without a state to start from.
eq(format.optimistic(shown, "next"), nil)
eq(format.optimistic(shown, "previous"), nil)
eq(format.optimistic(nil, "play-pause"), nil)
eq(format.optimistic({ active = false }, "play-pause"), nil)
eq(format.optimistic({ active = false, volume = 10 }, "volume-up").volume, 15)

-- A reply that still shows the old play state is corrected while the tap is fresh.
local pending = { playing = false, expires = 100 }
eq(format.settle({ active = true, playing = true }, pending, 99).playing, false)
eq(format.settle({ active = true, playing = true }, pending, 101).playing, true)
eq(format.settle({ active = true, playing = false }, pending, 99).playing, false)
eq(format.settle({ active = true, playing = true }, nil, 99).playing, true)
eq(format.settle({ active = false }, pending, 99).playing, nil)
eq(format.is_settled({ active = true, playing = false }, pending, 99), true)
eq(format.is_settled({ active = true, playing = true }, pending, 99), false)
eq(format.is_settled({ active = true, playing = true }, pending, 101), true)
eq(format.is_settled({ active = true, playing = true }, nil, 99), true)

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
-- An older PC app never sends one.
ask, seen = format.refresh_requested(nil, json_null)
eq(ask, false)
eq(seen, "")

-- Usage is polled every few minutes. When the last attempt never reached the
-- PC and the PC now answers the media poll, usage is asked for again at once.
eq(format.usage_retry_due(true, nil, 1000, 1030), true)
eq(format.usage_retry_due(true, nil, nil, 1030), true)
-- Not while the last attempt is still recent, or it would be asked every media poll.
eq(format.usage_retry_due(true, nil, 1000, 1029), false)
-- Not when usage is fine, or when the PC answered with an error of its own.
eq(format.usage_retry_due(false, nil, 1000, 2000), false)
eq(format.usage_retry_due(true, "Cannot reach Claude", 1000, 2000), false)
