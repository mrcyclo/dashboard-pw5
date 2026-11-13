--[[
Draws the dashboard: the background picture, one see-through panel per
widget with its component tree on top, and the corner with the battery,
refresh and exit controls. Trees are laid out by cd_tree in reference pixels
(1236 px wide screen) and scaled to the real screen here.
]]
local Blitbuffer = require("ffi/blitbuffer")
local CenterContainer = require("ui/widget/container/centercontainer")
local Device = require("device")
local Font = require("ui/font")
local FrameContainer = require("ui/widget/container/framecontainer")
local Geom = require("ui/geometry")
local ImageWidget = require("ui/widget/imagewidget")
local LeftContainer = require("ui/widget/container/leftcontainer")
local LineWidget = require("ui/widget/linewidget")
local OverlapGroup = require("ui/widget/overlapgroup")
local ProgressWidget = require("ui/widget/progresswidget")
local TextWidget = require("ui/widget/textwidget")
local Widget = require("ui/widget/widget")
local tree = require("cd_tree")

local Screen = Device.screen

local view = {}

local REFERENCE_WIDTH = 1236
local PADDING = 24
local CORNER_SIDE = 150
local BATTERY_W = 220
local OFFLINE_W = 240
local CORNER_TEXT = 35
local DEFAULT_FROST = 0.7

-- A see-through pane: it lightens whatever is already painted underneath, so
-- text on top stays readable while the background picture still shows through.
local Frost = Widget:extend{
    dimen = nil,
    amount = DEFAULT_FROST,
}

function Frost:getSize()
    return self.dimen
end

function Frost:paintTo(bb, x, y)
    bb:lightenRect(x, y, self.dimen.w, self.dimen.h, self.amount)
end

local function at(widget, x, y)
    widget.overlap_offset = { math.floor(x), math.floor(y) }
    return widget
end

-- Tree sizes are font pixel heights at the reference width; KOReader faces take
-- a size it scales by screen density, so divide that scaling back out.
local function face(size, scale)
    local density = Screen:scaleBySize(100) / 100
    return Font:getFace("cfont", math.max(math.floor(size * scale / density + 0.5), 1))
end

local function text_widget(text, size, bold, dim, scale, max_width)
    return TextWidget:new{
        text = text,
        face = face(size, scale),
        bold = bold,
        fgcolor = dim and Blitbuffer.COLOR_DARK_GRAY or Blitbuffer.COLOR_BLACK,
        max_width = max_width,
    }
end

local function scaled_rect(rect, scale)
    return {
        x = math.floor(rect.x * scale),
        y = math.floor(rect.y * scale),
        w = math.floor(rect.w * scale),
        h = math.floor(rect.h * scale),
    }
end

local function icon_widget(file, side, dim)
    if not file or side < 1 then return nil end
    return ImageWidget:new{
        file = file,
        width = side,
        height = side,
        alpha = true,
        dim = dim,
        -- Night mode already inverts the whole screen; inverting icons again would undo it.
        original_in_nightmode = false,
    }
end

-- blank holds the place of an icon that has not downloaded yet. KOReader
-- containers ask their content for its size, so it must have one.
local function blank(side)
    side = math.max(side, 1)
    return Widget:new{ dimen = Geom:new{ w = side, h = side } }
end

local function centered_in(widget, w, h)
    return CenterContainer:new{ dimen = Geom:new{ w = w, h = h }, widget }
end

local function placeholder(w, h)
    return FrameContainer:new{
        bordersize = Screen:scaleBySize(1),
        padding = 0,
        margin = 0,
        background = Blitbuffer.COLOR_LIGHT_GRAY,
        Widget:new{ dimen = Geom:new{ w = math.max(w - 2 * Screen:scaleBySize(1), 1), h = math.max(h - 2 * Screen:scaleBySize(1), 1) } },
    }
end

local function button_widget(node, w, h, icon_file)
    local side = math.min(w, h)
    local disabled = node.disabled == true
    if node.plain == true then
        local icon_side = math.floor(side * 0.63)
        return centered_in(icon_widget(icon_file(node.icon), icon_side, disabled) or blank(icon_side), w, h)
    end
    local border = Screen:scaleBySize(2)
    local inner = math.max(side - 2 * border, 1)
    local icon_side = math.floor(side * 0.42)
    return centered_in(FrameContainer:new{
        bordersize = border,
        color = disabled and Blitbuffer.COLOR_GRAY or Blitbuffer.COLOR_BLACK,
        radius = math.floor(side / 2),
        -- Opaque, so the button stays readable on top of a background picture.
        background = Blitbuffer.COLOR_WHITE,
        padding = 0,
        margin = 0,
        centered_in(icon_widget(icon_file(node.icon), icon_side, disabled) or blank(icon_side), inner, inner),
    }, w, h)
end

-- draw_box turns one laid-out box into a KOReader widget of its size, or nil.
local function draw_box(box, w, h, ctx)
    local node = box.node
    local kind = node.kind
    if kind == "text" or kind == "clock" or kind == "countdown" then
        if box.text == "" then return nil end
        local size = type(node.size) == "number" and node.size or 20
        local widget = text_widget(box.text, size, node.bold == true, node.dim == true, ctx.scale, w)
        return LeftContainer:new{ dimen = Geom:new{ w = w, h = h }, widget }
    end
    if kind == "bar" then
        local percent = type(node.percent) == "number" and node.percent or 0
        return ProgressWidget:new{
            width = w,
            height = h,
            percentage = math.min(math.max(percent, 0), 100) / 100,
            -- KOReader's default gray fill is hard to tell from the empty part on e-ink.
            fillcolor = Blitbuffer.COLOR_BLACK,
            bgcolor = Blitbuffer.COLOR_WHITE,
            bordercolor = Blitbuffer.COLOR_BLACK,
            bordersize = Screen:scaleBySize(1),
            radius = 0,
            margin_h = 0,
            margin_v = 0,
        }
    end
    if kind == "rule" then
        return LineWidget:new{ background = Blitbuffer.COLOR_DARK_GRAY, dimen = Geom:new{ w = w, h = math.max(h, 1) } }
    end
    if kind == "icon" then
        local side = math.min(w, h)
        return centered_in(icon_widget(ctx.icon_file(node.name), side, node.dim == true) or blank(side), w, h)
    end
    if kind == "image" then
        local bitmap = type(node.src) == "string" and ctx.image_for("image:" .. node.src)
        if not bitmap then return placeholder(w, h) end
        return ImageWidget:new{
            image = bitmap,
            -- The dashboard owns the bitmap and reuses it across redraws.
            image_disposable = false,
            width = w,
            height = h,
            scale_factor = 0,
        }
    end
    if kind == "button" then
        return button_widget(node, w, h, ctx.icon_file)
    end
    return nil
end

-- measure gives a text's width in reference pixels, as cd_tree expects.
local function measurer(scale)
    local cache = {}
    return function(text, size, bold)
        local key = string.format("%s|%d|%s", text, size, tostring(bold))
        if cache[key] then return cache[key] end
        local widget = text_widget(text, size, bold, false, scale)
        local width = math.ceil(widget:getSize().w / scale)
        widget:free()
        cache[key] = width
        return width
    end
end

local function corner(layers, state, screen_w, scale)
    local side = math.floor(CORNER_SIDE * scale)
    local exit = { x = screen_w - side, y = 0, w = side, h = side }
    local refresh = { x = exit.x - side, y = 0, w = side, h = side }
    local icon_side = math.floor(side * 0.3)
    local close_icon = icon_widget(state.icon_dir .. "/close.svg", icon_side)
    local refresh_icon = icon_widget(state.icon_dir .. "/refresh.svg", icon_side)
    table.insert(layers, at(centered_in(close_icon, side, side), exit.x, exit.y))
    table.insert(layers, at(centered_in(refresh_icon, side, side), refresh.x, refresh.y))

    local right = refresh.x
    if state.battery then
        local battery = text_widget((state.battery_symbol or "") .. " " .. state.battery .. "%", CORNER_TEXT, false, false, scale)
        local size = battery:getSize()
        table.insert(layers, at(battery, right - size.w, (side - size.h) / 2))
    end
    if state.offline then
        local mark = text_widget("PC offline", CORNER_TEXT, true, false, scale)
        local size = mark:getSize()
        local x = right - math.floor(BATTERY_W * scale) - size.w
        table.insert(layers, at(mark, x, (side - size.h) / 2))
    end

    local left = right - math.floor((BATTERY_W + OFFLINE_W) * scale)
    return {
        region = { x = left, y = 0, w = screen_w - left, h = side },
        exit = exit,
        refresh = refresh,
    }
end

--[[
state = {
    screen,             -- from cd_screen, or nil before the PC was ever reached
    shown,              -- kind -> tree to draw (the reply's tree with tap predictions)
    background, frost,  -- picture behind everything, and how strongly panels lighten it
    battery, battery_symbol, offline,
    now, date_fn,
    image_for(key),     -- bitmap for "image:<src>", or nil while it loads
    icon_file(name),    -- path of a downloaded icon, or nil
    icon_dir,           -- the plugin's own icons, for the corner
}
Returns the widget tree, the refresh regions by name ("all", "corner" and each
widget kind) and the tappable areas { rect, action, patch, disabled, kind }.
]]
function view.build(state)
    local screen_w, screen_h = Screen:getWidth(), Screen:getHeight()
    local scale = screen_w / REFERENCE_WIDTH
    local ctx = { scale = scale, image_for = state.image_for, icon_file = state.icon_file }
    local measure = measurer(scale)

    local layers = OverlapGroup:new{
        dimen = Geom:new{ w = screen_w, h = screen_h },
        allow_mirroring = false,
    }
    if state.background then
        table.insert(layers, ImageWidget:new{
            image = state.background,
            -- The dashboard owns the bitmap and reuses it across redraws.
            image_disposable = false,
            width = screen_w,
            height = screen_h,
        })
    end

    local regions = { all = { x = 0, y = 0, w = screen_w, h = screen_h } }
    local buttons = {}
    local widgets = state.screen and state.screen.widgets or {}
    for _, w in ipairs(widgets) do
        local rect = scaled_rect(w.rect, scale)
        regions[w.kind] = rect
        if state.background then
            table.insert(layers, at(Frost:new{ dimen = Geom:new{ w = rect.w, h = rect.h }, amount = state.frost or DEFAULT_FROST }, rect.x, rect.y))
        end
        local inner = {
            x = w.rect.x + PADDING,
            y = w.rect.y + PADDING,
            w = w.rect.w - 2 * PADDING,
            h = w.rect.h - 2 * PADDING,
        }
        local root = state.shown[w.kind] or w.tree
        for _, box in ipairs(tree.layout(root, inner, measure, state.now, state.date_fn)) do
            local r = scaled_rect(box, scale)
            local drawn = draw_box(box, r.w, r.h, ctx)
            if drawn then
                table.insert(layers, at(drawn, r.x, r.y))
            end
            if box.node.kind == "button" and type(box.node.action) == "string" then
                table.insert(buttons, {
                    rect = r,
                    action = box.node.action,
                    patch = box.node.patch,
                    disabled = box.node.disabled == true,
                    kind = w.kind,
                })
            end
        end
    end

    -- Only before the PC was ever reached: a screen with every widget hidden is a choice.
    if not state.screen then
        local waiting = text_widget("Waiting for the PC", 45, true, false, scale)
        local size = waiting:getSize()
        table.insert(layers, at(waiting, (screen_w - size.w) / 2, (screen_h - size.h) / 2))
    end

    local controls = corner(layers, state, screen_w, scale)
    regions.corner = controls.region
    table.insert(buttons, 1, { rect = controls.exit, action = "exit" })
    table.insert(buttons, 2, { rect = controls.refresh, action = "refresh" })

    local root = FrameContainer:new{
        background = Blitbuffer.COLOR_WHITE,
        bordersize = 0,
        padding = 0,
        margin = 0,
        layers,
    }
    return root, regions, buttons
end

function view.hit(buttons, x, y)
    if type(x) ~= "number" or type(y) ~= "number" then return nil end
    for _, b in ipairs(buttons) do
        local r = b.rect
        if x >= r.x and x < r.x + r.w and y >= r.y and y < r.y + r.h then
            return b
        end
    end
    return nil
end

return view
