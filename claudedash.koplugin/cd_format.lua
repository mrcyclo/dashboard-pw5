local format = {}

-- Indexed by os.date("*t").wday, where 1 is Sunday.
local WEEKDAYS = { "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday" }
local WEEKDAYS_SHORT = { "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat" }

local MINUTES_PER_DAY = 24 * 60
local DEFAULT_MEDIA_INTERVAL = 5
local MEDIA_BACKOFF_SECONDS = 30
local MEDIA_BACKOFF_AFTER_FAILURES = 3
-- Must match the step the PC applies per volume command.
local VOLUME_STEP = 5
-- Seconds to wait before asking again for a picture that failed, by number of failures.
local RETRY_DELAYS = { 60, 300, 900 }
-- Longer than a usage request can take, so two are never in flight.
local USAGE_RETRY_SECONDS = 30
local MAX_FULL_REFRESH_MINUTES = 24 * 60
-- Light and warmth arrive as Paperwhite 5 levels, 0 to 24; -1 means the PC leaves them alone.
local LIGHT_UNSET = -1
local MAX_LIGHT = 24
local MAX_MEDIA_INTERVAL_SECONDS = 300
-- The API reports limits that reset together a second or so apart.
local SAME_MOMENT_SECONDS = 120

-- KOReader's JSON decoder yields a function for JSON null, so a plain nil check is not enough.
local function number_or_nil(value)
    if type(value) ~= "number" then return nil end
    return value
end

local function window_or_nil(value)
    if type(value) ~= "table" then return nil end
    if not number_or_nil(value.used_percent) then return nil end
    return value
end

function format.text_or_nil(value)
    if type(value) ~= "string" or value == "" then return nil end
    return value
end

function format.clock(t)
    return string.format("%02d:%02d", t.hour, t.min)
end

function format.date_text(t)
    return string.format("%s, %02d/%02d/%04d", WEEKDAYS[t.wday], t.day, t.month, t.year)
end

local function model_windows(snapshot)
    local windows = {}
    if type(snapshot.models) ~= "table" then return windows end

    for _, entry in ipairs(snapshot.models) do
        local window = window_or_nil(entry)
        if window and format.text_or_nil(window.name) then
            table.insert(windows, window)
        end
    end
    return windows
end

function format.has_usage(snapshot)
    if type(snapshot) ~= "table" then return false end
    if window_or_nil(snapshot.five_hour) or window_or_nil(snapshot.seven_day) then return true end
    return #model_windows(snapshot) > 0
end

function format.percent(window, now)
    window = window_or_nil(window)
    if not window then return nil end

    local resets_at = number_or_nil(window.resets_at)
    if resets_at and resets_at <= now then return 0 end
    return window.used_percent
end

-- "resets in 2h 14m" until the unix time to, or nil once it has passed.
function format.countdown_text(to, now, date_fn)
    local resets_at = number_or_nil(to)
    if not resets_at or resets_at <= now then return nil end

    local minutes = math.ceil((resets_at - now) / 60)
    if minutes < 60 then
        return string.format("resets in %dm", minutes)
    end
    if minutes < MINUTES_PER_DAY then
        return string.format("resets in %dh %dm", math.floor(minutes / 60), minutes % 60)
    end

    local t = (date_fn or os.date)("*t", resets_at)
    return string.format("resets %s %s", WEEKDAYS_SHORT[t.wday], format.clock(t))
end

function format.updated_line(snapshot, fetch_failed, date_fn)
    if type(snapshot) ~= "table" then return nil end

    local fetched_at = number_or_nil(snapshot.fetched_at)
    if not fetched_at then return nil end

    local line = "Updated " .. format.clock((date_fn or os.date)("*t", fetched_at))
    if snapshot.stale == true or fetch_failed then
        return line .. " (stale)"
    end
    return line
end

local function row(label, window, now, date_fn)
    local percent = format.percent(window, now)
    local refresh = format.countdown_text(window.resets_at, now, date_fn)
    return {
        label = label,
        percent = percent,
        used = string.format("%d%% used", percent),
        refresh = refresh,
        -- Only set while the reset is still ahead, to compare rows by time rather than by text.
        resets_at = refresh and window.resets_at or nil,
    }
end

local function same_moment(a, b)
    if a == nil or b == nil then return a == b end
    return math.abs(a - b) < SAME_MOMENT_SECONDS
end

-- When every row refreshes at the same moment the time is shown once, in the header.
local function section(title, rows)
    local shared = rows[1]
    local result = { title = title, rows = rows }
    for _, r in ipairs(rows) do
        if not same_moment(r.resets_at, shared.resets_at) then
            return result
        end
    end
    result.refresh = shared.refresh
    for _, r in ipairs(rows) do
        r.refresh = nil
    end
    return result
end

function format.claude_sections(snapshot, now, date_fn)
    local sections = {}
    if type(snapshot) ~= "table" then return sections end

    local five_hour = window_or_nil(snapshot.five_hour)
    if five_hour then
        table.insert(sections, section("Claude · 5-hour limit", { row("All models", five_hour, now, date_fn) }))
    end

    local weekly = {}
    local seven_day = window_or_nil(snapshot.seven_day)
    if seven_day then
        table.insert(weekly, row("All models", seven_day, now, date_fn))
    end
    for _, window in ipairs(model_windows(snapshot)) do
        table.insert(weekly, row(window.name .. " only", window, now, date_fn))
    end
    if #weekly > 0 then
        table.insert(sections, section("Claude · weekly limits", weekly))
    end
    return sections
end

function format.is_active(media)
    return type(media) == "table" and media.active == true
end

function format.media_title(media, disconnected)
    local title = "Music"
    if format.is_active(media) then
        title = media.playing == true and "Playing" or "Paused"
        local app = format.text_or_nil(media.app)
        if app then
            title = title .. " · " .. app
        end
    end
    if disconnected then
        return title .. " (offline)"
    end
    return title
end

function format.volume_text(media)
    if type(media) ~= "table" then return nil end

    local volume = number_or_nil(media.volume)
    if not volume then return nil end
    if media.muted == true then return "Mute" end
    return tostring(volume)
end

-- Fill of the volume bar, 0 to 1. A muted output shows as empty.
function format.volume_fraction(media)
    if type(media) ~= "table" then return nil end

    local volume = number_or_nil(media.volume)
    if not volume then return nil end
    if media.muted == true then return 0 end
    return math.min(math.max(volume, 0), 100) / 100
end

local function visible_state(media)
    if type(media) ~= "table" then return nil end
    return {
        format.is_active(media),
        format.text_or_nil(media.title) or "",
        format.text_or_nil(media.artist) or "",
        format.text_or_nil(media.app) or "",
        media.playing == true,
        format.text_or_nil(media.art) or "",
        number_or_nil(media.volume) or -1,
        media.muted == true,
    }
end

function format.media_changed(old, new)
    local a, b = visible_state(old), visible_state(new)
    if a == nil or b == nil then return a ~= b end

    for i = 1, #a do
        if a[i] ~= b[i] then return true end
    end
    return false
end

local function copy(source)
    local result = {}
    for key, value in pairs(source) do
        result[key] = value
    end
    return result
end

-- What the screen should show the instant a button is tapped, before the PC
-- answers. Returns nil when the outcome cannot be predicted, as for a track change.
function format.optimistic(media, command)
    if type(media) ~= "table" then return nil end

    if command == "play-pause" then
        if not format.is_active(media) then return nil end
        local guess = copy(media)
        guess.playing = media.playing ~= true
        return guess
    end

    local delta = (command == "volume-up" and VOLUME_STEP) or (command == "volume-down" and -VOLUME_STEP) or nil
    if not delta then return nil end

    local guess = copy(media)
    local volume = number_or_nil(media.volume)
    if volume then
        guess.volume = math.min(math.max(volume + delta, 0), 100)
    end
    if delta > 0 and media.muted == true then
        guess.muted = false
    end
    return guess
end

-- pending = { playing = bool, expires = epoch }: the play state a recent tap asked for.
local function is_pending(pending, now)
    return type(pending) == "table" and now <= pending.expires
end

function format.is_settled(state, pending, now)
    if not is_pending(pending, now) then return true end
    return (state.playing == true) == pending.playing
end

-- Windows reports the new play state a moment after accepting the command, so a
-- reply that still shows the old one is corrected while the tap is fresh.
function format.settle(state, pending, now)
    if not is_pending(pending, now) or not format.is_active(state) then return state end

    local held = copy(state)
    held.playing = pending.playing
    return held
end

-- A picture that fails to download or decode is retried after a growing delay
-- instead of on every poll. failed = { hash, count, retry_at }.
function format.note_failure(failed, hash, now)
    local count = 1
    if failed and failed.hash == hash then
        count = failed.count + 1
    end
    local delay = RETRY_DELAYS[math.min(count, #RETRY_DELAYS)]
    return { hash = hash, count = count, retry_at = now + delay }
end

function format.retry_blocked(failed, hash, now)
    if not failed or failed.hash ~= hash then return false end
    return now < failed.retry_at
end

local function clamp(value, low, high)
    return math.min(math.max(value, low), high)
end

-- Display settings are chosen on the PC. Each field that is missing or of the
-- wrong type keeps its default, and every value is clamped to a usable range.
function format.display_settings(raw, defaults)
    local result = {
        panel_lighten = defaults.panel_lighten,
        full_refresh_minutes = defaults.full_refresh_minutes,
        media_interval_seconds = defaults.media_interval_seconds,
        frontlight = number_or_nil(defaults.frontlight) or LIGHT_UNSET,
        warmth = number_or_nil(defaults.warmth) or LIGHT_UNSET,
    }
    if type(raw) ~= "table" then return result end

    local frontlight = number_or_nil(raw.frontlight)
    if frontlight then
        result.frontlight = clamp(math.floor(frontlight), LIGHT_UNSET, MAX_LIGHT)
    end
    local warmth = number_or_nil(raw.warmth)
    if warmth then
        result.warmth = clamp(math.floor(warmth), LIGHT_UNSET, MAX_LIGHT)
    end

    local lighten = number_or_nil(raw.panel_lighten)
    if lighten then
        result.panel_lighten = clamp(lighten, 0, 1)
    end
    local refresh = number_or_nil(raw.full_refresh_minutes)
    if refresh then
        result.full_refresh_minutes = clamp(math.floor(refresh), 0, MAX_FULL_REFRESH_MINUTES)
    end
    local interval = number_or_nil(raw.media_interval_seconds)
    if interval then
        result.media_interval_seconds = clamp(math.floor(interval), 1, MAX_MEDIA_INTERVAL_SECONDS)
    end
    return result
end

-- Keeps a light level from the PC inside the device's own range.
function format.light_level(level, low, high)
    return clamp(level, low, high)
end

-- KOReader takes warmth as 0..100 on every device, whatever the hardware scale.
function format.warmth_percent(level)
    return math.floor(clamp(level, 0, MAX_LIGHT) * 100 / MAX_LIGHT + 0.5)
end

-- The PC asks for a full repaint by changing its refresh token. Returns
-- whether to repaint and the token to remember. A token already there when the
-- dashboard opens was meant for an earlier session; seen is nil until then.
function format.refresh_requested(seen, token)
    token = format.text_or_nil(token) or ""
    if seen == nil then return false, token end
    return token ~= "" and token ~= seen, token
end

-- True when usage should be asked for again right away: the last attempt got
-- no answer from the PC, and the PC is answering now. server_error is set when
-- the PC did answer but had no numbers, which asking again would not change.
function format.usage_retry_due(fetch_failed, server_error, asked_at, now)
    if not fetch_failed or server_error then return false end
    return asked_at == nil or now - asked_at >= USAGE_RETRY_SECONDS
end

-- A PC that stopped answering is polled less often, so a blocked request does
-- not freeze the screen every few seconds.
function format.next_media_delay(failures, config)
    local interval = number_or_nil(config.media_interval_seconds) or DEFAULT_MEDIA_INTERVAL
    if failures >= MEDIA_BACKOFF_AFTER_FAILURES then
        return math.max(interval, MEDIA_BACKOFF_SECONDS)
    end
    return interval
end

return format
