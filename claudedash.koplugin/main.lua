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
local lfs = require("libs/libkoreader-lfs")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local logger = require("logger")
local _ = require("gettext")

local config = require("cd_config")
local format = require("cd_format")
local net = require("cd_net")
local patch = require("cd_patch")
local screen = require("cd_screen")
local view = require("cd_view")

local Screen = Device.screen

local PLUGIN_DIR = debug.getinfo(1, "S").source:match("^@(.*)/[^/]*$") or "."

-- Kept on the Kindle so the dashboard still has something to show with the PC off.
local BACKGROUND_FILE = DataStorage:getDataDir() .. "/claudedash-background.png"
local BACKGROUND_HASH_FILE = BACKGROUND_FILE .. ".hash"
local SCREEN_FILE = DataStorage:getDataDir() .. "/claudedash-screen.lua"
local ICON_DIR = DataStorage:getDataDir() .. "/claudedash-icons"

-- Socket timeouts, in seconds. Requests run off the UI thread (see cd_net), so
-- these bound how stale the screen can get, not how long it can freeze.
local SCREEN_TIMEOUT = 3
local COMMAND_TIMEOUT = 3
local IMAGE_TIMEOUT = 5
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

-- Windows reports the new track a moment after it accepts a command.
local AFTER_COMMAND_SECONDS = 1
-- One missed poll is not worth a screen flash; two in a row is a real outage.
local DISCONNECTED_AFTER_FAILURES = 2
local MAX_PENDING_COMMANDS = 2
-- A scheduled step that runs this late means the UI thread was stuck.
local STALL_SECONDS = 3

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

local function file_exists(path)
    return lfs.attributes(path, "mode") == "file"
end

-- uses reports whether a tree draws the given icon or picture.
local function uses(node, item)
    if type(node) ~= "table" then return false end
    if item.icon and (node.name == item.icon or node.icon == item.icon) then return true end
    if not item.icon and node.kind == "image" and node.src == item.path then return true end
    if type(node.patch) == "table" and item.icon then
        for _, op in ipairs(node.patch) do
            if type(op) == "table" and type(op.values) == "table" then
                for _, value in ipairs(op.values) do
                    if value == item.icon then return true end
                end
            end
        end
    end
    if type(node.children) ~= "table" then return false end
    for _, child in ipairs(node.children) do
        if uses(child, item) then return true end
    end
    return false
end

-- ticks reports whether a tree has something that changes with the time.
local function ticks(node)
    if type(node) ~= "table" then return false end
    if node.kind == "clock" or node.kind == "countdown" or type(node.at) == "table" then return true end
    if type(node.children) ~= "table" then return false end
    for _, child in ipairs(node.children) do
        if ticks(child) then return true end
    end
    return false
end

local Dashboard = InputContainer:extend{
    covers_fullscreen = true,
    closed = false,

    -- The last screen from the PC (see cd_screen), and per widget kind the tree
    -- shown, which carries tap predictions, and the predictions still pending.
    current = nil,
    shown = nil,
    pending = nil,
    failures = 0,
    offline = false,
    -- Bumped on every tap, so a poll that was already under way cannot overwrite
    -- what the tap just predicted with the state from before it.
    epoch = 0,

    -- Downloaded pictures by key (see screen.images), icon files by name.
    images = nil,
    icon_files = nil,
    loading = nil,
    failed = nil,

    background = nil,
    background_hash = nil,
    background_loading = nil,
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
    -- When the poll was meant to run, to notice a stuck UI thread.
    poll_due = nil,
}

function Dashboard:init()
    local width, height = Screen:getWidth(), Screen:getHeight()
    self.dimen = Geom:new{ x = 0, y = 0, w = width, h = height }
    if Device:isTouchDevice() then
        self.ges_events = {
            Tap = { GestureRange:new{ ges = "tap", range = self.dimen } },
        }
    end
    self.shown, self.pending = {}, {}
    self.images, self.icon_files, self.loading, self.failed = {}, {}, {}, {}
    self.tick = function() self:onClockTick() end
    self.poll = function() self:onPoll() end
    -- Until the PC answers, the values in cd_config.lua stand in.
    self.display = format.display_settings(config, DISPLAY_FALLBACK)
    lfs.mkdir(ICON_DIR)
    self:loadSavedBackground()
    self:loadSavedScreen()
    self:render()
end

function Dashboard:render()
    if self[1] then
        self[1]:free()
    end
    local powerd = Device:getPowerDevice()
    local capacity = powerd:getCapacity()
    self[1], self.regions, self.buttons = view.build{
        screen = self.current,
        shown = self.shown,
        background = self.background,
        frost = self.display.panel_lighten,
        battery = capacity,
        battery_symbol = powerd:getBatterySymbol(powerd:isCharged(), powerd:isCharging(), capacity),
        offline = self.offline,
        now = os.time(),
        image_for = function(key) return self.images[key] end,
        icon_file = function(name) return self.icon_files[name] end,
        icon_dir = PLUGIN_DIR .. "/icons",
    }
end

-- Repaints the named regions ("all", "corner" or widget kinds). A whole-screen
-- repaint makes the entire e-ink panel flash, so it is kept for layout changes.
function Dashboard:redraw(names, mode)
    self:render()
    if type(names) == "string" then
        names = { names }
    end
    for _, name in ipairs(names) do
        local region = self.regions[name]
        if region then
            -- A fresh Geom each time: UIManager merges and rewrites the regions it is given.
            UIManager:setDirty(self, mode or "ui", Geom:new{ x = region.x, y = region.y, w = region.w, h = region.h })
        end
    end
end

function Dashboard:start()
    self.awake = hold_awake()
    self:scheduleTick()
    self:schedulePoll(1)
end

function Dashboard:scheduleTick()
    UIManager:scheduleIn(60 - os.time() % 60, self.tick)
end

-- Every minute: widgets with a clock, a countdown or a timed change, and the battery.
function Dashboard:onClockTick()
    if self.closed then return end
    if self:isFullRefreshDue() then
        self:redraw("all", "full")
    else
        local names = { "corner" }
        for _, w in ipairs(self.current and self.current.widgets or {}) do
            if ticks(self.shown[w.kind] or w.tree) then
                table.insert(names, w.kind)
            end
        end
        self:redraw(names)
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

function Dashboard:schedulePoll(delay)
    delay = delay or format.next_media_delay(self.failures, self.display)
    UIManager:unschedule(self.poll)
    UIManager:scheduleIn(delay, self.poll)
    self.poll_due = os.time() + delay
end

-- The poll is the dashboard's heartbeat. If it runs much later than it was
-- scheduled, something held the UI thread; the note is forced to disk because
-- a frozen Kindle is usually hard-reset, which loses the log's tail.
function Dashboard:noteStall()
    local due = self.poll_due
    self.poll_due = nil
    if not due then return end

    local late = os.time() - due
    if late < STALL_SECONDS then return end
    logger.warn("claudedash: UI thread was blocked for about", late, "seconds before", os.date("%H:%M:%S"))
    pcall(ffiutil.fsyncOpenedFile, io.stdout)
end

function Dashboard:onPoll()
    if self.closed then return end
    UIManager:unschedule(self.poll)
    self:noteStall()

    -- isOnline() needs working DNS, which a LAN without internet does not have.
    if not NetworkMgr:isConnected() then
        -- turnOnWifi() would scan and show dialogs over the dashboard; this only flips the radio.
        if NetworkMgr.restoreWifiAsync then
            NetworkMgr:restoreWifiAsync()
        end
        self:noteFailure("offline")
        self:schedulePoll()
        return
    end

    local epoch = self.epoch
    local path = "/screen.json?battery=" .. tostring(Device:getPowerDevice():getCapacity())
    net.json(config, "GET", path, SCREEN_TIMEOUT, function(reply, err)
        if self.closed then return end
        -- Scheduled before applying: if applying ever raises, polling must still go on.
        self:schedulePoll(format.next_media_delay(reply and 0 or self.failures + 1, self.display))
        -- A tap happened while this poll was under way: its answer is already out of date.
        if epoch ~= self.epoch then return end
        if not reply then
            self:noteFailure(err)
            return
        end
        self:applyScreen(reply)
    end)
end

function Dashboard:noteFailure(err)
    self.failures = self.failures + 1
    if self.failures == 1 then
        logger.warn("claudedash: screen fetch failed:", err)
    end
    if not self.offline and self.failures >= DISCONNECTED_AFTER_FAILURES then
        self.offline = true
        self:redraw("corner")
    end
end

function Dashboard:applyScreen(reply)
    local was_offline = self.offline
    self.failures, self.offline = 0, false

    self:syncSettings(reply)
    local current, changes = screen.accept(self.current, reply)
    if current == self.current then return end
    self.current = current

    local now = os.time()
    local names = {}
    local shown = {}
    for _, w in ipairs(current.widgets) do
        local had_prediction = self.pending[w.kind] ~= nil
        shown[w.kind], self.pending[w.kind] = patch.reconcile(self.pending[w.kind], w.tree, now)
        if changes.widgets[w.kind] or had_prediction then
            table.insert(names, w.kind)
        end
    end
    -- Predictions for widgets that left the screen go with them.
    for kind in pairs(self.pending) do
        if not shown[kind] then
            self.pending[kind] = nil
        end
    end
    self.shown = shown

    if changes.full or #names > 0 then
        write_file(SCREEN_FILE, screen.encode(current))
    end
    self:syncImages()

    local refresh
    refresh, self.refresh_seen = format.refresh_requested(self.refresh_seen, reply.refresh)
    local background_removed = self:syncBackground(reply)
    if refresh then
        self:redraw("all", "full")
        return
    end
    if changes.full or background_removed then
        self:redraw("all")
        return
    end
    if was_offline then
        table.insert(names, "corner")
    end
    if #names > 0 then
        self:redraw(names)
    end
end

function Dashboard:loadSavedScreen()
    local saved = screen.decode(read_file(SCREEN_FILE))
    if not saved then return end

    self.current = saved
    for _, w in ipairs(saved.widgets) do
        self.shown[w.kind] = w.tree
    end
    -- Icons downloaded in an earlier session are still on disk.
    for _, item in ipairs(screen.images(saved)) do
        local file = item.icon and self:iconFile(item)
        if file and file_exists(file) then
            self.icon_files[item.icon] = file
        end
    end
end

function Dashboard:iconFile(item)
    return string.format("%s/%s-%s.svg", ICON_DIR, item.icon, self.current.icons[item.icon])
end

-- Starts downloading every icon and picture the screen needs and does not have.
-- Pictures the screen no longer uses are freed.
function Dashboard:syncImages()
    local wanted = {}
    for _, item in ipairs(screen.images(self.current)) do
        wanted[item.key] = true
        self:fetchImage(item)
    end
    for key, bitmap in pairs(self.images) do
        if not wanted[key] then
            bitmap:free()
            self.images[key] = nil
        end
    end
end

function Dashboard:fetchImage(item)
    local key, now = item.key, os.time()
    if self.loading[key] or self.images[key] then return end
    if item.icon then
        local file = self:iconFile(item)
        if self.icon_files[item.icon] == file then return end
        if file_exists(file) then
            self.icon_files[item.icon] = file
            self:redrawUsers(item)
            return
        end
    end
    if format.retry_blocked(self.failed[key], key, now) then return end

    self.loading[key] = true
    net.raw(config, item.path, IMAGE_TIMEOUT, function(data)
        if self.closed then return end
        self.loading[key] = nil
        if not self:storeImage(item, data) then
            self.failed[key] = format.note_failure(self.failed[key], key, os.time())
            return
        end
        self.failed[key] = nil
        self:redrawUsers(item)
    end)
end

function Dashboard:storeImage(item, data)
    if not data then return false end
    if item.icon then
        local file = self:iconFile(item)
        if not write_file(file, data) then return false end
        self.icon_files[item.icon] = file
        return true
    end
    local bitmap = RenderImage:renderImageData(data, #data)
    if not bitmap then return false end
    self.images[item.key] = bitmap
    return true
end

-- Repaints the widgets that draw a newly arrived icon or picture.
function Dashboard:redrawUsers(item)
    local names = {}
    for _, w in ipairs(self.current and self.current.widgets or {}) do
        if uses(self.shown[w.kind] or w.tree, item) then
            table.insert(names, w.kind)
        end
    end
    if #names > 0 then
        self:redraw(names)
    end
end

-- Fetches the display settings when the PC reports that they changed.
function Dashboard:syncSettings(reply)
    local hash = format.text_or_nil(reply.settings)
    if not hash or hash == self.settings_hash or hash == self.settings_loading then return end

    self.settings_loading = hash
    net.json(config, "GET", "/kindle.json", SETTINGS_TIMEOUT, function(settings)
        if self.closed or self.settings_loading ~= hash then return end
        self.settings_loading = nil
        -- settings_hash stays as it was on failure, so the next poll asks again.
        if not settings then return end

        self.settings_hash = hash
        self.display = format.display_settings(settings, self.display)
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

function Dashboard:loadSavedBackground()
    local hash = format.text_or_nil(read_file(BACKGROUND_HASH_FILE))
    local data = hash and read_file(BACKGROUND_FILE)
    local bitmap = data and RenderImage:renderImageData(data, #data)
    if not bitmap then return end

    self.background, self.background_hash = bitmap, hash
end

-- Returns true when the picture behind the dashboard was removed. A new
-- picture arrives later, through the download callback.
function Dashboard:syncBackground(reply)
    local hash = format.text_or_nil(reply.background)
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
    local target = ges and ges.pos and view.hit(self.buttons or {}, ges.pos.x, ges.pos.y)
    if not target then return true end
    if target.action == "exit" then
        self:close()
        return true
    end
    if target.action == "refresh" then
        -- A flashing repaint of the whole screen clears the ghosting that partial updates leave behind.
        self:redraw("all", "full")
        return true
    end
    if target.disabled then return true end

    -- With the PC unreachable every tap waits out a timeout; a pile of them
    -- would keep the dashboard busy for minutes and then all fire at once.
    if net.pending_commands() >= MAX_PENDING_COMMANDS then return true end

    self.epoch = self.epoch + 1
    local kind = target.kind
    local before = self.shown[kind]
    if before then
        self.shown[kind], self.pending[kind] = patch.stack(self.pending[kind], target.patch, before, os.time())
        self:redraw(kind)
    end
    net.json(config, "POST", "/action/" .. target.action, COMMAND_TIMEOUT, function(reply, err)
        if self.closed then return end
        self:schedulePoll(AFTER_COMMAND_SECONDS)
        if reply then
            self:applyScreen(reply)
        else
            logger.warn("claudedash: action failed:", target.action, err)
        end
    end, true)
    return true
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
    self:schedulePoll(1)
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
    net.cancel()
    if self.awake then
        release_awake(self.awake)
        self.awake = nil
    end
    if self[1] then
        self[1]:free()
    end
    for _, bitmap in pairs(self.images) do
        bitmap:free()
    end
    self.images = {}
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
