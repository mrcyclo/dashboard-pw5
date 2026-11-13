local format = {}

-- Indexed by os.date("*t").wday, where 1 is Sunday.
local WEEKDAYS = { "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday" }
local WEEKDAYS_SHORT = { "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat" }

local MINUTES_PER_DAY = 24 * 60
local DEFAULT_MEDIA_INTERVAL = 5
local MEDIA_BACKOFF_SECONDS = 30
local MEDIA_BACKOFF_AFTER_FAILURES = 3
-- Seconds to wait before asking again for a picture that failed, by number of failures.
local RETRY_DELAYS = { 60, 300, 900 }
local MAX_FULL_REFRESH_MINUTES = 24 * 60
-- Light and warmth arrive as Paperwhite 5 levels, 0 to 24; -1 means the PC leaves them alone.
local LIGHT_UNSET = -1
local MAX_LIGHT = 24
local MAX_MEDIA_INTERVAL_SECONDS = 300

-- KOReader's JSON decoder yields a function for JSON null, so a plain nil check is not enough.
local function number_or_nil(value)
    if type(value) ~= "number" then return nil end
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
