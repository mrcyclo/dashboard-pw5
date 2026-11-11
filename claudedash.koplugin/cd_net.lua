--[[
All HTTP to the PC goes through here, and none of it runs on the UI thread.
Each request runs in a short-lived subprocess that is polled on a timer and
killed if it overruns, so a network problem can cost a missed update but can
never freeze the touch screen. See cd_async for the scheduling.
]]
local JSON = require("json")
local UIManager = require("ui/uimanager")
local ffi = require("ffi")
local ffiutil = require("ffi/util")
local http = require("socket.http")
local logger = require("logger")
local socket = require("socket")
local socketutil = require("socketutil")
local async = require("cd_async")

local net = {}

-- Added to a request's socket timeout before its subprocess is killed outright.
local KILL_MARGIN_SECONDS = 3
local REAP_SECONDS = 1

-- Finished or killed children stay as zombies until waited for.
local unreaped = {}
local reaping = false

local function reap()
    reaping = false
    local remaining = {}
    for _, pid in ipairs(unreaped) do
        if not ffiutil.isSubProcessDone(pid) then
            table.insert(remaining, pid)
        end
    end
    unreaped = remaining
    if #unreaped == 0 then return end

    reaping = true
    UIManager:scheduleIn(REAP_SECONDS, reap)
end

local function retire(handle)
    if handle.reaped then return end

    table.insert(unreaped, handle.pid)
    if reaping then return end
    reaping = true
    UIManager:scheduleIn(REAP_SECONDS, reap)
end

local runner = async.new{
    spawn = function(task)
        local pid, read_fd = ffiutil.runInSubProcess(function(_pid, write_fd)
            local ok, reply = pcall(task)
            ffiutil.writeToFD(write_fd, ok and reply or async.join_reply("error", tostring(reply)), true)
        end, true)
        if not pid then
            logger.warn("claudedash: cannot start a subprocess:", read_fd)
            return nil
        end
        return { pid = pid, fd = read_fd }
    end,
    poll = function(handle)
        -- The child writes its whole reply at once, so any readable byte means it is done working.
        local readable = ffiutil.getNonBlockingReadSize(handle.fd)
        if readable and readable > 0 then return "ready" end
        if ffiutil.isSubProcessDone(handle.pid) then
            handle.reaped = true
            return "ready"
        end
        return "running"
    end,
    collect = function(handle)
        local data = ffiutil.readAllFromFD(handle.fd)
        retire(handle)
        return data
    end,
    kill = function(handle)
        ffiutil.terminateSubProcess(handle.pid)
        ffi.C.close(handle.fd)
        retire(handle)
    end,
    schedule = function(seconds, fn) UIManager:scheduleIn(seconds, fn) end,
    now = os.time,
    log = function(message) logger.warn(message) end,
}

-- Runs inside the subprocess.
local function http_task(config, method, path, timeout)
    local url = string.format("http://%s:%d%s", config.host, config.port, path)
    return function()
        local chunks = {}
        socketutil:set_timeout(timeout, timeout)
        local code = socket.skip(1, http.request{
            url = url,
            method = method,
            headers = method == "POST" and { ["Content-Length"] = "0" } or nil,
            sink = socketutil.table_sink(chunks),
        })
        return async.join_reply(code, table.concat(chunks))
    end
end

local function request(config, method, path, timeout, on_body, urgent)
    runner:run(http_task(config, method, path, timeout), timeout + KILL_MARGIN_SECONDS, function(payload, err)
        if not payload then return on_body(nil, err) end

        local code, body = async.split_reply(payload)
        if code ~= 200 then return on_body(nil, code and ("http " .. code) or body) end
        on_body(body)
    end, urgent)
end

-- callback(table) on success, callback(nil, reason) otherwise. urgent requests
-- go ahead of anything still waiting in the queue.
function net.json(config, method, path, timeout, callback, urgent)
    request(config, method, path, timeout, function(body, err)
        if not body then return callback(nil, err) end

        local ok, value = pcall(JSON.decode, body)
        if not ok or type(value) ~= "table" then return callback(nil, "bad json") end
        callback(value)
    end, urgent)
end

-- callback(bytes) on success, callback(nil, reason) otherwise.
function net.raw(config, path, timeout, callback)
    request(config, "GET", path, timeout, function(body, err)
        if not body or body == "" then return callback(nil, err or "empty") end
        callback(body)
    end)
end

-- Taps still waiting for the PC. Callers use it to stop a dead connection
-- from collecting a long line of commands that would all fire late.
function net.pending_commands()
    return async.pending_urgent(runner)
end

-- Drops every pending request without calling back.
function net.cancel()
    runner:cancel()
end

return net
