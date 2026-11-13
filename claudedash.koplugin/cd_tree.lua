--[[
Lays out a widget's component tree, as sent by the PC in /screen.json.
The window's preview does the same in server/frontend/src/lib/tree.js, and
both are checked against server/internal/widget/testdata/tree_fixtures.json.

Rules:
1. col stacks its children; they get its width when align is "stretch"
   (default) or are bars and rules, else their own width aligned by align.
   Leftover height is shared by "grow" weight.
2. row places children left to right. A growing child starts at width 0 and
   takes its share of the leftover width; others keep their own width.
   Children of a row are aligned vertically by align ("center" default),
   except cols and rules, which take the row's full height.
3. Unknown kinds are skipped. Every box is clipped to the root box, and a
   text line too short to show whole is left out.
4. x and y are floored, then w and h.
]]
local format = require("cd_format")

local tree = {}

tree.KNOWN = {
    col = true, row = true, space = true, rule = true, text = true, clock = true,
    countdown = true, bar = true, icon = true, image = true, button = true,
}

local TEXTUAL = { text = true, clock = true, countdown = true }
local LINE_HEIGHT = 1.3

-- KOReader's JSON decoder yields a function for JSON null, so check types, not nil.
local function num(value, default)
    if type(value) ~= "number" then return default end
    return value
end

local function str(value)
    if type(value) ~= "string" then return "" end
    return value
end

local function known(node)
    return type(node) == "table" and tree.KNOWN[node.kind] == true
end

-- resolve returns the node with its "at" values applied once their time has passed.
function tree.resolve(node, now)
    local at = node.at
    if type(at) ~= "table" or type(at.set) ~= "table" then return node end
    local time = num(at.time, nil)
    if not time or now < time then return node end

    local copy = {}
    for key, value in pairs(node) do
        copy[key] = value
    end
    for key, value in pairs(at.set) do
        copy[key] = value
    end
    return copy
end

local function text_of(node, ctx)
    if node.kind == "clock" then
        local t = ctx.date_fn("*t", ctx.now)
        if node.format == "date" then
            return format.date_text(t)
        end
        return format.clock(t)
    end
    if node.kind == "countdown" then
        return format.countdown_text(node.to, ctx.now, ctx.date_fn) or ""
    end
    return str(node.text)
end

local function children_of(node, ctx)
    local list = {}
    if type(node.children) ~= "table" then return list end
    for _, child in ipairs(node.children) do
        if known(child) then
            table.insert(list, tree.resolve(child, ctx.now))
        end
    end
    return list
end

local function text_height(size)
    return math.ceil(size * LINE_HEIGHT)
end

-- size returns a node's own width and height; axis is the parent's direction.
local function size(node, axis, ctx)
    local kind = node.kind
    if TEXTUAL[kind] then
        local font = num(node.size, 20)
        return ctx.measure(text_of(node, ctx), font, node.bold == true), text_height(font)
    end
    if kind == "icon" or kind == "button" then
        local side = num(node.size, 0)
        return side, side
    end
    if kind == "image" then
        return num(node.w, 0), num(node.h, 0)
    end
    if kind == "bar" then
        return 0, num(node.height, 8)
    end
    if kind == "rule" or kind == "space" then
        local amount = kind == "rule" and num(node.thickness, 2) or num(node.size, 0)
        if axis == "row" then
            return amount, 0
        end
        return 0, amount
    end

    local gap = num(node.gap, 0)
    local w, h, count = 0, 0, 0
    for _, child in ipairs(children_of(node, ctx)) do
        local cw, ch = size(child, kind, ctx)
        count = count + 1
        if kind == "col" then
            w, h = math.max(w, cw), h + ch
        else
            w, h = w + cw, math.max(h, ch)
        end
    end
    local gaps = math.max(count - 1, 0) * gap
    if kind == "col" then
        return w, h + gaps
    end
    return w + gaps, h
end

local function grow_of(node)
    local weight = num(node.grow, 0)
    if weight < 0 then return 0 end
    return weight
end

local place

local function place_col(node, x, y, w, h, ctx)
    local kids = children_of(node, ctx)
    local gap = num(node.gap, 0)
    local align = str(node.align)
    if align == "" then align = "stretch" end

    local sizes, used, weights = {}, 0, 0
    for i, child in ipairs(kids) do
        local cw, ch = size(child, "col", ctx)
        sizes[i] = { w = cw, h = ch }
        used = used + ch
        weights = weights + grow_of(child)
    end
    local leftover = math.max(h - used - math.max(#kids - 1, 0) * gap, 0)

    local cy = y
    for i, child in ipairs(kids) do
        local ch = sizes[i].h
        if weights > 0 then
            ch = ch + leftover * grow_of(child) / weights
        end
        local cw, cx = w, x
        local stretches = align == "stretch" or child.kind == "bar" or child.kind == "rule"
        if not stretches then
            cw = math.min(sizes[i].w, w)
            if align == "center" then
                cx = x + (w - cw) / 2
            elseif align == "end" then
                cx = x + w - cw
            end
        end
        place(child, cx, cy, cw, ch, ctx)
        cy = cy + ch + gap
    end
end

local function place_row(node, x, y, w, h, ctx)
    local kids = children_of(node, ctx)
    local gap = num(node.gap, 0)
    local align = str(node.align)
    if align == "" then align = "center" end

    local sizes, used, weights = {}, 0, 0
    for i, child in ipairs(kids) do
        local cw, ch = size(child, "row", ctx)
        if grow_of(child) > 0 then
            cw = 0
        end
        sizes[i] = { w = cw, h = ch }
        used = used + cw
        weights = weights + grow_of(child)
    end
    local leftover = math.max(w - used - math.max(#kids - 1, 0) * gap, 0)

    local cx = x
    for i, child in ipairs(kids) do
        local cw = sizes[i].w
        if weights > 0 then
            cw = cw + leftover * grow_of(child) / weights
        end
        local ch, cy = h, y
        if child.kind ~= "col" and child.kind ~= "rule" then
            ch = sizes[i].h
            if align == "center" then
                cy = y + (h - ch) / 2
            elseif align == "end" then
                cy = y + h - ch
            end
        end
        place(child, cx, cy, cw, ch, ctx)
        cx = cx + cw + gap
    end
end

local function emit(node, x, y, w, h, ctx)
    local box = ctx.root
    local x0, y0 = math.floor(x), math.floor(y)
    local w0, h0 = math.floor(w), math.floor(h)
    local left, top = math.max(x0, box.x), math.max(y0, box.y)
    local right, bottom = math.min(x0 + w0, box.x + box.w), math.min(y0 + h0, box.y + box.h)
    if right <= left or bottom <= top then return end
    -- A line of text cannot be cut: KOReader would draw all of it, past the widget.
    if TEXTUAL[node.kind] and bottom - top < text_height(num(node.size, 20)) then return end

    local out = { node = node, x = left, y = top, w = right - left, h = bottom - top }
    if TEXTUAL[node.kind] then
        out.text = text_of(node, ctx)
    end
    table.insert(ctx.boxes, out)
end

place = function(node, x, y, w, h, ctx)
    if node.kind == "col" then
        place_col(node, x, y, w, h, ctx)
    elseif node.kind == "row" then
        place_row(node, x, y, w, h, ctx)
    elseif node.kind ~= "space" then
        emit(node, x, y, w, h, ctx)
    end
end

--[[
Returns the drawable boxes in paint order: { node, x, y, w, h, text }.
measure(text, size, bold) gives a text's width in pixels.
]]
function tree.layout(root, box, measure, now, date_fn)
    local ctx = { boxes = {}, root = box, measure = measure, now = now or os.time(), date_fn = date_fn or os.date }
    if not known(root) then return ctx.boxes end

    place(tree.resolve(root, ctx.now), box.x, box.y, box.w, box.h, ctx)
    return ctx.boxes
end

function tree.find(node, id)
    if type(node) ~= "table" then return nil end
    if node.id == id then return node end
    if type(node.children) ~= "table" then return nil end
    for _, child in ipairs(node.children) do
        local found = tree.find(child, id)
        if found then return found end
    end
    return nil
end

return tree
