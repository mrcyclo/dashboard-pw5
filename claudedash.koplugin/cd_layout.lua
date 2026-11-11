--[[
Where everything goes. Two panels, one hugging the top edge (clock and Claude
usage) and one hugging the bottom edge (music), leave the middle of the screen
open for the background picture.

All measurements are for the 1236 px wide Paperwhite 5 and scale with screen width.
]]
local layout = {}

local REFERENCE_WIDTH = 1236

local EDGE_MARGIN = 40
local PANEL_PADDING = 24
local EXIT_SIDE = 150
local BATTERY_W = 220

-- Bottom panel, from the bottom up.
local BUTTON_SIDE = 150
local ROW_GAP = 10
local HEADER_HEIGHT = 70
local COVER_SIDE = 130
local TEXT_GAP = 20
local VOLUME_VALUE_W = 130

local ZONE_NAMES = { "exit", "refresh", "previous", "play_pause", "next", "volume_down", "volume_up" }

local function rect(x, y, w, h)
    return { x = math.floor(x), y = math.floor(y), w = math.floor(w), h = math.floor(h) }
end

function layout.zones(width, height)
    local scale = width / REFERENCE_WIDTH
    local function px(value)
        return math.floor(value * scale)
    end

    local margin, padding = px(EDGE_MARGIN), px(PANEL_PADDING)
    local button = px(BUTTON_SIDE)
    local panel_x, panel_w = margin, width - 2 * margin
    local inner_x, inner_w = panel_x + padding, panel_w - 2 * padding
    local inner_right = inner_x + inner_w

    local volume_y = height - margin - padding - button
    local playback_y = volume_y - px(ROW_GAP) - button
    local header_y = playback_y - px(HEADER_HEIGHT)
    local media_panel_y = header_y - padding

    local next_x = inner_right - button
    local cover_side = px(COVER_SIDE)
    local text_x = inner_x + cover_side + px(TEXT_GAP)
    local previous_x = next_x - 2 * button

    local value_w = px(VOLUME_VALUE_W)
    local volume_up_x = inner_right - value_w - button
    local volume_bar_x = inner_x + button

    local exit_side = px(EXIT_SIDE)
    local exit_x = width - exit_side
    local refresh_x = exit_x - exit_side
    local battery_w = px(BATTERY_W)

    return {
        -- Top right corner, where a close button is expected.
        exit = rect(exit_x, 0, exit_side, exit_side),
        refresh = rect(refresh_x, 0, exit_side, exit_side),
        previous = rect(previous_x, playback_y, button, button),
        play_pause = rect(next_x - button, playback_y, button, button),
        next = rect(next_x, playback_y, button, button),
        volume_down = rect(inner_x, volume_y, button, button),
        volume_up = rect(volume_up_x, volume_y, button, button),

        -- Drawing areas only, not tappable.
        battery = rect(refresh_x - battery_w, 0, battery_w, exit_side),
        top_panel = rect(panel_x, margin, panel_w, padding * 2 + 1),
        content = rect(inner_x, margin + padding, inner_w, 1),
        media_panel = rect(panel_x, media_panel_y, panel_w, height - margin - media_panel_y),
        media_header = rect(inner_x, header_y, inner_w, px(HEADER_HEIGHT)),
        cover = rect(inner_x, playback_y + (button - cover_side) / 2, cover_side, cover_side),
        track_text = rect(text_x, playback_y, previous_x - px(TEXT_GAP) - text_x, button),
        volume_bar = rect(volume_bar_x, volume_y, volume_up_x - volume_bar_x, button),
        volume_value = rect(inner_right - value_w, volume_y, value_w, button),
    }
end

-- The top panel wraps its content, but never reaches into the media panel.
function layout.top_panel(zones, content_height)
    local panel = zones.top_panel
    local padding = zones.content.y - panel.y
    local limit = zones.media_panel.y - panel.y
    return {
        x = panel.x,
        y = panel.y,
        w = panel.w,
        h = math.max(math.min(math.floor(content_height) + 2 * padding, limit), 1),
    }
end

-- Full-width bands of the screen, one per kind of change, so an update repaints
-- (and on e-ink, flashes) only the part that actually changed.
-- column = { clock, gap, claude }: heights of the blocks in the top panel.
function layout.regions(width, height, zones, column)
    local function band(y, h)
        y = math.min(math.max(math.floor(y), 0), height - 1)
        h = math.max(math.min(math.floor(h), height - y), 1)
        return { x = 0, y = y, w = width, h = h }
    end

    local panel = layout.top_panel(zones, column.clock + column.gap + column.claude)
    local clock_end = math.min(zones.content.y + column.clock, panel.y + panel.h)
    local claude_top = math.min(clock_end + column.gap, panel.y + panel.h)
    return {
        -- Starts at the top edge so the battery and exit icon refresh with the clock.
        clock = band(0, clock_end),
        claude = band(claude_top, panel.y + panel.h - claude_top),
        -- Runs to the bottom edge: the buttons and volume row change with the music.
        media = band(zones.media_panel.y, height - zones.media_panel.y),
        all = band(0, height),
    }
end

function layout.hit(zones, x, y)
    if type(x) ~= "number" or type(y) ~= "number" then return nil end

    for _, name in ipairs(ZONE_NAMES) do
        local z = zones[name]
        if x >= z.x and x < z.x + z.w and y >= z.y and y < z.y + z.h then
            return name
        end
    end
    return nil
end

return layout
