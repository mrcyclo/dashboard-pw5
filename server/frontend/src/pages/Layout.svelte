<script>
  import { ChartColumn, Clock, LayoutGrid, Music } from '@lucide/svelte'
  import { MoveWidget, SetWidgetOption, SetWidgetSize } from '../../bindings/claudedash/internal/desk/app'
  import { app, call } from '../lib/store.svelte.js'
  import { canShow } from '../lib/grid.js'
  import KindleScreen from '../lib/KindleScreen.svelte'

  // Icons named by each widget kind; a kind this window does not know gets a plain one.
  const ICONS = { clock: Clock, 'chart-column': ChartColumn, music: Music }

  let widgets = $derived(app.state.widgets)
  let background = $derived(app.state.background)
  let view = $derived({ x: background.focus_x, y: background.focus_y, zoom: background.zoom })
  let selected = $state(null)

  const sizeLabel = (size) => (size === 'none' ? 'None' : size.replace('x', '×'))
  const placementOf = (kind) => widgets.layout.find((p) => p.kind === kind)

  function choose(kind, size) {
    selected = kind
    if (placementOf(kind)?.size === size) return
    call(() => SetWidgetSize(kind, size))
  }

  function chooseOption(kind, key, value) {
    selected = kind
    if (widgets.options?.[kind]?.[key] === value) return
    call(() => SetWidgetOption(kind, key, value))
  }

  function move(kind, col, row) {
    selected = kind
    const current = placementOf(kind)
    if (current && current.col === col && current.row === row) return
    call(() => MoveWidget(kind, col, row))
  }
</script>

<header class="mb-5">
  <h1 class="text-2xl font-semibold">Layout</h1>
  <p class="text-sm text-base-content/60">Which widgets the Kindle shows, how big, and where. Drag a widget to move it</p>
</header>

<div class="flex min-h-0 flex-1 gap-5">
  <div class="flex flex-col items-center gap-2">
    <KindleScreen
      width={330}
      mode="layout"
      focus={view}
      zoom={view.zoom}
      lighten={app.state.display.panel_lighten}
      {selected}
      onselect={(kind) => (selected = kind)}
      onmove={move}
    />
  </div>

  <div class="cd-scroll -mr-2 flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto pr-2">
    <section>
      <div class="mb-1.5 px-1 text-xs text-base-content/60">Widgets</div>
      <div class="cd-card overflow-hidden">
        {#each widgets.catalog as kind (kind.kind)}
          {@const Icon = ICONS[kind.icon] ?? LayoutGrid}
          {@const current = placementOf(kind.kind)?.size ?? 'none'}
          <div
            class="border-t border-white/[.06] px-4 py-3 first:border-t-0 {selected === kind.kind ? 'bg-[rgba(255,153,0,.07)]' : ''}"
            role="presentation"
            onclick={() => (selected = kind.kind)}
          >
            <div class="mb-2 flex items-center gap-2.5 font-semibold">
              <span class="grid size-[28px] place-items-center rounded-[8px] bg-[rgba(255,153,0,.14)] text-[#febd69]">
                <Icon size={16} />
              </span>
              {kind.label}
            </div>
            <div class="join">
              {#each ['none', ...kind.sizes] as size (size)}
                {@const check = canShow(widgets.layout, kind.kind, size)}
                <button
                  class="btn join-item btn-sm min-w-[64px] {current === size
                    ? size === 'none'
                      ? 'btn-neutral'
                      : 'border-[#ff9900] bg-[rgba(255,153,0,.16)] text-[#febd69]'
                    : 'btn-ghost border-white/[.12]'}"
                  disabled={!check.ok && current !== size}
                  title={check.ok ? '' : check.reason}
                  onclick={(event) => {
                    event.stopPropagation()
                    choose(kind.kind, size)
                  }}
                >
                  {sizeLabel(size)}
                </button>
              {/each}
            </div>
            {#each kind.options ?? [] as option (option.key)}
              {@const value = widgets.options?.[kind.kind]?.[option.key] ?? option.default}
              <div class="mt-2.5 flex items-center gap-3">
                <span class="w-[72px] text-sm text-base-content/60">{option.label}</span>
                <div class="join">
                  {#each option.choices as choice (choice.value)}
                    <button
                      class="btn join-item btn-sm min-w-[64px] {value === choice.value
                        ? 'border-[#ff9900] bg-[rgba(255,153,0,.16)] text-[#febd69]'
                        : 'btn-ghost border-white/[.12]'}"
                      onclick={(event) => {
                        event.stopPropagation()
                        chooseOption(kind.kind, option.key, choice.value)
                      }}
                    >
                      {choice.label}
                    </button>
                  {/each}
                </div>
              </div>
            {/each}
          </div>
        {/each}
      </div>
    </section>
    <p class="px-1 text-sm text-base-content/60">
      Sizes are columns × rows of the Kindle's 6 × 8 grid. Widgets never overlap; the top right corner is kept for the battery, refresh and exit
      buttons.
    </p>
  </div>
</div>
