--[[
Runs slow jobs (network requests) one at a time without ever blocking the
caller. Each job runs somewhere else (in practice a subprocess), is polled on a
timer, and is killed if it outlives its deadline. KOReader has a single UI
thread, so a blocking call there freezes touch input and the screen.

The outside world is injected as `deps` so the scheduling logic can be tested:
    spawn(task)       -> handle, or nil when the job could not be started
    poll(handle)      -> "ready" once the result can be collected, else "running"
    collect(handle)   -> the job's result as a string
    kill(handle)      -> stop the job and release its resources
    schedule(seconds, fn)
    now()             -> seconds
    log(message)
]]
local async = {}

local POLL_SECONDS = 0.2

local Runner = {}
Runner.__index = Runner

function async.new(deps)
    return setmetatable({ deps = deps, queue = {}, current = nil }, Runner)
end

function Runner:is_idle()
    return self.current == nil and #self.queue == 0
end

-- Number of urgent jobs running or waiting, so a caller can refuse to pile up more.
function async.pending_urgent(runner)
    local count = (runner.current and runner.current.urgent) and 1 or 0
    for _, job in ipairs(runner.queue) do
        if job.urgent then
            count = count + 1
        end
    end
    return count
end

-- Urgent jobs keep their own order and sit ahead of every ordinary job.
local function first_ordinary_index(queue)
    for index, job in ipairs(queue) do
        if not job.urgent then return index end
    end
    return #queue + 1
end

-- callback(data, err) always runs later, never from inside run().
-- An urgent job (a button tap) goes ahead of ordinary ones, and interrupts an
-- ordinary job that is already running; that job is started again afterwards.
function Runner:run(task, timeout, callback, urgent)
    local job = { task = task, timeout = timeout, callback = callback, urgent = urgent == true }
    if job.urgent then
        table.insert(self.queue, first_ordinary_index(self.queue), job)
        self:preempt()
    else
        table.insert(self.queue, job)
    end
    self:pump()
end

function Runner:preempt()
    local running = self.current
    if not running or running.urgent or not running.handle then return end

    self.deps.kill(running.handle)
    running.handle = nil
    self.current = nil
    table.insert(self.queue, first_ordinary_index(self.queue), running)
end

function Runner:pump()
    if self.current or #self.queue == 0 then return end

    local job = table.remove(self.queue, 1)
    self.current = job
    -- Identifies this attempt, so timers left over from an interrupted attempt do nothing.
    job.attempt = (job.attempt or 0) + 1
    local attempt = job.attempt
    job.handle = self.deps.spawn(job.task)
    if not job.handle then
        self.deps.schedule(0, function() self:finish(job, attempt, nil, "spawn failed") end)
        return
    end
    job.deadline = self.deps.now() + job.timeout
    self.deps.schedule(POLL_SECONDS, function() self:check(job, attempt) end)
end

function Runner:is_current(job, attempt)
    return self.current == job and job.attempt == attempt
end

function Runner:check(job, attempt)
    if not self:is_current(job, attempt) then return end

    if self.deps.poll(job.handle) == "ready" then
        self:finish(job, attempt, self.deps.collect(job.handle), nil)
        return
    end
    if self.deps.now() >= job.deadline then
        self.deps.kill(job.handle)
        self:finish(job, attempt, nil, "timeout")
        return
    end
    self.deps.schedule(POLL_SECONDS, function() self:check(job, attempt) end)
end

function Runner:finish(job, attempt, data, err)
    if not self:is_current(job, attempt) then return end
    self.current = nil

    local ok, failure = pcall(job.callback, data, err)
    if not ok then
        self.deps.log("claudedash: callback failed: " .. tostring(failure))
    end
    self:pump()
end

-- Stops everything without calling any callback.
function Runner:cancel()
    local job = self.current
    self.current = nil
    self.queue = {}
    if job and job.handle then
        self.deps.kill(job.handle)
    end
end

-- A job's result crosses the pipe as "<status>\n<body>". The status is an HTTP
-- code, or the word LuaSocket uses for a failure such as "timeout" or "closed".
function async.join_reply(status, body)
    return tostring(status) .. "\n" .. (body or "")
end

-- Returns the numeric status and the body, or nil and a reason.
function async.split_reply(payload)
    if type(payload) ~= "string" then return nil, "no reply" end

    local newline = payload:find("\n", 1, true)
    if not newline then return nil, "malformed reply" end

    local status = payload:sub(1, newline - 1)
    local code = tonumber(status)
    if not code then return nil, status end
    return code, payload:sub(newline + 1)
end

return async
