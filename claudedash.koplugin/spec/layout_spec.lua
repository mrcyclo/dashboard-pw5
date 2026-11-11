local layout = require("cd_layout")

local function eq(actual, expected)
    if actual ~= expected then
        error(string.format("expected %s, got %s", tostring(expected), tostring(actual)), 2)
    end
end

local TAPPABLE = { "exit", "refresh", "previous", "play_pause", "next", "volume_down", "volume_up" }
-- Areas the view draws into. They are not tappable.
local DRAWN = { "battery", "top_panel", "media_panel", "media_header", "cover", "track_text", "volume_bar", "volume_value" }

local function overlaps(a, b)
    return a.x < b.x + b.w and b.x < a.x + a.w and a.y < b.y + b.h and b.y < a.y + a.h
end

local function inside(inner, outer)
    return inner.x >= outer.x and inner.y >= outer.y
        and inner.x + inner.w <= outer.x + outer.w and inner.y + inner.h <= outer.y + outer.h
end

local function check(width, height, min_side)
    local screen = { x = 0, y = 0, w = width, h = height }
    local zones = layout.zones(width, height)

    for _, name in ipairs(TAPPABLE) do
        local zone = zones[name]
        assert(zone, "missing zone " .. name)
        assert(inside(zone, screen), name .. " leaves the screen")
        assert(zone.w >= min_side and zone.h >= min_side, name .. " is too small to tap")
        assert(zone.x == math.floor(zone.x) and zone.w == math.floor(zone.w), name .. " is not on whole pixels")
    end
    for i = 1, #TAPPABLE do
        for j = i + 1, #TAPPABLE do
            assert(not overlaps(zones[TAPPABLE[i]], zones[TAPPABLE[j]]), TAPPABLE[i] .. " overlaps " .. TAPPABLE[j])
        end
    end
    for _, name in ipairs(TAPPABLE) do
        local zone = zones[name]
        eq(layout.hit(zones, zone.x + zone.w / 2, zone.y + zone.h / 2), name)
        eq(layout.hit(zones, zone.x, zone.y), name)
        eq(layout.hit(zones, zone.x + zone.w - 1, zone.y + zone.h - 1), name)
    end

    for _, name in ipairs(DRAWN) do
        local zone = zones[name]
        assert(zone, "missing area " .. name)
        assert(inside(zone, screen), name .. " leaves the screen")
        assert(zone.w > 0 and zone.h > 0, name .. " is empty")
    end
    return zones
end

-- Kindle Paperwhite 5 in portrait.
local W, H = 1236, 1648
local zones = check(W, H, 150)

eq(layout.hit(zones, 618, 800), nil)
eq(layout.hit(zones, -5, -5), nil)
eq(layout.hit(zones, 5000, 5000), nil)
eq(layout.hit(zones, nil, nil), nil)

-- The exit button lives in the top right corner, where a close button is expected.
eq(zones.exit.x + zones.exit.w, W)
eq(zones.exit.y, 0)
-- The screen refresh button is its neighbour on the left, in the same row.
eq(zones.refresh.y, zones.exit.y)
eq(zones.refresh.x + zones.refresh.w, zones.exit.x)
-- The battery reading sits just left of both, on the same line.
assert(zones.battery.x + zones.battery.w <= zones.refresh.x, "battery overlaps the refresh button")
assert(zones.battery.x > W / 2, "battery belongs on the right half")
eq(layout.hit(zones, zones.battery.x + 5, zones.battery.y + 5), nil)

-- Everything about music sits inside the bottom panel, which hugs the bottom edge.
for _, name in ipairs({ "media_header", "cover", "track_text", "previous", "play_pause", "next",
        "volume_down", "volume_bar", "volume_up", "volume_value" }) do
    assert(inside(zones[name], zones.media_panel), name .. " is outside the media panel")
end
assert(zones.media_panel.y + zones.media_panel.h > H - 80, "media panel should sit at the bottom")
assert(zones.media_panel.y > H * 0.6, "media panel is too tall to leave the middle open")

-- One row: cover, track text, then the three playback buttons on the right.
eq(zones.previous.y, zones.play_pause.y)
eq(zones.previous.y, zones.next.y)
assert(zones.previous.x < zones.play_pause.x and zones.play_pause.x < zones.next.x, "previous, play, next run left to right")
assert(zones.cover.x + zones.cover.w <= zones.track_text.x, "track text starts after the cover")
assert(zones.track_text.x + zones.track_text.w <= zones.previous.x, "track text ends before the buttons")
assert(zones.track_text.w >= W * 0.25, "too little room left for the track title")
assert(not overlaps(zones.cover, zones.previous), "cover overlaps a button")
assert(zones.cover.y >= zones.previous.y and zones.cover.y + zones.cover.h <= zones.previous.y + zones.previous.h,
    "cover shares the buttons' row")

-- Below it, the volume row: quieter button, level bar, louder button, number.
assert(zones.volume_down.y >= zones.previous.y + zones.previous.h, "volume sits below playback")
eq(zones.volume_down.y, zones.volume_up.y)
eq(zones.volume_bar.y, zones.volume_down.y)
assert(zones.volume_down.x + zones.volume_down.w <= zones.volume_bar.x, "bar starts after the quieter button")
assert(zones.volume_bar.x + zones.volume_bar.w <= zones.volume_up.x, "bar ends before the louder button")
assert(zones.volume_up.x + zones.volume_up.w <= zones.volume_value.x, "value sits right of the louder button")
assert(zones.volume_bar.w >= W * 0.25, "bar is too short to read")
eq(layout.hit(zones, zones.volume_bar.x + zones.volume_bar.w / 2, zones.volume_bar.y + zones.volume_bar.h / 2), nil)
eq(layout.hit(zones, zones.cover.x + 5, zones.cover.y + 5), nil)

-- The header sits above the playback row.
assert(zones.media_header.y + zones.media_header.h <= zones.previous.y, "header overlaps the playback row")

-- The top panel starts near the top edge; its height depends on the content.
assert(zones.top_panel.y < 80, "top panel should sit at the top")
eq(zones.top_panel.x, zones.media_panel.x)
eq(zones.top_panel.w, zones.media_panel.w)
assert(zones.content.x > zones.top_panel.x and zones.content.w < zones.top_panel.w, "content is inset from the panel edge")

-- With the usual content the middle of the screen stays open for the background picture.
local top = layout.top_panel(zones, 360)
eq(top.x, zones.top_panel.x)
eq(top.y, zones.top_panel.y)
eq(top.w, zones.top_panel.w)
assert(top.h > 360, "the panel is taller than its content by its padding")
local open = zones.media_panel.y - (top.y + top.h)
assert(open >= H * 0.3, "the open middle is only " .. open .. " px tall")
-- A very tall top panel is stopped short of the media panel instead of covering it.
local huge = layout.top_panel(zones, 5000)
assert(huge.y + huge.h <= zones.media_panel.y, "top panel runs into the media panel")

-- Refresh regions: each kind of change repaints only its own band of the screen.
local regions = layout.regions(W, H, zones, { clock = 190, gap = 20, claude = 170 })
local function band(r)
    eq(r.x, 0)
    eq(r.w, W)
end
band(regions.clock)
band(regions.claude)
band(regions.media)
band(regions.all)
-- The clock band starts at the very top so the battery and exit icon refresh with it.
eq(regions.clock.y, 0)
eq(regions.clock.h, zones.content.y + 190)
eq(regions.claude.y, zones.content.y + 190 + 20)
-- The Claude band reaches the panel's lower edge, so the panel's own border area is repainted with it.
eq(regions.claude.y + regions.claude.h, layout.top_panel(zones, 190 + 20 + 170).y + layout.top_panel(zones, 190 + 20 + 170).h)
eq(regions.media.y, zones.media_panel.y)
eq(regions.media.y + regions.media.h, H)
eq(regions.all.y, 0)
eq(regions.all.h, H)
assert(regions.clock.y + regions.clock.h <= regions.claude.y, "clock and Claude bands overlap")
assert(regions.claude.y + regions.claude.h <= regions.media.y, "Claude and media bands overlap")

-- Content taller than the screen never yields a region outside it.
local tall = layout.regions(W, H, zones, { clock = 900, gap = 20, claude = 2000 })
for _, name in ipairs({ "clock", "claude", "media" }) do
    local r = tall[name]
    assert(r.y >= 0 and r.h >= 1 and r.y + r.h <= H, name .. " region leaves the screen")
end

-- Other portrait screens still get a valid, proportionally scaled layout.
check(1072, 1448, 120)
check(758, 1024, 90)
