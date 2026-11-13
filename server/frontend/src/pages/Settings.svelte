<script>
  import { FolderOpen, PanelBottomClose, Power } from '@lucide/svelte'
  import { OpenDataFolder, SaveSettings } from '../../bindings/claudedash/internal/desk/app'
  import { app, call } from '../lib/store.svelte.js'

  const MIN_PORT = 1024
  const MAX_PORT = 65535
  const intervals = [
    { value: 300, label: '5 min' },
    { value: 600, label: '10 min' },
    { value: 900, label: '15 min' },
    { value: 1800, label: '30 min' },
  ]

  let saved = $derived(app.state.settings)
  let listening = $derived(app.state.listening)
  // The port is typed, so it is kept as a draft until Save is pressed.
  let port = $state(String(app.state.settings.port))
  let saving = $state(false)
  // Digits only: Number() alone would also accept "0x2000", "1e4" or padded text.
  let portNumber = $derived(/^\d+$/.test(port) ? Number(port) : NaN)
  let portValid = $derived(portNumber >= MIN_PORT && portNumber <= MAX_PORT)
  let portChanged = $derived(portNumber !== saved.port)

  function save(change, message) {
    return call(() => SaveSettings({ ...saved, ...change }), message)
  }

  async function savePort() {
    if (!portValid || saving) return
    saving = true
    const ok = await save({ port: portNumber }, `Listening on port ${portNumber}`)
    saving = false
    if (!ok) {
      port = String(app.state.settings.port)
    }
  }

  async function toggle(event, key) {
    const input = event.currentTarget
    if (await save({ [key]: input.checked })) return
    input.checked = app.state.settings[key]
  }

  // A select keeps showing the rejected choice after a failed save unless it is set back by hand.
  async function changeInterval(event) {
    const select = event.currentTarget
    if (await save({ usage_interval_seconds: Number(select.value) })) return
    select.value = String(app.state.settings.usage_interval_seconds)
  }

  const toggles = [
    {
      key: 'autostart',
      icon: Power,
      title: 'Start with Windows',
      description: 'Runs at sign-in and shows only a tray icon.',
    },
    {
      key: 'close_to_tray',
      icon: PanelBottomClose,
      title: 'Minimize to tray on close',
      description: 'Closing the window keeps the app running for the Kindle. Turn this off to quit when the window closes.',
    },
  ]
</script>

<header class="mb-5">
  <h1 class="text-2xl font-semibold">Settings</h1>
  <p class="text-sm text-base-content/60">How Claude Dashboard runs on this computer</p>
</header>

<div class="cd-scroll -mr-2 flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto pr-2">
  <section>
    <div class="mb-1.5 px-1 text-xs text-base-content/60">Connection</div>
    <div class="cd-card overflow-hidden">
      <div class="flex items-center gap-3 px-4 py-3">
        <div class="flex-1">
          <div class="font-semibold">Port</div>
          {#if !portValid}
            <div class="text-sm text-[#ff8278]">Port must be a number from {MIN_PORT} to {MAX_PORT}</div>
          {:else if !listening}
            <div class="text-sm text-[#ff8278]">
              Cannot open port {saved.port}, so the Kindle cannot reach this PC. Close the program using this port and press Retry, or choose another port
            </div>
          {:else}
            <div class="text-sm text-base-content/60">
              The Kindle finds this port on its own
            </div>
          {/if}
        </div>
        <input type="text" inputmode="numeric" class="input input-sm w-24 text-right font-mono" bind:value={port} />
        <button class="btn btn-primary btn-sm" disabled={saving || !portValid || (!portChanged && listening)} onclick={savePort}>
          {!portChanged && !listening ? 'Retry' : 'Save'}
        </button>
      </div>

      <label class="flex items-center gap-3 border-t border-white/[.06] px-4 py-3">
        <div class="flex-1">
          <div class="font-semibold">Usage polling interval</div>
          <div class="text-sm text-base-content/60">No shorter than 5 minutes, because Claude rate-limits usage requests</div>
        </div>
        <select class="select select-sm w-28" value={saved.usage_interval_seconds} onchange={changeInterval}>
          {#each intervals as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
        </select>
      </label>
    </div>
  </section>

  <section>
    <div class="mb-1.5 px-1 text-xs text-base-content/60">Application</div>
    <div class="cd-card overflow-hidden">
      {#each toggles as t (t.key)}
        <label class="flex cursor-pointer items-center gap-3.5 border-t border-white/[.06] px-4 py-3 first:border-t-0">
          <div class="grid size-[34px] shrink-0 place-items-center rounded-[9px] bg-[rgba(255,153,0,.14)] text-[#febd69]">
            <t.icon size={18} />
          </div>
          <div class="flex-1">
            <div class="font-semibold">{t.title}</div>
            <div class="text-sm text-base-content/60">{t.description}</div>
          </div>
          <input type="checkbox" class="toggle toggle-primary" checked={saved[t.key]} onchange={(e) => toggle(e, t.key)} />
        </label>
      {/each}
      <div class="flex items-center gap-3.5 border-t border-white/[.06] px-4 py-3">
        <div class="grid size-[34px] shrink-0 place-items-center rounded-[9px] bg-[rgba(255,153,0,.14)] text-[#febd69]">
          <FolderOpen size={18} />
        </div>
        <div class="flex-1">
          <div class="font-semibold">Data folder</div>
          <div class="text-sm text-base-content/60">Where settings, logs and saved usage data are kept</div>
        </div>
        <button class="btn btn-ghost btn-sm" onclick={() => call(OpenDataFolder)}>Open</button>
      </div>
    </div>
  </section>
</div>
