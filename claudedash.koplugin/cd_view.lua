local Blitbuffer = require("ffi/blitbuffer")
local CenterContainer = require("ui/widget/container/centercontainer")
local Device = require("device")
local Font = require("ui/font")
local FrameContainer = require("ui/widget/container/framecontainer")
local Geom = require("ui/geometry")
local HorizontalGroup = require("ui/widget/horizontalgroup")
local HorizontalSpan = require("ui/widget/horizontalspan")
local ImageWidget = require("ui/widget/imagewidget")
local LeftContainer = require("ui/widget/container/leftcontainer")
local LineWidget = require("ui/widget/linewidget")
local OverlapGroup = require("ui/widget/overlapgroup")
local ProgressWidget = require("ui/widget/progresswidget")
local TextWidget = require("ui/widget/textwidget")
local VerticalGroup = require("ui/widget/verticalgroup")
local VerticalSpan = require("ui/widget/verticalspan")
local Widget = require("ui/widget/widget")
local format = require("cd_format")
local layout = require("cd_layout")

local Screen = Device.screen

local view = {}

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

local function scaled(size)
    return Screen:scaleBySize(size)
end

local function text(str, size, bold, max_width)
    return TextWidget:new{
        text = str,
        face = Font:getFace("cfont", size),
        bold = bold,
        max_width = max_width,
    }
end

local function span(size)
    return VerticalSpan:new{ width = scaled(size) }
end

local function left(widget, width)
    return LeftContainer:new{
        dimen = Geom:new{ w = width, h = widget:getSize().h },
        widget,
    }
end

-- Two widgets pushed to the opposite ends of a row.
local function spread(left_widget, right_widget, width)
    local gap = math.max(width - left_widget:getSize().w - right_widget:getSize().w, 0)
    return HorizontalGroup:new{ left_widget, HorizontalSpan:new{ width = gap }, right_widget }
end

local function rule(width)
    return LineWidget:new{
        background = Blitbuffer.COLOR_DARK_GRAY,
        dimen = Geom:new{ w = width, h = Screen:scaleBySize(1) },
    }
end

local function header(group, title, note, width)
    local note_widget = note and text(note, 13)
    local title_width = note_widget and (width - note_widget:getSize().w - scaled(8)) or width
    local title_widget = text(title, 15, true, title_width)
    if note_widget then
        table.insert(group, spread(title_widget, note_widget, width))
    else
        table.insert(group, left(title_widget, width))
    end
    table.insert(group, span(2))
    table.insert(group, rule(width))
    table.insert(group, span(5))
end

-- One limit: its label and "N% used" on a line, a bar below.
local function limit_cell(row, width)
    local used = text(row.used, 15, true)
    local label = text(row.label, 15, true, width - used:getSize().w - scaled(8))
    local cell = VerticalGroup:new{
        align = "left",
        spread(label, used, width),
        span(2),
        ProgressWidget:new{
            width = width,
            height = scaled(10),
            percentage = row.percent / 100,
        },
    }
    if row.refresh then
        table.insert(cell, left(text(row.refresh, 13), width))
    end
    return cell
end

-- Limits sit two to a line so the section stays short.
local function limit_rows(group, rows, width)
    local gap = scaled(14)
    local cell_width = math.floor((width - gap) / 2)
    for index = 1, #rows, 2 do
        if index > 1 then
            table.insert(group, span(6))
        end
        local first, second = rows[index], rows[index + 1]
        if second then
            table.insert(group, HorizontalGroup:new{
                align = "top",
                limit_cell(first, cell_width),
                HorizontalSpan:new{ width = gap },
                limit_cell(second, cell_width),
            })
        else
            table.insert(group, limit_cell(first, width))
        end
    end
end

local function claude_block(group, state, width)
    local sections = format.claude_sections(state.snapshot, state.now)
    if #sections == 0 then
        header(group, "Claude", nil, width)
        table.insert(group, left(text(state.server_error or "Cannot reach the PC", 15), width))
        return
    end

    for index, section in ipairs(sections) do
        if index > 1 then
            table.insert(group, span(8))
        end
        header(group, section.title, section.refresh, width)
        limit_rows(group, section.rows, width)
    end

    -- When the numbers are current the timestamp is noise; it matters once they are old.
    local is_stale = state.fetch_failed or (type(state.snapshot) == "table" and state.snapshot.stale == true)
    local updated = is_stale and format.updated_line(state.snapshot, state.fetch_failed)
    if updated then
        table.insert(group, span(4))
        table.insert(group, left(text(updated, 13), width))
    end
end

local function icon(state, name, size, dim)
    return ImageWidget:new{
        file = state.icon_dir .. "/" .. name .. ".svg",
        width = size,
        height = size,
        alpha = true,
        dim = dim,
        -- Night mode already inverts the whole screen; inverting icons again would undo it.
        original_in_nightmode = false,
    }
end

local function cover(state, side)
    if state.cover then
        return CenterContainer:new{
            dimen = Geom:new{ w = side, h = side },
            ImageWidget:new{
                image = state.cover,
                -- The dashboard owns the bitmap and reuses it across redraws.
                image_disposable = false,
                width = side,
                height = side,
                scale_factor = 0,
            },
        }
    end
    local inner = side - 2 * Screen:scaleBySize(1)
    return FrameContainer:new{
        bordersize = Screen:scaleBySize(1),
        padding = 0,
        margin = 0,
        background = Blitbuffer.COLOR_LIGHT_GRAY,
        CenterContainer:new{
            dimen = Geom:new{ w = inner, h = inner },
            icon(state, "note", math.floor(side * 0.4)),
        },
    }
end

local function at(widget, x, y)
    widget.overlap_offset = { math.floor(x), math.floor(y) }
    return widget
end

-- Places a widget so its center matches the center of an area.
local function centered(widget, zone)
    local size = widget:getSize()
    return at(widget, zone.x + (zone.w - size.w) / 2, zone.y + (zone.h - size.h) / 2)
end

local function button(state, zone, icon_name, disabled)
    local side = math.floor(math.min(zone.w, zone.h) * 0.8)
    local border = Screen:scaleBySize(2)
    local inner = side - 2 * border
    return centered(FrameContainer:new{
        bordersize = border,
        color = disabled and Blitbuffer.COLOR_GRAY or Blitbuffer.COLOR_BLACK,
        radius = math.floor(side / 2),
        -- Opaque, so the button stays readable on top of a background picture.
        background = Blitbuffer.COLOR_WHITE,
        padding = 0,
        margin = 0,
        CenterContainer:new{
            dimen = Geom:new{ w = inner, h = inner },
            icon(state, icon_name, math.floor(side * 0.42), disabled),
        },
    }, zone)
end

-- Quieter icon, level bar, louder icon, number: the two icons are the buttons.
local function volume_row(layers, state, zones)
    local icon_size = math.floor(zones.volume_down.h * 0.42)
    table.insert(layers, centered(icon(state, "volume-low", icon_size), zones.volume_down))
    table.insert(layers, centered(icon(state, "volume-high", icon_size), zones.volume_up))

    local fraction = format.volume_fraction(state.media)
    if fraction then
        table.insert(layers, centered(ProgressWidget:new{
            width = zones.volume_bar.w,
            height = scaled(9),
            percentage = fraction,
        }, zones.volume_bar))
    end

    local volume = format.volume_text(state.media)
    if volume then
        table.insert(layers, centered(text(volume, 22, true), zones.volume_value))
    end
end

local function media_panel(layers, state, zones)
    local header_zone = zones.media_header
    local title = VerticalGroup:new{ align = "left" }
    header(title, format.media_title(state.media, state.media_disconnected), nil, header_zone.w)
    table.insert(layers, at(title, header_zone.x, header_zone.y))

    table.insert(layers, at(cover(state, zones.cover.w), zones.cover.x, zones.cover.y))

    local text_zone = zones.track_text
    local lines = VerticalGroup:new{ align = "left" }
    if format.is_active(state.media) then
        local track = format.text_or_nil(state.media.title)
        local artist = format.text_or_nil(state.media.artist)
        if track then
            table.insert(lines, text(track, 19, true, text_zone.w))
        end
        if artist then
            table.insert(lines, text(artist, 16, false, text_zone.w))
        end
    else
        table.insert(lines, text("Nothing playing", 16, false, text_zone.w))
    end
    table.insert(layers, at(lines, text_zone.x, text_zone.y + (text_zone.h - lines:getSize().h) / 2))

    local idle = not format.is_active(state.media)
    local playing = not idle and state.media.playing == true
    table.insert(layers, button(state, zones.previous, "previous", idle))
    table.insert(layers, button(state, zones.play_pause, playing and "pause" or "play", idle))
    table.insert(layers, button(state, zones.next, "next", idle))
    volume_row(layers, state, zones)
end

local function frost(area, amount)
    return at(Frost:new{ dimen = Geom:new{ w = area.w, h = area.h }, amount = amount }, area.x, area.y)
end

--[[
state = {
    now, date_table,
    battery, battery_symbol,
    snapshot, fetch_failed, server_error,     -- Claude usage
    media, media_disconnected, cover,         -- music
    background,                               -- full-screen picture, or nil
    frost,                                    -- how strongly panels lighten the picture, 0 to 1
    zones, icon_dir,
}
Returns the widget tree and the refresh regions that go with it.
]]
function view.build(state)
    local screen_w, screen_h = Screen:getWidth(), Screen:getHeight()
    local zones = state.zones
    local width = zones.content.w

    local clock = VerticalGroup:new{ align = "left" }
    table.insert(clock, left(text(format.clock(state.date_table), 44, true), width))
    table.insert(clock, left(text(format.date_text(state.date_table), 17), width))

    local claude = VerticalGroup:new{ align = "left" }
    claude_block(claude, state, width)

    local gap = scaled(8)
    local content = VerticalGroup:new{
        align = "left",
        clock,
        VerticalSpan:new{ width = gap },
        claude,
    }
    local column = { clock = clock:getSize().h, gap = gap, claude = claude:getSize().h }

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
        local amount = state.frost or DEFAULT_FROST
        table.insert(layers, frost(layout.top_panel(zones, content:getSize().h), amount))
        table.insert(layers, frost(zones.media_panel, amount))
    end

    table.insert(layers, at(content, zones.content.x, zones.content.y))
    local corner_icon = math.floor(zones.exit.w * 0.3)
    table.insert(layers, centered(icon(state, "close", corner_icon), zones.exit))
    table.insert(layers, centered(icon(state, "refresh", corner_icon), zones.refresh))

    if state.battery then
        local battery = text((state.battery_symbol or "") .. " " .. state.battery .. "%", 17)
        local size = battery:getSize()
        local zone = zones.battery
        table.insert(layers, at(battery, zone.x + zone.w - size.w, zone.y + (zone.h - size.h) / 2))
    end

    media_panel(layers, state, zones)

    local tree = FrameContainer:new{
        background = Blitbuffer.COLOR_WHITE,
        bordersize = 0,
        padding = 0,
        margin = 0,
        layers,
    }
    return tree, layout.regions(screen_w, screen_h, zones, column)
end

return view
