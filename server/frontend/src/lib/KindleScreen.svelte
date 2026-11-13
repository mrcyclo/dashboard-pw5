<script>
  import { onMount } from 'svelte'
  import { RotateCw, X } from '@lucide/svelte'
  import { app, localUrl } from './store.svelte.js'
  import { focusFromDrag, zoomFromWheel } from './format.js'
  import { gridLines, SCREEN, snap } from './grid.js'
  import WidgetTree from './WidgetTree.svelte'

  /*
  The Kindle screen drawn from the same widget trees the Kindle receives.
  mode "background": dragging moves the background picture, the wheel zooms it.
  mode "layout": the grid shows and dragging a widget moves it.
  */
  let {
    width = 252,
    mode = 'background',
    focus,
    zoom,
    lighten,
    onfocus,
    onzoom,
    selected = null,
    onselect,
    onmove,
  } = $props()

  const lines = gridLines()

  let screenEl
  let picture = $state(null)
  let pictureReady = $state(false)
  let drag = $state(null)
  let fontsReady = $state(false)

  let scale = $derived(width / SCREEN.width)
  let background = $derived(app.state.background)
  let screen = $derived(app.state.widgets?.screen)
  let widgets = $derived(screen?.widgets ?? [])
  let battery = $derived(app.state.kindle?.battery)
  let shown = $derived(drag?.type === 'background' ? drag.current : focus)

  // Text is measured with the font the Kindle uses, once it has loaded.
  const canvas = typeof document !== 'undefined' ? document.createElement('canvas').getContext('2d') : null
  let measure = $derived.by(() => {
    fontsReady
    const cache = new Map()
    return (text, size, bold) => {
      const key = `${bold ? 1 : 0}|${size}|${text}`
      if (!cache.has(key)) {
        canvas.font = `${bold ? 700 : 400} ${size}px 'Noto Sans'`
        cache.set(key, Math.ceil(canvas.measureText(text).width))
      }
      return cache.get(key)
    }
  })

  onMount(() => {
    Promise.all([document.fonts.load("400 20px 'Noto Sans'"), document.fonts.load("700 20px 'Noto Sans'")]).then(() => {
      fontsReady = true
    })
  })

  function startBackground(event) {
    if (mode !== 'background' || !background.set || !pictureReady) return
    screenEl.setPointerCapture(event.pointerId)
    drag = { type: 'background', x: event.clientX, y: event.clientY, from: { ...focus }, current: { x: focus.x, y: focus.y } }
  }

  function startWidget(event, placed) {
    if (mode !== 'layout') return
    event.stopPropagation()
    screenEl.setPointerCapture(event.pointerId)
    drag = { type: 'widget', kind: placed.kind, rect: placed.rect, x: event.clientX, y: event.clientY, dx: 0, dy: 0 }
  }

  function move(event) {
    if (!drag) return
    if (drag.type === 'widget') {
      drag.dx = (event.clientX - drag.x) / scale
      drag.dy = (event.clientY - drag.y) / scale
      return
    }
    drag.current = focusFromDrag(
      drag.from,
      event.clientX - drag.x,
      event.clientY - drag.y,
      { w: picture.naturalWidth, h: picture.naturalHeight },
      { w: screenEl.clientWidth, h: screenEl.clientHeight },
      zoom,
    )
  }

  function end() {
    if (!drag) return
    const done = drag
    drag = null
    if (done.type === 'widget') {
      const moved = Math.abs(done.dx) > 8 || Math.abs(done.dy) > 8
      if (!moved) {
        onselect?.(done.kind)
        return
      }
      const cell = snap(done.rect.x + done.dx, done.rect.y + done.dy)
      onmove?.(done.kind, cell.col, cell.row)
      return
    }
    if (done.current.x !== done.from.x || done.current.y !== done.from.y) {
      onfocus?.(done.current)
    }
  }

  function wheel(event) {
    if (mode !== 'background' || !background.set || !pictureReady || drag) return
    event.preventDefault()
    const next = zoomFromWheel(zoom, event.deltaY)
    if (next !== zoom) {
      onzoom?.(next)
    }
  }

  const box = (r) => `left:${r.x}px;top:${r.y}px;width:${r.w}px;height:${r.h}px`
  const offset = (placed) =>
    drag?.type === 'widget' && drag.kind === placed.kind ? `transform:translate(${drag.dx}px,${drag.dy}px)` : ''
</script>

<!-- The bezel is a separate element so the screen inside keeps the Kindle's exact 3:4 shape. -->
<div class="shrink-0 rounded-md bg-[#37475a] p-[6px]">
  <div
    bind:this={screenEl}
    class="relative overflow-hidden bg-white select-none {mode === 'background' && background.set ? 'cursor-grab active:cursor-grabbing' : ''}"
    style="width:{width}px;height:{(width * SCREEN.height) / SCREEN.width}px"
    role="img"
    aria-label="Kindle screen preview"
    onpointerdown={startBackground}
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
      class="absolute top-0 left-0 origin-top-left font-['Noto_Sans'] leading-none"
      style="width:{SCREEN.width}px;height:{SCREEN.height}px;transform:scale({scale})"
    >
      {#if mode === 'layout'}
        <svg class="pointer-events-none absolute inset-0" width={SCREEN.width} height={SCREEN.height}>
          {#each lines.xs as x (x)}
            <line x1={x} y1={lines.top} x2={x} y2={lines.bottom} stroke="rgba(0,0,0,0.22)" stroke-width="3" />
          {/each}
          {#each lines.ys as y (y)}
            <line x1={lines.left} y1={y} x2={lines.right} y2={y} stroke="rgba(0,0,0,0.22)" stroke-width="3" />
          {/each}
        </svg>
      {/if}

      {#each widgets as placed (placed.kind)}
        <div class="absolute inset-0 {mode === 'layout' ? '' : 'pointer-events-none'}" style={offset(placed)}>
          <div
            class="absolute {mode === 'layout' ? 'cursor-move' : ''} {selected === placed.kind ? 'outline-[10px] outline-[#ff9900] outline-solid' : ''}"
            style="{box(placed.rect)};{background.set ? `background: rgba(255, 255, 255, ${lighten})` : ''}"
            role="button"
            tabindex="-1"
            aria-label={placed.kind}
            onpointerdown={(event) => startWidget(event, placed)}
          ></div>
          <div class="pointer-events-none">
            <WidgetTree {placed} icons={screen.icons} now={app.now} {measure} />
          </div>
        </div>
      {/each}

      <div class="pointer-events-none absolute top-0 right-0 flex h-[150px] items-center text-[35px] text-[#111]">
        {#if battery !== undefined && battery !== null}<span class="pr-[40px]">{battery}%</span>{/if}
        <span class="grid w-[150px] place-items-center"><RotateCw size={45} strokeWidth={2.5} /></span>
        <span class="grid w-[150px] place-items-center"><X size={50} strokeWidth={2.5} /></span>
      </div>
    </div>
  </div>
</div>
