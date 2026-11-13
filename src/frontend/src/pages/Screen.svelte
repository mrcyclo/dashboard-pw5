<script>
  import { Image, RefreshCw, Trash2 } from '@lucide/svelte'
  import {
    ChooseBackground,
    ClearBackground,
    RefreshKindle,
    SaveDisplay,
    SetBackgroundView,
  } from '../../bindings/claudedash/internal/desk/app'
  import { app, call } from '../lib/store.svelte.js'
  import { MAX_ZOOM, MIN_ZOOM } from '../lib/format.js'
  import KindleScreen from '../lib/KindleScreen.svelte'

  const refreshOptions = [
    { value: 0, label: 'Off' },
    { value: 15, label: '15 min' },
    { value: 30, label: '30 min' },
    { value: 60, label: '60 min' },
  ]
  const mediaOptions = [3, 5, 10, 30]

  let background = $derived(app.state.background)
  let display = $derived(app.state.display)

  // A released drag or a zoom is shown at once and kept until the backend
  // confirms it, so the picture does not jump back to the old crop in between.
  let pendingView = $state(null)
  let view = $derived(pendingView ?? { x: background.focus_x, y: background.focus_y, zoom: background.zoom })
  let zoomTimer

  // The Paperwhite 5 has levels 0 to 24 for both; must match config.MaxLight.
  const MAX_LIGHT = 24

  // Light and warmth follow their sliders while dragged, and save on release.
  let lightDraft = $state({})
  const lights = [
    { key: 'frontlight', title: 'Front light', hint: 'Level 0 turns the light off' },
    { key: 'warmth', title: 'Warmth', hint: 'Higher makes the light more yellow' },
  ]

  function saveDisplay(change) {
    return call(() => SaveDisplay({ ...display, ...change }))
  }

  async function commitView(next) {
    clearTimeout(zoomTimer)
    pendingView = next
    await call(() => SetBackgroundView(next.x, next.y, next.zoom))
    if (pendingView === next) {
      pendingView = null
    }
  }

  function previewZoom(zoom) {
    clearTimeout(zoomTimer)
    pendingView = { ...view, zoom }
  }

  // The wheel sends many small steps; only the place it comes to rest is saved.
  function wheelZoom(zoom) {
    previewZoom(zoom)
    const next = pendingView
    zoomTimer = setTimeout(() => commitView(next), 400)
  }

  async function commitLight(key) {
    const value = lightDraft[key]
    if (value === undefined) return
    await saveDisplay({ [key]: value })
    lightDraft = { ...lightDraft, [key]: undefined }
  }

  // A select keeps showing the rejected choice after a failed save unless it is set back by hand.
  async function changeSelect(event, key) {
    const select = event.currentTarget
    if (await saveDisplay({ [key]: Number(select.value) })) return
    select.value = String(app.state.display[key])
  }
</script>

<header class="mb-5">
  <h1 class="text-2xl font-semibold">Screen</h1>
  <p class="text-sm text-base-content/60">Background, light and refresh. The Kindle picks up changes within a few seconds</p>
</header>

<div class="flex min-h-0 flex-1 gap-5">
  <div class="flex flex-col items-center gap-2">
    <KindleScreen
      focus={view}
      zoom={view.zoom}
      onfocus={(next) => commitView({ ...next, zoom: view.zoom })}
      onzoom={wheelZoom}
    />
    <span class="text-xs text-base-content/50">A simulation, not a screenshot from the Kindle</span>
  </div>

  <div class="cd-scroll -mr-2 flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto pr-2">
    <section>
      <div class="mb-1.5 px-1 text-xs text-base-content/60">Background</div>
      <div class="cd-card overflow-hidden">
        <div class="flex items-center gap-3 px-4 py-3">
          <div class="min-w-0 flex-1">
            <div class="truncate font-semibold">{background.set ? background.name : 'No picture chosen'}</div>
            <div class="text-sm text-base-content/60">
              {background.set ? 'Drag the picture in the frame on the left to choose the visible part' : 'White background. Choose a JPG or PNG picture'}
            </div>
          </div>
          <button class="btn btn-primary btn-sm gap-1.5" onclick={() => call(ChooseBackground)}>
            <Image size={15} />
            {background.set ? 'Change picture' : 'Choose picture'}
          </button>
          {#if background.set}
            <button class="btn btn-ghost btn-sm btn-square" aria-label="Remove background" onclick={() => call(ClearBackground, 'Background removed')}>
              <Trash2 size={15} />
            </button>
          {/if}
        </div>
        {#if background.set}
          <div class="border-t border-white/[.06] px-4 py-3">
            <div class="flex items-baseline justify-between">
              <span class="font-semibold">Zoom</span>
              <span class="text-sm text-base-content/80">{Math.round(view.zoom * 100)}%</span>
            </div>
            <input
              type="range"
              class="range range-primary range-xs mt-2 w-full"
              min={MIN_ZOOM * 100}
              max={MAX_ZOOM * 100}
              step="5"
              value={Math.round(view.zoom * 100)}
              oninput={(e) => previewZoom(Number(e.currentTarget.value) / 100)}
              onchange={(e) => commitView({ ...view, zoom: Number(e.currentTarget.value) / 100 })}
            />
            <div class="mt-1 text-sm text-base-content/60">Zoom in to show only part of the picture. You can also scroll the mouse wheel over the frame on the left</div>
          </div>
        {/if}
      </div>
    </section>

    <section>
      <div class="mb-1.5 px-1 text-xs text-base-content/60">Display</div>
      <div class="cd-card overflow-hidden">
        <label class="flex items-center gap-3 px-4 py-3">
          <div class="flex-1">
            <div class="font-semibold">Periodic full refresh</div>
            <div class="text-sm text-base-content/60">The screen flashes once to clear e-ink ghosting</div>
          </div>
          <select class="select select-sm w-28" value={display.full_refresh_minutes} onchange={(e) => changeSelect(e, 'full_refresh_minutes')}>
            {#each refreshOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
          </select>
        </label>

        <div class="flex items-center gap-3 border-t border-white/[.06] px-4 py-3">
          <div class="flex-1">
            <div class="font-semibold">Refresh screen now</div>
            <div class="text-sm text-base-content/60">The Kindle flashes and redraws everything on its next poll</div>
          </div>
          <button
            class="btn btn-sm gap-1.5"
            onclick={() => call(RefreshKindle, 'Sent. The Kindle will refresh in a few seconds')}
          >
            <RefreshCw size={15} />
            Refresh
          </button>
        </div>

        <label class="flex items-center gap-3 border-t border-white/[.06] px-4 py-3">
          <div class="flex-1">
            <div class="font-semibold">Media polling interval</div>
            <div class="text-sm text-base-content/60">Shorter shows track changes sooner and uses more battery</div>
          </div>
          <select class="select select-sm w-28" value={display.media_interval_seconds} onchange={(e) => changeSelect(e, 'media_interval_seconds')}>
            {#each mediaOptions as s (s)}<option value={s}>{s} s</option>{/each}
          </select>
        </label>
      </div>
    </section>

    <section>
      <div class="mb-1.5 px-1 text-xs text-base-content/60">Light</div>
      <div class="cd-card overflow-hidden">
        {#each lights as light (light.key)}
          {@const value = lightDraft[light.key] ?? display[light.key]}
          <div class="border-t border-white/[.06] px-4 py-3 first:border-t-0">
            <div class="flex items-baseline justify-between">
              <span class="font-semibold">{light.title}</span>
              <span class="text-sm text-base-content/80">{value < 0 ? "Kindle's own" : `Level ${value}/${MAX_LIGHT}`}</span>
            </div>
            <input
              type="range"
              class="range range-primary range-xs mt-2 w-full"
              min="0"
              max={MAX_LIGHT}
              step="1"
              value={Math.max(value, 0)}
              oninput={(e) => (lightDraft = { ...lightDraft, [light.key]: Number(e.currentTarget.value) })}
              onchange={() => commitLight(light.key)}
            />
            <div class="mt-1 text-sm text-base-content/60">
              {value < 0 ? 'Not set from the PC, so the Kindle keeps its current level. Drag to set' : light.hint}
            </div>
          </div>
        {/each}
      </div>
    </section>
  </div>
</div>
