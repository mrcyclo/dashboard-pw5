local config = {
    -- IP of the PC running claudedash.exe, as shown on the Overview page of its window.
    host = "192.168.1.10",
    port = 8377,
    interval_seconds = 300,

    -- The three values below are set from the Kindle page of the PC app, which
    -- overrides them. They only apply until the PC has been reached.
    media_interval_seconds = 5,
    -- Minutes between whole-screen flashing refreshes that clear e-ink ghosting. 0 turns them off.
    full_refresh_minutes = 0,
    -- How strongly the two panels lighten the background picture behind the text:
    -- 0 leaves the picture untouched, 1 makes the panels solid white.
    panel_lighten = 0.7,
}

-- cd_config_local.lua is optional and not kept in git: a table of the same
-- shape whose fields replace the ones above, so your own address stays private.
local ok, own = pcall(require, "cd_config_local")
if ok and type(own) == "table" then
    for key, value in pairs(own) do
        config[key] = value
    end
end

return config
