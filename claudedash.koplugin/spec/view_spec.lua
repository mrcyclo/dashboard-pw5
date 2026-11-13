-- Runs cd_view against small stand-ins for the KOReader widgets it uses. Like
-- KOReader, a container asks its content for its size when it paints, so a
-- widget without a size fails here instead of crashing the Kindle.

local Widget = {}
Widget.__index = Widget

function Widget:extend(o)
    o = o or {}
    setmetatable(o, self)
    o.__index = o
    return o
end

function Widget:new(o)
    o = o or {}
    setmetatable(o, self)
    return o
end

function Widget:getSize()
    return self.dimen
end

function Widget:free() end

local function size_of(content)
    local size = content:getSize()
    assert(type(size) == "table" and type(size.w) == "number" and type(size.h) == "number", "a widget has no size")
    return size
end

local function stub(name, value)
    package.preload[name] = function() return value end
end

local Container = Widget:extend{}
function Container:paint()
    size_of(self[1])
end

local Frame = Widget:extend{}
function Frame:getSize()
    local inner = size_of(self[1])
    local border = 2 * (self.bordersize or 0)
    return { w = inner.w + border, h = inner.h + border }
end

local Text = Widget:extend{}
function Text:getSize()
    return { w = math.floor(#self.text * self.face.size / 2), h = self.face.size }
end

local Image = Widget:extend{}
function Image:getSize()
    return { w = self.width, h = self.height }
end

local Progress = Widget:extend{}
function Progress:getSize()
    return { w = self.width, h = self.height }
end

stub("ffi/blitbuffer", { COLOR_BLACK = 0, COLOR_DARK_GRAY = 1, COLOR_GRAY = 2, COLOR_LIGHT_GRAY = 3, COLOR_WHITE = 4 })
stub("device", {
    screen = {
        getWidth = function() return 1236 end,
        getHeight = function() return 1648 end,
        scaleBySize = function(_, value) return value * 2 end,
    },
})
stub("ui/font", { getFace = function(_, _, size) return { size = size } end })
stub("ui/geometry", { new = function(_, o) return o end })
stub("ui/widget/widget", Widget)
stub("ui/widget/container/centercontainer", Container)
stub("ui/widget/container/leftcontainer", Container)
stub("ui/widget/container/framecontainer", Frame)
stub("ui/widget/imagewidget", Image)
stub("ui/widget/linewidget", Widget:extend{})
stub("ui/widget/overlapgroup", Widget:extend{})
stub("ui/widget/progresswidget", Progress)
stub("ui/widget/textwidget", Text)

local view = require("cd_view")

-- paint walks the tree the way UIManager would and fails on a widget with no size.
local function paint(widget)
    if widget.paint then
        widget:paint()
    end
    for _, child in ipairs(widget) do
        paint(child)
    end
end

local media_tree = { kind = "col", gap = 8, children = {
    { kind = "text", text = "Playing · Chrome", size = 29, bold = true },
    { kind = "row", gap = 16, children = {
        { kind = "image", src = "/media/art.png?v=1", w = 120, h = 120 },
        { kind = "icon", name = "note", size = 120 },
        { kind = "col", grow = 1, children = { { kind = "text", text = "Song", size = 37 } } },
        { kind = "button", icon = "previous", size = 120, action = "media.previous" },
        { kind = "button", id = "pp", icon = "pause", size = 120, action = "media.play-pause" },
        { kind = "button", icon = "next", size = 120, action = "media.next", disabled = true },
    } },
    { kind = "row", gap = 16, children = {
        { kind = "button", icon = "volume-low", size = 100, action = "media.volume-down", plain = true },
        { kind = "bar", percent = 20, height = 16, grow = 1 },
        { kind = "button", icon = "volume-high", size = 100, action = "media.volume-up", plain = true },
        { kind = "text", text = "20", size = 43, bold = true },
    } },
} }

local function state(icon_file)
    return {
        screen = {
            layout = "L",
            widgets = { { kind = "media", hash = "h", rect = { x = 48, y = 1224, w = 1140, h = 376 }, tree = media_tree } },
            icons = {},
        },
        shown = {},
        battery = 76,
        battery_symbol = "",
        offline = true,
        now = 0,
        image_for = function() return nil end,
        icon_file = icon_file,
        icon_dir = "icons",
    }
end

-- Icons and pictures that have not downloaded yet: placeholders that still have a size.
local root, regions, buttons = view.build(state(function() return nil end))
paint(root)
assert(regions.media and regions.media.y == 1224, "the widget has a refresh region")
assert(regions.corner and regions.all, "corner and whole-screen regions")

-- With icons downloaded.
root, _, buttons = view.build(state(function(name) return "icons/" .. name .. ".svg" end))
paint(root)

-- Exit and refresh come first, then the widget's buttons with their actions.
assert(buttons[1].action == "exit" and buttons[2].action == "refresh", "corner buttons")
local actions = {}
for _, b in ipairs(buttons) do
    actions[b.action] = b
end
assert(actions["media.play-pause"] and actions["media.next"].disabled, "widget buttons")
assert(view.hit(buttons, actions["media.previous"].rect.x + 1, actions["media.previous"].rect.y + 1).action == "media.previous", "hit test")
assert(view.hit(buttons, 600, 900) == nil, "a tap on nothing")

-- Before the PC was ever reached.
root = view.build({ shown = {}, now = 0, image_for = function() return nil end, icon_file = function() return nil end, icon_dir = "icons" })
paint(root)
