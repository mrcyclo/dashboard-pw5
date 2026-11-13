local tree = require("cd_tree")

-- FIXTURES comes from server/internal/widget/testdata/tree_fixtures.json,
-- loaded by the Go test runner; the window's preview checks the same file.
assert(type(FIXTURES) == "table" and #FIXTURES > 0, "fixtures were not loaded")

local function measure(text, size)
    return math.floor(#text * size / 2)
end

local function fail(name, message)
    error(name .. ": " .. message, 2)
end

for _, fixture in ipairs(FIXTURES) do
    local box = { x = 0, y = 0, w = fixture.box.w, h = fixture.box.h }
    local boxes = tree.layout(fixture.tree, box, measure, 0)
    local placed = {}
    for _, b in ipairs(boxes) do
        if b.node.id then
            table.insert(placed, b)
        end
        if b.x < 0 or b.y < 0 or b.x + b.w > box.w or b.y + b.h > box.h then
            fail(fixture.name, "a box leaves the widget")
        end
    end
    if #placed ~= #fixture.expect then
        fail(fixture.name, string.format("expected %d boxes, got %d", #fixture.expect, #placed))
    end
    for i, want in ipairs(fixture.expect) do
        local got = placed[i]
        if got.node.id ~= want.id or got.x ~= want.x or got.y ~= want.y or got.w ~= want.w or got.h ~= want.h then
            fail(fixture.name, string.format(
                "%s: expected %d,%d %dx%d, got %s %d,%d %dx%d",
                want.id, want.x, want.y, want.w, want.h, tostring(got.node.id), got.x, got.y, got.w, got.h
            ))
        end
    end
end

-- Text boxes carry the string to draw.
local boxes = tree.layout({ kind = "text", id = "t", text = "Hi", size = 10 }, { x = 5, y = 7, w = 50, h = 20 }, measure, 0)
assert(#boxes == 1 and boxes[1].text == "Hi" and boxes[1].x == 5 and boxes[1].y == 7, "a root leaf fills the box")

-- Clocks and countdowns are filled in on the Kindle.
local now = 1794400000
local function fake_date(fmt, epoch)
    return { wday = 2, hour = 9, min = 5, day = 13, month = 11, year = 2026 }
end
boxes = tree.layout({ kind = "col", gap = 0, children = {
    { kind = "clock", format = "time", size = 20 },
    { kind = "clock", format = "date", size = 20 },
    { kind = "countdown", to = now + 90, size = 20 },
    { kind = "countdown", to = now - 1, size = 20 },
} }, { x = 0, y = 0, w = 400, h = 200 }, measure, now, fake_date)
assert(boxes[1].text == "09:05", "time: " .. tostring(boxes[1].text))
assert(boxes[2].text == "Monday, 13/11/2026", "date: " .. tostring(boxes[2].text))
assert(boxes[3].text == "resets in 2m", "countdown: " .. tostring(boxes[3].text))
assert(boxes[4].text == "", "a past countdown draws nothing")

-- A node switches to its "at" values once the time has passed.
local node = { kind = "text", text = "74% used", size = 10, at = { time = now, set = { text = "0% used" } } }
assert(tree.layout(node, { x = 0, y = 0, w = 100, h = 20 }, measure, now - 1)[1].text == "74% used", "before the switch")
assert(tree.layout(node, { x = 0, y = 0, w = 100, h = 20 }, measure, now)[1].text == "0% used", "after the switch")
assert(node.text == "74% used", "the tree itself is not changed")
local bar = tree.resolve({ kind = "bar", percent = 74, at = { time = now, set = { percent = 0 } } }, now + 5)
assert(bar.percent == 0, "bar switch")

-- JSON null arrives as a function and must not break anything.
local json_null = function() end
boxes = tree.layout({ kind = "col", gap = json_null, children = {
    { kind = "text", text = json_null, size = json_null, at = json_null },
    json_null,
    { kind = "bar", percent = 10, height = 4 },
} }, { x = 0, y = 0, w = 100, h = 100 }, measure, now)
assert(#boxes == 2 and boxes[1].text == "", "null fields fall back to defaults")
assert(tree.layout(json_null, { x = 0, y = 0, w = 10, h = 10 }, measure, now)[1] == nil, "a null root draws nothing")

-- find looks up a node by id anywhere in the tree.
local root = { kind = "row", children = { { kind = "col", children = { { kind = "button", id = "pp", size = 10 } } } } }
assert(tree.find(root, "pp").size == 10, "find nested")
assert(tree.find(root, "nope") == nil, "find missing")
