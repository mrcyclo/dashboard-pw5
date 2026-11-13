<script>
  import { Music, Pause, Play, RotateCw, SkipBack, SkipForward, Volume1, Volume2, X } from '@lucide/svelte'
  import { app, localUrl } from './store.svelte.js'
  import { focusFromDrag, mediaHeading, timeText, usageSections, zoomFromWheel } from './format.js'
  import { KINDLE, kindleZones } from './kindleLayout.js'

  // focus, zoom and lighten come from the page so the preview follows a slider
  // or a drag at once; onfocus is called when a drag ends, onzoom on every wheel notch.
  let { focus, zoom, lighten, onfocus, onzoom } = $props()

  const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
  const PREVIEW_WIDTH = 252
  const zones = kindleZones()
  const box = (z) => `left:${z.x}px;top:${z.y}px;width:${z.w}px;height:${z.h}px`

  let screen
  let picture = $state(null)
  // False until the picture has loaded, and again if it fails: there is nothing to drag then.
  let pictureReady = $state(false)
  let drag = $state(null)
  // While dragging, the picture follows the pointer; the page only hears about it on release.
  let shown = $derived(drag ? drag.current : focus)

  let background = $derived(app.state.background)
  let draggable = $derived(background.set && pictureReady)
  let sections = $derived(usageSections(app.state.usage, app.now))
  let media = $derived(app.state.media)
  let frost = $derived(background.set ? `background: rgba(255, 255, 255, ${lighten})` : '')
  let today = $derived.by(() => {
    const date = new Date(app.now * 1000)
    const pad = (n) => String(n).padStart(2, '0')
    return `${WEEKDAYS[date.getDay()]}, ${pad(date.getDate())}/${pad(date.getMonth() + 1)}/${date.getFullYear()}`
  })
  let buttons = $derived([
    { zone: zones.previous, icon: SkipBack },
    { zone: zones.play_pause, icon: media.active && media.playing ? Pause : Play },
    { zone: zones.next, icon: SkipForward },
  ])

  function start(event) {
    if (!draggable) return
    screen.setPointerCapture(event.pointerId)
    drag = { x: event.clientX, y: event.clientY, from: { x: focus.x, y: focus.y }, current: { x: focus.x, y: focus.y } }
  }

  function move(event) {
    if (!drag) return
    drag.current = focusFromDrag(
      drag.from,
      event.clientX - drag.x,
      event.clientY - drag.y,
      { w: picture.naturalWidth, h: picture.naturalHeight },
      // The screen element has no border, so this is exactly the area the picture covers.
      { w: screen.clientWidth, h: screen.clientHeight },
      zoom,
    )
  }

  function wheel(event) {
    if (!draggable || drag) return
    event.preventDefault()
    const next = zoomFromWheel(zoom, event.deltaY)
    if (next !== zoom) {
      onzoom(next)
    }
  }

  function end() {
    if (!drag) return
    const result = drag.current
    const moved = result.x !== drag.from.x || result.y !== drag.from.y
    drag = null
    if (moved) {
      onfocus(result)
    }
  }
</script>

{#snippet heading(title, note)}
  <div class="flex items-baseline justify-between gap-4">
    <span class="truncate text-[31px] font-bold">{title}</span>
    {#if note}<span class="shrink-0 text-[27px]">{note}</span>{/if}
  </div>
  <div class="mt-1 mb-2.5 h-0.5 bg-[#555]"></div>
{/snippet}

{#snippet bar(percent, height)}
  <div class="overflow-hidden rounded-[5px] border-2 border-[#1b1b1a] bg-white" style="height:{height}px">
    <div class="h-full bg-[#777]" style="width:{percent}%"></div>
  </div>
{/snippet}

<!-- The bezel is a separate element so the screen inside keeps the Kindle's exact 3:4 shape. -->
<div class="shrink-0 rounded-md bg-[#37475a] p-[6px]">
  <div
    bind:this={screen}
    class="relative aspect-[3/4] overflow-hidden bg-white {draggable ? 'cursor-grab active:cursor-grabbing' : ''}"
    style="width:{PREVIEW_WIDTH}px"
    role="img"
    aria-label="Kindle screen preview"
    onpointerdown={start}
    onpointermove={move}
    onpointerup={end}
    onpointercancel={end}
    onwheel={wheel}
  >
    {#if background.set}
      {#key background.source_hash}
        <img
          bind:this={picture}
          class="pointer-events-none absolute inset-0 size-full object-cover"
          style="object-position: {shown.x * 100}% {shown.y * 100}%; transform-origin: {shown.x * 100}% {shown.y * 100}%; transform: scale({zoom})"
          src={localUrl(`/background-source.png?v=${background.source_hash}`)}
          alt=""
          draggable="false"
          onload={() => (pictureReady = true)}
          onerror={() => (pictureReady = false)}
        />
      {/key}
    {/if}

    <!-- Drawn at the Kindle's real pixel size with the plugin's own coordinates
         (kindleLayout.js mirrors cd_layout.lua, the sizes mirror cd_view.lua), then scaled down. -->
    <div
      class="pointer-events-none absolute top-0 left-0 origin-top-left leading-[1.3] text-[#1b1b1a] select-none"
      style="width:{KINDLE.width}px;height:{KINDLE.height}px;transform:scale({PREVIEW_WIDTH / KINDLE.width});font-family:'Noto Sans','Segoe UI',sans-serif"
    >
      <div class="absolute" style="left:{zones.top_panel.x}px;top:{zones.top_panel.y}px;width:{zones.top_panel.w}px;padding:{zones.padding}px;{frost}">
        <div class="text-[91px] leading-[1.25] font-bold">{timeText(app.now)}</div>
        <div class="text-[35px]">{today}</div>
        <div class="h-4"></div>
        {#each sections as section, index (section.title)}
          {#if index > 0}<div class="h-4"></div>{/if}
          {@render heading(section.title, section.refresh)}
          <div class="grid grid-cols-2 gap-x-7 gap-y-3">
            {#each section.rows as row, i (row.label)}
              <div class="min-w-0 {i === section.rows.length - 1 && i % 2 === 0 ? 'col-span-2' : ''}">
                <div class="flex justify-between gap-4 text-[31px] font-bold">
                  <span class="truncate">{row.label}</span><span class="shrink-0">{row.used}</span>
                </div>
                <div class="mt-1">{@render bar(row.percent, 20)}</div>
                {#if row.refresh}<div class="text-[27px]">{row.refresh}</div>{/if}
              </div>
            {/each}
          </div>
        {:else}
          {@render heading('Claude')}
          <div class="text-[31px]">{app.state.usage.error ?? 'Cannot reach the PC'}</div>
        {/each}
      </div>

      <div class="absolute grid place-items-center" style={box(zones.refresh)}><RotateCw size={45} strokeWidth={2.5} /></div>
      <div class="absolute grid place-items-center" style={box(zones.exit)}><X size={50} strokeWidth={2.5} /></div>

      <div class="absolute" style="{box(zones.media_panel)};{frost}"></div>
      <div class="absolute" style="left:{zones.media_header.x}px;top:{zones.media_header.y}px;width:{zones.media_header.w}px">
        {@render heading(mediaHeading(media))}
      </div>

      {#if media.active && media.art}
        <img class="absolute object-contain" style={box(zones.cover)} src={localUrl(`/media/art.png?v=${media.art}`)} alt="" draggable="false" />
      {:else}
        <div class="absolute grid place-items-center border-2 border-[#1b1b1a] bg-[#c9c9c9]" style={box(zones.cover)}><Music size={52} /></div>
      {/if}

      <div class="absolute flex flex-col justify-center" style={box(zones.track_text)}>
        {#if media.active}
          {#if media.title}<div class="truncate text-[39px] font-bold">{media.title}</div>{/if}
          {#if media.artist}<div class="truncate text-[33px]">{media.artist}</div>{/if}
        {:else}
          <div class="text-[33px]">Nothing playing</div>
        {/if}
      </div>

      {#each buttons as button (button.zone.x)}
        <div class="absolute grid place-items-center" style={box(button.zone)}>
          <div
            class="grid size-[120px] place-items-center rounded-full border-4 bg-white {media.active ? 'border-[#1b1b1a]' : 'border-[#999] text-[#999]'}"
          >
            <button.icon size={50} fill="currentColor" />
          </div>
        </div>
      {/each}

      <div class="absolute grid place-items-center" style={box(zones.volume_down)}><Volume1 size={63} fill="currentColor" /></div>
      <div class="absolute grid place-items-center" style={box(zones.volume_up)}><Volume2 size={63} fill="currentColor" /></div>
      {#if media.volume !== undefined}
        <div class="absolute flex items-center" style={box(zones.volume_bar)}>
          <div class="w-full">{@render bar(media.muted ? 0 : media.volume, 18)}</div>
        </div>
        <div class="absolute grid place-items-center text-[45px] font-bold" style={box(zones.volume_value)}>
          {media.muted ? 'Mute' : media.volume}
        </div>
      {/if}
    </div>
  </div>
</div>
