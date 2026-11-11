local async = require("cd_async")

local function eq(actual, expected)
    if actual ~= expected then
        error(string.format("expected %s, got %s", tostring(expected), tostring(actual)), 2)
    end
end

-- A fake world: tasks never run in a real subprocess. Each spawned handle is
-- completed by the test, and scheduled functions run when the test says so.
local function world()
    local w = { now = 1000, scheduled = {}, spawned = {}, killed = {}, logged = {}, spawn_fails = false }
    w.deps = {
        spawn = function(task)
            if w.spawn_fails then return nil end
            local handle = { task = task, state = "running" }
            table.insert(w.spawned, handle)
            return handle
        end,
        poll = function(handle) return handle.state end,
        collect = function(handle)
            handle.collected = true
            return handle.data
        end,
        kill = function(handle)
            handle.state = "killed"
            table.insert(w.killed, handle)
        end,
        schedule = function(_seconds, fn) table.insert(w.scheduled, fn) end,
        now = function() return w.now end,
        log = function(message) table.insert(w.logged, message) end,
    }
    -- Runs everything scheduled so far, once.
    function w.tick(seconds)
        w.now = w.now + (seconds or 0)
        local due = w.scheduled
        w.scheduled = {}
        for _, fn in ipairs(due) do fn() end
    end
    function w.complete(index, data)
        w.spawned[index].state = "ready"
        w.spawned[index].data = data
    end
    return w
end

-- A result arrives through the callback, never synchronously inside run().
do
    local w = world()
    local runner = async.new(w.deps)
    local got, calls = nil, 0
    runner:run("task-a", 5, function(data, err) got, calls = data, calls + 1 end)
    eq(#w.spawned, 1)
    eq(w.spawned[1].task, "task-a")
    eq(calls, 0)
    w.tick(1)
    eq(calls, 0)
    w.complete(1, "payload")
    w.tick(1)
    eq(calls, 1)
    eq(got, "payload")
    eq(w.spawned[1].collected, true)
    eq(#w.killed, 0)
end

-- Only one job runs at a time; the rest wait their turn in order.
do
    local w = world()
    local runner = async.new(w.deps)
    local order = {}
    runner:run("first", 5, function() table.insert(order, "first") end)
    runner:run("second", 5, function() table.insert(order, "second") end)
    runner:run("third", 5, function() table.insert(order, "third") end)
    eq(#w.spawned, 1)
    w.complete(1, "x")
    w.tick(1)
    eq(#w.spawned, 2)
    eq(w.spawned[2].task, "second")
    w.complete(2, "x")
    w.tick(1)
    w.complete(3, "x")
    w.tick(1)
    eq(table.concat(order, ","), "first,second,third")
end

-- An urgent job (a button tap) jumps ahead of queued polls, but not the running one.
do
    local w = world()
    local runner = async.new(w.deps)
    runner:run("running", 5, function() end)
    runner:run("poll", 5, function() end)
    runner:run("tap", 5, function() end, true)
    w.complete(1, "x")
    w.tick(1)
    eq(w.spawned[2].task, "tap")
end

-- A job that never finishes is killed at its deadline and reported as a timeout.
do
    local w = world()
    local runner = async.new(w.deps)
    local got_data, got_err = "unset", nil
    runner:run("stuck", 5, function(data, err) got_data, got_err = data, err end)
    w.tick(4)
    eq(got_err, nil)
    eq(#w.killed, 0)
    w.tick(1)
    eq(#w.killed, 1)
    eq(got_data, nil)
    eq(got_err, "timeout")
    -- The queue keeps moving after a timeout.
    runner:run("next", 5, function() end)
    eq(#w.spawned, 2)
end

-- Polling continues for as long as the job is running: the UI is never blocked waiting.
do
    local w = world()
    local runner = async.new(w.deps)
    runner:run("slow", 60, function() end)
    for _ = 1, 20 do
        eq(#w.scheduled, 1)
        w.tick(1)
    end
    eq(#w.killed, 0)
end

-- Failing to start a subprocess is an error for that job only.
do
    local w = world()
    local runner = async.new(w.deps)
    local first_err, second_ok
    w.spawn_fails = true
    runner:run("a", 5, function(_data, err) first_err = err end)
    w.tick(0)
    eq(first_err, "spawn failed")
    w.spawn_fails = false
    runner:run("b", 5, function(data) second_ok = data end)
    w.complete(1, "fine")
    w.tick(1)
    eq(second_ok, "fine")
end

-- A callback that raises is logged and does not stop later jobs.
do
    local w = world()
    local runner = async.new(w.deps)
    -- Kept in a table: the test VM (gopher-lua) loses track of plain locals
    -- captured by closures once an error has unwound through pcall.
    local seen = { reached = false }
    runner:run("bad", 5, function() error("boom") end)
    runner:run("good", 5, function() seen.reached = true end)
    w.complete(1, "x")
    w.tick(1)
    eq(#w.logged, 1)
    w.complete(2, "x")
    w.tick(1)
    eq(seen.reached, true)
end

-- Cancelling kills the running job, drops the queue, and calls nobody back.
do
    local w = world()
    local runner = async.new(w.deps)
    local calls = 0
    runner:run("running", 5, function() calls = calls + 1 end)
    runner:run("queued", 5, function() calls = calls + 1 end)
    runner:cancel()
    eq(#w.killed, 1)
    w.tick(10)
    eq(calls, 0)
    eq(#w.spawned, 1)
    eq(runner:is_idle(), true)
    -- A cancelled runner can be used again.
    runner:run("again", 5, function() calls = calls + 1 end)
    w.complete(2, "x")
    w.tick(1)
    eq(calls, 1)
end

eq(async.new(world().deps):is_idle(), true)

-- Replies cross the pipe as "<status>\n<body>".
local code, body = async.split_reply("200\n{\"a\":1}")
eq(code, 200)
eq(body, "{\"a\":1}")
code, body = async.split_reply("200\n")
eq(code, 200)
eq(body, "")
code, body = async.split_reply("404\nnot found\nsecond line")
eq(code, 404)
eq(body, "not found\nsecond line")
-- LuaSocket reports failures as words in place of a status code.
code, body = async.split_reply("timeout\n")
eq(code, nil)
eq(body, "timeout")
for _, broken in ipairs({ "", "200", "garbage" }) do
    code = async.split_reply(broken)
    eq(code, nil)
end
eq((async.split_reply(nil)), nil)
eq(async.join_reply(200, "abc"), "200\nabc")
eq(async.join_reply("closed", ""), "closed\n")
eq(async.join_reply(nil, nil), "nil\n")

-- Several taps run in the order they were made, ahead of queued polls.
do
    local w = world()
    local runner = async.new(w.deps)
    runner:run("tap-0", 5, function() end, true)
    runner:run("poll", 5, function() end)
    runner:run("tap-1", 5, function() end, true)
    runner:run("tap-2", 5, function() end, true)
    local order = {}
    for i = 1, 4 do
        table.insert(order, w.spawned[i].task)
        w.complete(i, "x")
        w.tick(1)
    end
    eq(table.concat(order, ","), "tap-0,tap-1,tap-2,poll")
end

-- A tap does not wait behind a slow download: the download is stopped and redone afterwards.
do
    local w = world()
    local runner = async.new(w.deps)
    local results = {}
    runner:run("download", 18, function(data, err) table.insert(results, "download:" .. tostring(data or err)) end)
    runner:run("poll", 5, function() table.insert(results, "poll") end)
    runner:run("tap", 5, function() table.insert(results, "tap") end, true)
    eq(#w.killed, 1)
    eq(w.killed[1].task, "download")
    eq(#w.spawned, 2)
    eq(w.spawned[2].task, "tap")
    eq(#results, 0)
    w.complete(2, "x")
    w.tick(1)
    -- The interrupted download goes first among the ordinary jobs, and reports once, with its real result.
    eq(w.spawned[3].task, "download")
    w.complete(3, "picture")
    w.tick(1)
    w.complete(4, "x")
    w.tick(1)
    eq(table.concat(results, ","), "tap,download:picture,poll")
end

-- A running tap is never interrupted by another tap.
do
    local w = world()
    local runner = async.new(w.deps)
    runner:run("tap-a", 5, function() end, true)
    runner:run("tap-b", 5, function() end, true)
    eq(#w.killed, 0)
    eq(#w.spawned, 1)
end

-- Calling run() or cancel() from inside a callback is safe.
do
    local w = world()
    local runner = async.new(w.deps)
    local seen = { second = false }
    runner:run("first", 5, function()
        runner:run("second", 5, function() seen.second = true end)
    end)
    w.complete(1, "x")
    w.tick(1)
    eq(#w.spawned, 2)
    w.complete(2, "x")
    w.tick(1)
    eq(seen.second, true)

    runner:run("third", 5, function() runner:cancel() end)
    runner:run("fourth", 5, function() seen.fourth = true end)
    w.complete(3, "x")
    w.tick(1)
    eq(runner:is_idle(), true)
    eq(seen.fourth, nil)
end

eq(async.pending_urgent(async.new(world().deps)), 0)
do
    local w = world()
    local runner = async.new(w.deps)
    runner:run("tap-a", 5, function() end, true)
    runner:run("poll", 5, function() end)
    runner:run("tap-b", 5, function() end, true)
    eq(async.pending_urgent(runner), 2)
    w.complete(1, "x")
    w.tick(1)
    eq(async.pending_urgent(runner), 1)
end
