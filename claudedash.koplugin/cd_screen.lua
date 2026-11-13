--[[
Turns a /screen.json reply into what has to be redrawn, and saves the last
screen to disk so the dashboard can show it while the PC is off.
]]
local screen = {}

local function is_rect(rect)
    if type(rect) ~= "table" then return false end
    for _, key in ipairs({ "x", "y", "w", "h" }) do
        if type(rect[key]) ~= "number" then return false end
    end
    return true
end

local function valid_widgets(list)
    local out = {}
    for _, w in ipairs(list) do
        if type(w) == "table" and type(w.kind) == "string" and is_rect(w.rect) and type(w.tree) == "table" then
            table.insert(out, {
                kind = w.kind,
                rect = { x = w.rect.x, y = w.rect.y, w = w.rect.w, h = w.rect.h },
                hash = type(w.hash) == "string" and w.hash or "",
                tree = w.tree,
            })
        end
    end
    return out
end

local function icon_map(icons)
    local out = {}
    if type(icons) ~= "table" then return out end
    for name, hash in pairs(icons) do
        if type(name) == "string" and type(hash) == "string" then
            out[name] = hash
        end
    end
    return out
end

--[[
Returns the new screen and { full = bool, widgets = { [kind] = true } }.
full means repaint everything: the first screen, or widgets shown, hidden,
moved or resized. A reply that is not a screen keeps the previous one.
]]
function screen.accept(previous, reply)
    local changes = { full = false, widgets = {} }
    if type(reply) ~= "table" or type(reply.widgets) ~= "table" then
        return previous, changes
    end

    local current = {
        layout = type(reply.layout) == "string" and reply.layout or "",
        widgets = valid_widgets(reply.widgets),
        icons = icon_map(reply.icons),
    }
    if not previous or previous.layout ~= current.layout then
        changes.full = true
        return current, changes
    end

    local old = {}
    for _, w in ipairs(previous.widgets) do
        old[w.kind] = w.hash
    end
    for _, w in ipairs(current.widgets) do
        if old[w.kind] ~= w.hash or w.hash == "" then
            changes.widgets[w.kind] = true
        end
    end
    return current, changes
end

local function encode_value(value, out)
    local kind = type(value)
    if kind == "string" then
        table.insert(out, string.format("%q", value))
    elseif kind == "number" then
        table.insert(out, string.format("%.17g", value))
    elseif kind == "boolean" then
        table.insert(out, tostring(value))
    elseif kind == "table" then
        table.insert(out, "{")
        for _, item in ipairs(value) do
            encode_value(item, out)
            table.insert(out, ",")
        end
        local keys = {}
        for key in pairs(value) do
            if type(key) == "string" then
                table.insert(keys, key)
            end
        end
        table.sort(keys)
        for _, key in ipairs(keys) do
            if type(value[key]) ~= "function" then
                table.insert(out, string.format("[%q]=", key))
                encode_value(value[key], out)
                table.insert(out, ",")
            end
        end
        table.insert(out, "}")
    else
        table.insert(out, "nil")
    end
end

function screen.encode(current)
    local out = { "return " }
    encode_value(current, out)
    return table.concat(out)
end

-- decode reads a saved screen. It runs with an empty environment, so a
-- tampered file can describe data but call nothing.
function screen.decode(text)
    if type(text) ~= "string" then return nil end
    local chunk = loadstring(text)
    if not chunk then return nil end
    setfenv(chunk, {})
    local ok, value = pcall(chunk)
    if not ok or type(value) ~= "table" or type(value.widgets) ~= "table" then return nil end
    return {
        layout = type(value.layout) == "string" and value.layout or "",
        widgets = valid_widgets(value.widgets),
        icons = icon_map(value.icons),
    }
end

local function collect_images(node, out, seen)
    if type(node) ~= "table" then return end
    if node.kind == "image" and type(node.src) == "string" then
        local key = "image:" .. node.src
        if not seen[key] then
            seen[key] = true
            table.insert(out, { key = key, path = node.src })
        end
    end
    if type(node.children) == "table" then
        for _, child in ipairs(node.children) do
            collect_images(child, out, seen)
        end
    end
end

-- images lists what the screen needs downloaded. The key changes with the
-- content, so a new cover or icon is fetched and a failed one is retried by key.
function screen.images(current)
    local out, seen = {}, {}
    if not current then return out end

    local names = {}
    for name in pairs(current.icons) do
        table.insert(names, name)
    end
    table.sort(names)
    for _, name in ipairs(names) do
        table.insert(out, {
            key = "icon:" .. name .. ":" .. current.icons[name],
            path = "/icons/" .. name .. ".svg",
            icon = name,
        })
    end
    for _, w in ipairs(current.widgets) do
        collect_images(w.tree, out, seen)
    end
    return out
end

return screen
