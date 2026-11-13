local screen = require("cd_screen")
local format = require("cd_format")

local function eq(actual, expected, what)
    if actual ~= expected then
        error(string.format("%s: expected %s, got %s", what or "value", tostring(expected), tostring(actual)), 2)
    end
end

local json_null = function() end

local function widget(kind, hash, extra)
    local w = {
        kind = kind,
        hash = hash,
        rect = { x = 48, y = 48, w = 562, h = 180 },
        tree = { kind = "col", children = { { kind = "text", text = kind, size = 20 } } },
    }
    for key, value in pairs(extra or {}) do
        w[key] = value
    end
    return w
end

local function reply(layout, widgets, icons)
    return { layout = layout, widgets = widgets, icons = icons or {} }
end

local function count(t)
    local n = 0
    for _ in pairs(t) do
        n = n + 1
    end
    return n
end

-- The first reply repaints everything.
local current, changes = screen.accept(nil, reply("L1", { widget("clock", "a"), widget("claude", "b") }))
eq(changes.full, true, "first reply")
eq(#current.widgets, 2, "widgets kept")

-- Same hashes: nothing to redraw.
local again
again, changes = screen.accept(current, reply("L1", { widget("clock", "a"), widget("claude", "b") }))
eq(changes.full, false, "same layout")
eq(count(changes.widgets), 0, "nothing changed")

-- One changed hash redraws only that widget.
again, changes = screen.accept(current, reply("L1", { widget("clock", "a"), widget("claude", "c") }))
eq(changes.full, false, "same layout")
eq(changes.widgets.claude, true, "claude changed")
eq(changes.widgets.clock, nil, "clock unchanged")

-- A new layout repaints everything.
again, changes = screen.accept(current, reply("L2", { widget("clock", "a") }))
eq(changes.full, true, "layout changed")
eq(#again.widgets, 1, "hidden widget gone")

-- Broken entries are dropped and the others kept.
again = screen.accept(nil, reply("L1", {
    widget("clock", "a"),
    widget("claude", "b", { tree = json_null }),
    widget("media", "c", { rect = { x = 1, y = 2, w = "wide", h = 4 } }),
    widget(json_null, "d"),
    json_null,
    widget("other", json_null),
}))
eq(#again.widgets, 2, "only valid widgets")
eq(again.widgets[1].kind, "clock", "first valid")
eq(again.widgets[2].hash, "", "a missing hash still draws")

-- A reply that is not a screen changes nothing.
again, changes = screen.accept(current, json_null)
eq(again, current, "bad reply keeps the screen")
eq(changes.full, false, "no repaint")
again, changes = screen.accept(current, { layout = "L1", widgets = "many" })
eq(again, current, "bad widget list keeps the screen")

-- The screen survives a round trip through the disk.
local saved = screen.encode(current)
local loaded = screen.decode(saved)
eq(loaded.layout, "L1", "layout")
eq(#loaded.widgets, 2, "widgets")
eq(loaded.widgets[2].tree.children[1].text, "claude", "nested tree")
eq(loaded.widgets[1].rect.w, 562, "rect")
eq(screen.decode("this is not a saved screen"), nil, "garbage")
eq(screen.decode("return os.exit()"), nil, "code is not run")
eq(screen.decode(nil), nil, "missing file")
local quoted = screen.decode(screen.encode({ layout = 'a "quoted"\nline', widgets = {}, icons = {} }))
eq(quoted.layout, 'a "quoted"\nline', "strings are escaped")

-- images lists every icon and picture the screen needs, with a key that changes with it.
local with_art = screen.accept(nil, reply("L1", {
    widget("media", "m", { tree = { kind = "row", children = {
        { kind = "image", src = "/media/art.png?v=1", w = 120, h = 120 },
        { kind = "button", icon = "play", size = 120 },
    } } }),
}, { play = "p1", pause = "p2" }))
local wanted = {}
for _, item in ipairs(screen.images(with_art)) do
    wanted[item.key] = item.path
end
eq(wanted["icon:play:p1"], "/icons/play.svg", "icon")
eq(wanted["icon:pause:p2"], "/icons/pause.svg", "icon from a patch")
eq(wanted["image:/media/art.png?v=1"], "/media/art.png?v=1", "picture")

-- A failed download waits before it is asked for again.
local failed = {}
failed["icon:play:p1"] = format.note_failure(failed["icon:play:p1"], "icon:play:p1", 1000)
eq(format.retry_blocked(failed["icon:play:p1"], "icon:play:p1", 1030), true, "waits")
eq(format.retry_blocked(failed["icon:play:p1"], "icon:play:p1", 1060), false, "tries again")

-- A widget that drops out (its entry broke) under the same layout hash leaves
-- its old pixels behind unless the whole screen is repainted.
local before = screen.accept(nil, reply("L1", { widget("clock", "a"), widget("claude", "b") }))
local after
after, changes = screen.accept(before, reply("L1", { widget("clock", "a"), widget("claude", "b", { tree = json_null }) }))
eq(#after.widgets, 1, "the broken widget is gone")
eq(changes.full, true, "a widget leaving repaints everything")

-- Icon files from older hashes, or of icons no longer used, are cleaned up.
local cached = { "play-p1.svg", "play-old.svg", "pause-p2.svg", "gone-x.svg", "notes.txt" }
local stale = screen.stale_icon_files(cached, { play = "p1", pause = "p2" })
table.sort(stale)
eq(#stale, 2, "two stale files")
eq(stale[1], "gone-x.svg", "an icon no screen uses")
eq(stale[2], "play-old.svg", "an older version")
eq(#screen.stale_icon_files(cached, {}), 0, "nothing is removed before the PC listed its icons")
