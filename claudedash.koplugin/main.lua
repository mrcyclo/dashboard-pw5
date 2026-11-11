local DataStorage = require("datastorage")
local Device = require("device")
local Geom = require("ui/geometry")
local GestureRange = require("ui/gesturerange")
local InputContainer = require("ui/widget/container/inputcontainer")
local NetworkMgr = require("ui/network/manager")
local PluginShare = require("pluginshare")
local RenderImage = require("ui/renderimage")
local UIManager = require("ui/uimanager")
local ffiutil = require("ffi/util")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local logger = require("logger")
local _ = require("gettext")

local config = require("cd_config")
local format = require("cd_format")
local layout = require("cd_layout")
local net = require("cd_net")
local view = require("cd_view")

local Screen = Device.screen

local PLUGIN_DIR = debug.getinfo(1, "S").source:match("^@(.*)/[^/]*$") or "."

-- The picture is kept on the Kindle so it is still there when the PC is off.
local BACKGROUND_FILE = DataStorage:getDataDir() .. "/claudedash-background.png"
local BACKGROUND_HASH_FILE = BACKGROUND_FILE .. ".hash"

-- Socket timeouts, in seconds. Requests run off the UI thread (see cd_net), so
-- these bound how stale the screen can get, not how long it can freeze.
local USAGE_TIMEOUT = 5
local MEDIA_TIMEOUT = 2
local COMMAND_TIMEOUT = 3
local ART_TIMEOUT = 5
local BACKGROUND_TIMEOUT = 15
local SETTINGS_TIMEOUT = 3

-- Used when cd_config.lua does not say otherwise.
local DISPLAY_FALLBACK = {
    panel_lighten = 0.7,
    full_refresh_minutes = 0,
    media_interval_seconds = 5,
    frontlight = -1,
    warmth = -1,
}

local OFFLINE_RETRY_SECONDS = 30
-- Windows reports the new track a moment after it accepts a command.
local AFTER_COMMAND_SECONDS = 1
-- One missed poll is not worth a screen flash; two in a row is a real outage.
local DISCONNECTED_AFTER_FAILURES = 2
-- How long a tapped play state is trusted over what the PC still reports.
local PENDING_SECONDS = 4
local MAX_PENDING_COMMANDS = 2
-- A scheduled step that runs this late means the UI thread was stuck.
local STALL_SECONDS = 3

local COMMANDS = {
    previous = "previous",
    play_pause = "play-pause",
    next = "next",
    volume_down = "volume-down",
    volume_up = "volume-up",
}
local NEEDS_SESSION = { previous = true, play_pause = true, next = true }

local function set_screensaver_blocked(blocked)
    if not Device:isKindle() then return end
    -- Kindle's own power daemon ignores KOReader's standby hooks.
    os.execute("lipc-set-prop com.lab126.powerd preventScreenSaver " .. (blocked and "1" or "0"))
end

--[[
Three separate things put a Kindle to sleep, and an always-on dashboard has to
hold off all of them:
  * the Kindle's own screensaver timer (preventScreenSaver above);
  * KOReader's AutoSuspend plugin, which suspends the device 15 minutes after the
    last touch. A dashboard is watched, not touched, so it would be suspended
    with its last frame still on screen and touch input switched off, which
    looks exactly like a freeze. pause_auto_suspend is the flag that plugin honours;
  * AutoSuspend's standby between input events (preventStandby).
keepalive tells AutoSuspend not to reset the Kindle's t1 timer, which its own
comments say misbehaves once preventScreenSaver is set.
]]
local function hold_awake()
    local saved = {
        pause_auto_suspend = PluginShare.pause_auto_suspend,
        keepalive = PluginShare.keepalive,
    }
    PluginShare.pause_auto_suspend = true
    PluginShare.keepalive = true
    UIManager:preventStandby()
    set_screensaver_blocked(true)
    return saved
end

local function release_awake(saved)
    set_screensaver_blocked(false)
    UIManager:allowStandby()
    PluginShare.pause_auto_suspend = saved.pause_auto_suspend
    PluginShare.keepalive = saved.keepalive
end

local function read_file(path)
    local file = io.open(path, "rb")
    if not file then return nil end
    local data = file:read("*a")
    file:close()
    return data
end

local function write_file(path, data)
    local file = io.open(path, "wb")
    if not file then return false end
    file:write(data)
    file:close()
    return true
end

local Dashboard = InputContainer:extend{
    covers_fullscreen = true,
    closed = false,

    snapshot = nil,
    fetch_failed = false,
    -- Reason reported by a reachable PC that has no numbers, such as an expired token.
    server_error = nil,

    media = nil,
    media_failures = 0,
    -- Bumped on every tap, so a poll that was already under way cannot overwrite
    -- what the tap just changed with the state from before it.
    media_epoch = 0,
    -- Play state requested by a recent tap, see format.settle.
    pending = nil,

    cover = nil,
    cover_hash = nil,
    -- Hash of the cover being downloaded, so the same picture is not requested twice.
    cover_loading = nil,
    background = nil,
    background_hash = nil,
    background_loading = nil,
    -- Pictures that failed to load, see format.note_failure.
    cover_failed = nil,
    background_failed = nil,

    -- How to draw, as chosen on the PC; see format.display_settings.
    display = nil,
    settings_hash = nil,
    settings_loading = nil,
    -- Light and warmth levels last sent to the hardware.
    applied_frontlight = nil,
    applied_warmth = nil,
    -- Last refresh token seen from the PC, see format.refresh_requested.
    refresh_seen = nil,
    -- When the media poll was meant to run, to notice a stuck UI thread.
    media_due = nil,
}

function Dashboard:init()
    local width, height = Screen:getWidth(), Screen:getHeight()
    self.dimen = Geom:new{ x = 0, y = 0, w = width, h = height }
    self.zones = layout.zones(width, height)
    if Device:isTouchDevice() then
        self.ges_events = {
            Tap = { GestureRange:new{ ges = "tap", range = self.dimen } },
        }
    end
    self.tick = function() self:onClockTick() end
    self.poll = function() self:onPoll() end
    self.media_poll = function() self:onMediaPoll() end
    -- Until the PC answers, the values in cd_config.lua stand in.
    self.display = format.display_settings(config, DISPLAY_FALLBACK)
    self:loadSavedBackground()
    self:render()
end

function Dashboard:render()
    if self[1] then
        self[1]:free()
    end
    local powerd = Device:getPowerDevice()
    local capacity = powerd:getCapacity()
    local previous = self.regions
    self[1], self.regions = view.build{
        now = os.time(),
        date_table = os.date("*t"),
        battery = capacity,
        battery_symbol = powerd:getBatterySymbol(powerd:isCharged(), powerd:isCharging(), capacity),
        snapshot = self.snapshot,
        fetch_failed = self.fetch_failed,
        server_error = self.server_error,
        media = self.media,
        media_disconnected = self.media_failures >= DISCONNECTED_AFTER_FAILURES,
        cover = self.cover,
        background = self.background,
        frost = self.display.panel_lighten,
        zones = self.zones,
        icon_dir = PLUGIN_DIR .. "/icons",
    }
    -- When a block grows or shrinks, everything below it moves.
    self.layout_shifted = previous ~= nil and (
        previous.claude.y ~= self.regions.claude.y
        or previous.claude.h ~= self.regions.claude.h
        or previous.media.y ~= self.regions.media.y
    )
end

-- Repaints one band of the screen (clock, claude or media). A whole-screen
-- repaint makes the entire e-ink panel flash, so it is kept for layout shifts.
function Dashboard:redraw(name, mode)
    self:render()
    local region = self.layout_shifted and self.regions.all or self.regions[name]
    -- A fresh Geom each time: UIManager merges and rewrites the regions it is given.
    UIManager:setDirty(self, mode or "ui", Geom:new{ x = region.x, y = region.y, w = region.w, h = region.h })
end

function Dashboard:start()
    self.awake = hold_awake()
    self:scheduleTick()
    UIManager:scheduleIn(1, self.poll)
    self:scheduleMediaPoll(2)
end

function Dashboard:scheduleTick()
    UIManager:scheduleIn(60 - os.time() % 60, self.tick)
end

function Dashboard:onClockTick()
    if self.closed then return end
    if self:isFullRefreshDue() then
        self:redraw("all", "full")
    else
        self:redraw("clock")
    end
    self:scheduleTick()
end

-- An optional periodic flashing refresh clears e-ink ghosting; off by default.
function Dashboard:isFullRefreshDue()
    local every = self.display.full_refresh_minutes
    if every <= 0 then return false end

    local t = os.date("*t")
    return (t.hour * 60 + t.min) % every == 0
end

function Dashboard:onPoll()
    if self.closed then return end
    UIManager:unschedule(self.poll)

    -- isOnline() needs working DNS, which a LAN without internet does not have.
    if not NetworkMgr:isConnected() then
        -- turnOnWifi() would scan and show dialogs over the dashboard; this only flips the radio.
        if NetworkMgr.restoreWifiAsync then
            NetworkMgr:restoreWifiAsync()
        end
        UIManager:scheduleIn(OFFLINE_RETRY_SECONDS, self.poll)
        self:applyFetch(nil, "offline")
        return
    end
    UIManager:scheduleIn(config.interval_seconds, self.poll)
    net.json(config, "GET", "/usage.json", USAGE_TIMEOUT, function(snapshot, err)
        if self.closed then return end
        self:applyFetch(snapshot, err)
    end)
end

function Dashboard:applyFetch(snapshot, err)
    self.server_error = nil
    -- A reachable PC with no numbers yet (expired token) must not wipe what is on screen.
    if snapshot and not format.has_usage(snapshot) then
        self.server_error = format.text_or_nil(snapshot.error)
        snapshot, err = nil, self.server_error or "no usage data"
    end
    if snapshot then
        self.snapshot = snapshot
        self.fetch_failed = false
    else
        logger.warn("claudedash: fetch failed:", err)
        self.fetch_failed = true
    end
    self:redraw("claude")
end

function Dashboard:scheduleMediaPoll(delay)
    delay = delay or format.next_media_delay(self.media_failures, self.display)
    UIManager:unschedule(self.media_poll)
    UIManager:scheduleIn(delay, self.media_poll)
    self.media_due = os.time() + delay
end

-- The media poll is the dashboard's heartbeat. If it runs much later than it
-- was scheduled, something held the UI thread; the note is forced to disk
-- because a frozen Kindle is usually hard-reset, which loses the log's tail.
function Dashboard:noteStall()
    local due = self.media_due
    self.media_due = nil
    if not due then return end

    local late = os.time() - due
    if late < STALL_SECONDS then return end
    logger.warn("claudedash: UI thread was blocked for about", late, "seconds before", os.date("%H:%M:%S"))
    pcall(ffiutil.fsyncOpenedFile, io.stdout)
end

function Dashboard:onMediaPoll()
    if self.closed then return end
    UIManager:unschedule(self.media_poll)
    self:noteStall()

    if not NetworkMgr:isConnected() then
        self:applyMedia(nil, "offline")
        self:scheduleMediaPoll()
        return
    end
    local epoch = self.media_epoch
    net.json(config, "GET", "/media.json", MEDIA_TIMEOUT, function(state, err)
        if self.closed then return end
        -- Scheduled before applying: if applying ever raises, polling must still go on.
        self:scheduleMediaPoll(format.next_media_delay(state and 0 or self.media_failures + 1, self.display))
        -- A tap happened while this poll was under way: its answer is already out of date.
        if epoch == self.media_epoch then
            self:applyMedia(state, err)
        end
    end)
end

function Dashboard:applyMedia(state, err)
    local was_disconnected = self.media_failures >= DISCONNECTED_AFTER_FAILURES
    if not state then
        self.media_failures = self.media_failures + 1
        if self.media_failures == 1 then
            logger.warn("claudedash: media fetch failed:", err)
        end
        if not was_disconnected and self.media_failures >= DISCONNECTED_AFTER_FAILURES then
            self:redraw("media")
        end
        return
    end

    self.media_failures = 0
    local now = os.time()
    if format.is_settled(state, self.pending, now) then
        self.pending = nil
    else
        state = format.settle(state, self.pending, now)
    end
    local changed = format.media_changed(self.media, state)
    self.media = state
    self:syncSettings(state)
    local cover_changed = self:syncCover(state)
    local refresh
    refresh, self.refresh_seen = format.refresh_requested(self.refresh_seen, state.refresh)
    if refresh then
        self:syncBackground(state)
        self:redraw("all", "full")
        return
    end
    if self:syncBackground(state) then
        self:redraw("all")
        return
    end
    if changed or cover_changed or was_disconnected then
        self:redraw("media")
    end
end

-- Fetches the display settings when the PC reports that they changed. An
-- older PC app reports nothing, and the values in cd_config.lua stay in use.
function Dashboard:syncSettings(state)
    local hash = format.text_or_nil(state.settings)
    if not hash or hash == self.settings_hash or hash == self.settings_loading then return end

    self.settings_loading = hash
    net.json(config, "GET", "/kindle.json", SETTINGS_TIMEOUT, function(reply)
        if self.closed or self.settings_loading ~= hash then return end
        self.settings_loading = nil
        -- settings_hash stays as it was on failure, so the next poll asks again.
        if not reply then return end

        self.settings_hash = hash
        self.display = format.display_settings(reply, self.display)
        self:applyLight()
        self:redraw("all")
    end)
end

-- Sets the front light and its warmth to what the PC chose. A negative value
-- means the PC has no opinion and the Kindle keeps its own setting.
function Dashboard:applyLight()
    local powerd = Device:getPowerDevice()
    local light = self.display.frontlight
    if light >= 0 and light ~= self.applied_frontlight and Device:hasFrontlight() then
        self.applied_frontlight = light
        local level = format.light_level(light, powerd.fl_min, powerd.fl_max)
        local ok, err = pcall(powerd.setIntensity, powerd, level)
        if not ok then
            logger.warn("claudedash: could not set the front light:", err)
        end
    end

    local warmth = self.display.warmth
    if warmth >= 0 and warmth ~= self.applied_warmth and Device:hasNaturalLight() then
        self.applied_warmth = warmth
        local ok, err = pcall(powerd.setWarmth, powerd, format.warmth_percent(warmth))
        if not ok then
            logger.warn("claudedash: could not set the warmth:", err)
        end
    end
end

-- Starts loading the track's cover when it changed. Returns true when the
-- cover on screen has to change right now (the old one is taken down at once,
-- so a previous track's cover never sits next to the new title).
function Dashboard:syncCover(state)
    local hash = format.is_active(state) and format.text_or_nil(state.art) or nil
    if hash == self.cover_hash and self.cover_loading == nil then return false end
    if hash ~= nil and hash == self.cover_loading then return false end

    local had_cover = self.cover ~= nil
    self:freeCover()
    self.cover_loading = nil
    if not hash or format.retry_blocked(self.cover_failed, hash, os.time()) then return had_cover end
    self.cover_loading = hash

    net.raw(config, "/media/art.png", ART_TIMEOUT, function(data)
        if self.closed or self.cover_loading ~= hash then return end
        self.cover_loading = nil

        local bitmap = data and RenderImage:renderImageData(data, #data)
        if not bitmap then
            self.cover_failed = format.note_failure(self.cover_failed, hash, os.time())
            return
        end
        self.cover_failed = nil
        self.cover, self.cover_hash = bitmap, hash
        self:redraw("media")
    end)
    return had_cover
end

function Dashboard:freeCover()
    if self.cover then
        self.cover:free()
    end
    self.cover, self.cover_hash = nil, nil
end

function Dashboard:loadSavedBackground()
    local hash = format.text_or_nil(read_file(BACKGROUND_HASH_FILE))
    local data = hash and read_file(BACKGROUND_FILE)
    local bitmap = data and RenderImage:renderImageData(data, #data)
    if not bitmap then return end

    self.background, self.background_hash = bitmap, hash
end

-- Returns true when the picture behind the dashboard was removed. A new
-- picture arrives later, through the download callback.
function Dashboard:syncBackground(state)
    local hash = format.text_or_nil(state.background)
    if hash == self.background_hash then
        -- The PC went back to the picture already shown: forget any other download under way.
        self.background_loading = nil
        return false
    end
    if hash ~= nil and hash == self.background_loading then return false end
    if hash ~= nil and format.retry_blocked(self.background_failed, hash, os.time()) then return false end

    if not hash then
        self.background_loading = nil
        self:freeBackground()
        os.remove(BACKGROUND_FILE)
        os.remove(BACKGROUND_HASH_FILE)
        return true
    end

    self.background_loading = hash
    local path = string.format("/background.png?w=%d&h=%d", Screen:getWidth(), Screen:getHeight())
    net.raw(config, path, BACKGROUND_TIMEOUT, function(data)
        if self.closed or self.background_loading ~= hash then return end
        self.background_loading = nil

        local bitmap = data and RenderImage:renderImageData(data, #data)
        -- On failure the old picture stays.
        if not bitmap then
            self.background_failed = format.note_failure(self.background_failed, hash, os.time())
            return
        end
        self.background_failed = nil
        self:freeBackground()
        self.background, self.background_hash = bitmap, hash
        if write_file(BACKGROUND_FILE, data) then
            write_file(BACKGROUND_HASH_FILE, hash)
        end
        self:redraw("all")
    end)
    return false
end

function Dashboard:freeBackground()
    if self.background then
        self.background:free()
    end
    self.background, self.background_hash = nil, nil
end

function Dashboard:onTap(_arg, ges)
    local name = ges and ges.pos and layout.hit(self.zones, ges.pos.x, ges.pos.y)
    if name == "exit" then
        self:close()
        return true
    end
    if name == "refresh" then
        -- A flashing repaint of the whole screen clears the ghosting that partial updates leave behind.
        self:redraw("all", "full")
        return true
    end

    local command = name and COMMANDS[name]
    if not command then return true end
    if NEEDS_SESSION[name] and not format.is_active(self.media) then return true end

    -- With the PC unreachable every tap waits out a timeout; a pile of them
    -- would keep the dashboard busy for minutes and then all fire at once.
    if net.pending_commands() >= MAX_PENDING_COMMANDS then return true end

    self.media_epoch = self.media_epoch + 1
    self:showTap(command)
    net.json(config, "POST", "/media/" .. command, COMMAND_TIMEOUT, function(state, err)
        if self.closed then return end
        self:scheduleMediaPoll(AFTER_COMMAND_SECONDS)
        if state then
            self:applyMedia(state)
        else
            logger.warn("claudedash: command failed:", command, err)
        end
    end, true)
    return true
end

-- Draws the expected result at once; the PC's answer arrives later and corrects it if needed.
function Dashboard:showTap(command)
    local guess = format.optimistic(self.media, command)
    if not guess then return end

    if command == COMMANDS.play_pause then
        self.pending = { playing = guess.playing == true, expires = os.time() + PENDING_SECONDS }
    end
    self.media = guess
    self:redraw("media")
end

-- The power button still puts the Kindle to sleep on purpose. Coming back,
-- everything on screen is out of date, so repaint it all and poll at once.
function Dashboard:onResume()
    if self.closed then return end
    logger.info("claudedash: resumed from sleep at", os.date("%H:%M:%S"))
    set_screensaver_blocked(true)
    UIManager:unschedule(self.tick)
    self:redraw("all", "full")
    self:scheduleTick()
    self:scheduleMediaPoll(1)
    UIManager:unschedule(self.poll)
    UIManager:scheduleIn(2, self.poll)
end

function Dashboard:onSuspend()
    if self.closed then return end
    logger.warn("claudedash: the device is going to sleep at", os.date("%H:%M:%S"))
    -- KOReader only wakes up when the Kindle reports leaving its screensaver. With the
    -- screensaver blocked that report never comes, and the device stays deaf to touch
    -- and to the power button until it is hard-reset. So once a sleep is under way,
    -- let the Kindle run its normal cycle; the block goes back on in onResume.
    set_screensaver_blocked(false)
    pcall(ffiutil.fsyncOpenedFile, io.stdout)
end

function Dashboard:close()
    UIManager:close(self, "full")
end

-- Runs for every way the widget can go away, so the Kindle is never left unable to sleep.
function Dashboard:onCloseWidget()
    if self.closed then return end
    self.closed = true
    UIManager:unschedule(self.tick)
    UIManager:unschedule(self.poll)
    UIManager:unschedule(self.media_poll)
    net.cancel()
    if self.awake then
        release_awake(self.awake)
        self.awake = nil
    end
    if self[1] then
        self[1]:free()
    end
    self:freeCover()
    self:freeBackground()
end

local ClaudeDash = WidgetContainer:extend{
    name = "claudedash",
    is_doc_only = false,
}

function ClaudeDash:init()
    self.ui.menu:registerToMainMenu(self)
end

function ClaudeDash:addToMainMenu(menu_items)
    menu_items.claudedash = {
        text = _("Claude Dashboard"),
        sorting_hint = "tools",
        callback = function()
            local dashboard = Dashboard:new{}
            UIManager:show(dashboard, "full")
            dashboard:start()
        end,
    }
end

return ClaudeDash
