local patch = require("cd_patch")
local tree = require("cd_tree")

local function eq(actual, expected, what)
    if actual ~= expected then
        error(string.format("%s: expected %s, got %s", what or "value", tostring(expected), tostring(actual)), 2)
    end
end

local function media(icon, volume, text)
    return { kind = "col", children = {
        { kind = "button", id = "pp", icon = icon, size = 120 },
        { kind = "bar", id = "vol-bar", percent = volume, height = 16 },
        { kind = "text", id = "vol-num", text = text or tostring(volume), size = 43 },
    } }
end

local toggle = { { id = "pp", op = "toggle", prop = "icon", values = { "pause", "play" } } }
local up = {
    { id = "vol-bar", op = "add", prop = "percent", by = 5, min = 0, max = 100 },
    { id = "vol-num", op = "add", prop = "text", by = 5, min = 0, max = 100 },
}

-- apply copies: the original tree is untouched.
local original = media("pause", 20)
local changed = patch.apply(original, toggle)
eq(tree.find(changed, "pp").icon, "play", "toggle")
eq(tree.find(original, "pp").icon, "pause", "original untouched")
eq(tree.find(patch.apply(changed, toggle), "pp").icon, "pause", "toggle back")
eq(tree.find(patch.apply(media("stop", 20), toggle), "pp").icon, "pause", "anything else becomes the first value")

-- add works on numbers and on text holding a number, clamped.
local louder = patch.apply(patch.apply(original, up), up)
eq(tree.find(louder, "vol-bar").percent, 30, "add twice")
eq(tree.find(louder, "vol-num").text, "30", "add on text")
eq(tree.find(patch.apply(media("pause", 98), up), "vol-bar").percent, 100, "clamped at max")
local down = { { id = "vol-bar", op = "add", prop = "percent", by = -5, min = 0, max = 100 } }
eq(tree.find(patch.apply(media("pause", 3), down), "vol-bar").percent, 0, "clamped at min")
eq(tree.find(patch.apply(media("pause", 0, "Mute"), up), "vol-num").text, "Mute", "add skips text that is not a number")
eq(tree.find(patch.apply(original, { { id = "pp", op = "set", prop = "icon", value = "next" } }), "pp").icon, "next", "set")
eq(tree.find(patch.apply(original, { { id = "nope", op = "set", prop = "icon", value = "x" } }), "pp").icon, "pause", "missing id is ignored")

-- stack applies on what is shown, so quick taps add up.
local now = 1000
local shown, pending = patch.stack(nil, up, original, now)
shown, pending = patch.stack(pending, up, shown, now + 1)
eq(tree.find(shown, "vol-bar").percent, 30, "two taps")
shown, pending = patch.stack(nil, toggle, original, now)
shown, pending = patch.stack(pending, toggle, shown, now)
eq(tree.find(shown, "pp").icon, "pause", "two play/pause taps cancel out")

-- reconcile keeps the prediction while the PC still reports the old state.
shown, pending = patch.stack(nil, toggle, original, now)
local stale = media("pause", 25)
local result, still = patch.reconcile(pending, stale, now + 1)
eq(tree.find(result, "pp").icon, "play", "prediction kept")
eq(tree.find(result, "vol-bar").percent, 25, "other values come from the PC")
assert(still ~= nil, "still pending")

-- The PC agreeing ends the prediction.
result, still = patch.reconcile(pending, media("play", 25), now + 1)
eq(tree.find(result, "pp").icon, "play", "agreed")
assert(still == nil, "settled")

-- After 4 seconds the PC wins.
result, still = patch.reconcile(pending, stale, now + 4)
eq(tree.find(result, "pp").icon, "pause", "expired")
assert(still == nil, "expired pending is dropped")

-- A widget that lost the patched node (hidden, moved, replaced) drops the prediction.
result, still = patch.reconcile(pending, { kind = "col", children = {} }, now + 1)
assert(still == nil and result.kind == "col", "missing node drops the prediction")

-- No prediction: the incoming tree is shown as is.
result, still = patch.reconcile(nil, stale, now)
assert(result == stale and still == nil, "nothing pending")

-- Bad patches are ignored rather than breaking the tap.
local json_null = function() end
shown, pending = patch.stack(nil, json_null, original, now)
eq(tree.find(shown, "pp").icon, "pause", "null patch")
shown = patch.apply(original, { json_null, { id = "pp", op = "explode", prop = "icon" } })
eq(tree.find(shown, "pp").icon, "pause", "unknown op")
