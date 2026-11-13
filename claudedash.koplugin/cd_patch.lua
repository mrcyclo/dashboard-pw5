--[[
Predicted changes for taps. A button may carry a patch: a list of ops on
nodes of its own widget, found by id. The Kindle applies it at once and keeps
the predicted values until the PC reports the same values, or for HOLD_SECONDS.
]]
local tree = require("cd_tree")

local patch = {}

-- The PC's reply to a command often still carries the old state for a moment.
local HOLD_SECONDS = 4
patch.HOLD_SECONDS = HOLD_SECONDS

local function copy(value)
    if type(value) ~= "table" then return value end
    local out = {}
    for key, item in pairs(value) do
        out[key] = copy(item)
    end
    return out
end

local function clamp(value, low, high)
    if type(low) == "number" and value < low then return low end
    if type(high) == "number" and value > high then return high end
    return value
end

-- next_value returns the predicted value, or nil when the op does not apply.
local function next_value(op, current)
    if op.op == "set" then
        return op.value
    end
    if op.op == "toggle" then
        if type(op.values) ~= "table" or op.values[1] == nil then return nil end
        if current == op.values[1] then
            return op.values[2]
        end
        return op.values[1]
    end
    if op.op == "add" then
        if type(op.by) ~= "number" then return nil end
        local number = current
        if type(current) == "string" then
            local digits = current:match("^%-?%d+$")
            number = digits and tonumber(digits)
        end
        if type(number) ~= "number" then return nil end
        local result = clamp(number + op.by, op.min, op.max)
        if type(current) == "string" then
            return tostring(math.floor(result))
        end
        return result
    end
    return nil
end

-- apply returns a patched copy of root, plus the values it set as { [id] = { [prop] = value } }.
function patch.apply(root, ops)
    local out = copy(root)
    local set = {}
    if type(ops) ~= "table" then return out, set end

    for _, op in ipairs(ops) do
        if type(op) == "table" and type(op.id) == "string" and type(op.prop) == "string" then
            local node = tree.find(out, op.id)
            local value = node and next_value(op, node[op.prop])
            if value ~= nil then
                node[op.prop] = value
                set[op.id] = set[op.id] or {}
                set[op.id][op.prop] = value
            end
        end
    end
    return out, set
end

-- stack applies a tap's ops to what is shown and adds them to the pending prediction.
function patch.stack(pending, ops, shown, now)
    local result, set = patch.apply(shown, ops)
    local next_pending = { expires = now + HOLD_SECONDS, values = {} }
    if pending then
        for id, props in pairs(pending.values) do
            next_pending.values[id] = copy(props)
        end
    end
    for id, props in pairs(set) do
        next_pending.values[id] = next_pending.values[id] or {}
        for prop, value in pairs(props) do
            next_pending.values[id][prop] = value
        end
    end
    if next(next_pending.values) == nil then
        return result, pending
    end
    return result, next_pending
end

-- reconcile returns the tree to show for a new reply and what is still pending.
function patch.reconcile(pending, incoming, now)
    if not pending or now >= pending.expires then return incoming, nil end

    local agreed = true
    for id, props in pairs(pending.values) do
        local node = tree.find(incoming, id)
        if not node then return incoming, nil end
        for prop, value in pairs(props) do
            if node[prop] ~= value then
                agreed = false
            end
        end
    end
    if agreed then return incoming, nil end

    local shown = copy(incoming)
    for id, props in pairs(pending.values) do
        local node = tree.find(shown, id)
        for prop, value in pairs(props) do
            node[prop] = value
        end
    end
    return shown, pending
end

-- expire ends a prediction whose hold ran out while no reply came. It returns
-- the tree to show again (the PC's last one) and nil, or nil and the pending
-- prediction while it still holds.
function patch.expire(pending, last, now)
    if not pending then return nil, nil end
    if now < pending.expires then return nil, pending end
    return last, nil
end

return patch
