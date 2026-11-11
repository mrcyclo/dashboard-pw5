<script>
  import { Music, RefreshCw, X } from '@lucide/svelte'
  import { app, localUrl } from './store.svelte.js'
  import { focusFromDrag, mediaHeading, timeText, usageSections, zoomFromWheel } from './format.js'

  // focus, zoom and lighten come from the page so the preview follows a slider
  // or a drag at once; onfocus is called when a drag ends, onzoom on every wheel notch.
  let { focus, zoom, lighten, onfocus, onzoom } = $props()

  const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

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
  let today = $derived.by(() => {
    const date = new Date(app.now * 1000)
    const pad = (n) => String(n).padStart(2, '0')
    return `${WEEKDAYS[date.getDay()]}, ${pad(date.getDate())}/${pad(date.getMonth() + 1)}/${date.getFullYear()}`
  })

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

<!-- The bezel is a separate element so the screen inside keeps the Kindle's exact 3:4 shape. -->
<div class="shrink-0 rounded-md bg-[#37475a] p-[6px]">
  <!-- Proportions follow claudedash.koplugin/cd_layout.lua for a 1236 x 1648 screen. -->
  <div
    bind:this={screen}
    class="relative aspect-[3/4] w-[252px] overflow-hidden bg-[#eceae3] text-[#1b1b1a] {draggable ? 'cursor-grab active:cursor-grabbing' : ''}"
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

    <div
      class="absolute inset-x-[3.2%] top-[2.4%] px-[5%] py-[4%]"
      style={background.set ? `background: rgba(255, 255, 255, ${lighten})` : ''}
    >
      <div class="flex items-start justify-between">
        <span class="text-[25px] leading-none font-bold">{timeText(app.now)}</span>
        <span class="flex items-center gap-1.5"><RefreshCw size={10} /> <X size={11} /></span>
      </div>
      <div class="mt-0.5 text-[9px]">{today}</div>

      {#each sections as section (section.title)}
        <div class="mt-1.5 flex items-baseline justify-between border-b border-[#5f5e5a] pb-0.5 text-[8px]">
          <span class="font-bold">{section.title}</span>
          {#if section.refresh}<span>{section.refresh}</span>{/if}
        </div>
        <div class="mt-1 grid grid-cols-2 gap-x-2 gap-y-1">
          {#each section.rows as row (row.label)}
            <div class={section.rows.length === 1 ? 'col-span-2' : ''}>
              <div class="flex justify-between gap-1 text-[7.5px] font-bold">
                <span class="truncate">{row.label}</span><span class="shrink-0">{row.used}</span>
              </div>
              <div class="mt-0.5 h-[5px] border border-[#1b1b1a] bg-white">
                <div class="h-full bg-[#5f5e5a]" style="width: {row.percent}%"></div>
              </div>
              {#if row.refresh}<div class="text-[7px]">{row.refresh}</div>{/if}
            </div>
          {/each}
        </div>
      {:else}
        <div class="mt-1.5 text-[8px]">{app.state.usage.error ?? 'Cannot reach the PC'}</div>
      {/each}
    </div>

    <div
      class="absolute inset-x-[3.2%] bottom-[2.4%] px-[5%] py-[3.5%]"
      style={background.set ? `background: rgba(255, 255, 255, ${lighten})` : ''}
    >
      <div class="border-b border-[#5f5e5a] pb-0.5 text-[8px] font-bold">{mediaHeading(media)}</div>
      <div class="mt-1.5 flex items-center gap-1.5">
        {#if media.active && media.art}
          <img class="size-[26px] shrink-0 object-cover" src={localUrl(`/media/art.png?v=${media.art}`)} alt="" draggable="false" />
        {:else}
          <div class="grid size-[26px] shrink-0 place-items-center border border-[#1b1b1a] bg-[#c9c7bf]"><Music size={11} /></div>
        {/if}
        <div class="min-w-0 flex-1">
          {#if media.active}
            {#if media.title}<div class="truncate text-[10px] font-bold">{media.title}</div>{/if}
            {#if media.artist}<div class="truncate text-[8px]">{media.artist}</div>{/if}
          {:else}
            <div class="text-[8px]">Nothing playing</div>
          {/if}
        </div>
        {#each [0, 1, 2] as i (i)}
          <div class="size-[24px] shrink-0 rounded-full border-[1.5px] border-[#1b1b1a] bg-white {media.active ? '' : 'opacity-40'}"></div>
        {/each}
      </div>
      <div class="mt-2 flex items-center gap-2">
        <div class="h-[5px] flex-1 border border-[#1b1b1a] bg-white">
          <div class="h-full bg-[#5f5e5a]" style="width: {media.muted ? 0 : (media.volume ?? 0)}%"></div>
        </div>
        {#if media.volume !== undefined}
          <span class="min-w-5 text-right text-[10px] font-bold">{media.muted ? 'Mute' : media.volume}</span>
        {/if}
      </div>
    </div>
  </div>
</div>
