<script>
  import {
    AlignCenter,
    AlignLeft,
    AlignRight,
    AppWindow,
    ChartColumn,
    ChevronDown,
    ChevronUp,
    Clock,
    LayoutGrid,
    Music,
    Plus,
    RefreshCw,
    Trash2,
  } from '@lucide/svelte'
  import {
    AddApp,
    MoveApp,
    MoveWidget,
    RefreshKindle,
    RemoveApp,
    SetWidgetLighten,
    SetWidgetOption,
    SetWidgetSize,
  } from '../../bindings/claudedash/internal/desk/app'
  import { app, call, localUrl } from '../lib/store.svelte.js'
  import { canShow } from '../lib/grid.js'
  import KindleScreen from '../lib/KindleScreen.svelte'

  // Icons named by each widget kind; a kind this window does not know gets a plain one.
  const ICONS = { clock: Clock, 'chart-column': ChartColumn, music: Music, 'app-window': AppWindow }
  // Option choices drawn as an icon; others show their label.
  const CHOICE_ICONS = { left: AlignLeft, center: AlignCenter, right: AlignRight }

  let widgets = $derived(app.state.widgets)
  let background = $derived(app.state.background)
  let view = $derived({ x: background.focus_x, y: background.focus_y, zoom: background.zoom })
  let selected = $state(null)
  // Transparency sliders move the preview while dragged, and save on release.
  let lightenDrafts = $state({})

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

  const transparency = (lighten) => 100 - Math.round(lighten * 100)
  const lightenOf = (kind) => lightenDrafts[kind] ?? widgets.lighten?.[kind] ?? 0.7

  async function commitLighten(kind) {
    const value = lightenDrafts[kind]
    if (value === undefined) return
    await call(() => SetWidgetLighten(kind, value))
    const { [kind]: _, ...rest } = lightenDrafts
    lightenDrafts = rest
  }

  function move(kind, col, row) {
    selected = kind
    const current = placementOf(kind)
    if (current && current.col === col && current.row === row) return
    call(() => MoveWidget(kind, col, row))
  }
</script>

<header class="mb-5">
  <div class="flex items-center gap-3">
    <h1 class="text-2xl font-semibold">Layout</h1>
    <button
      class="btn btn-sm ml-auto gap-1.5"
      title="The Kindle flashes and redraws everything on its next poll"
      onclick={() => call(RefreshKindle, 'Sent. The Kindle will refresh in a few seconds')}
    >
      <RefreshCw size={15} />
      Refresh Kindle
    </button>
  </div>
  <p class="text-sm text-base-content/60">Which widgets the Kindle shows, how big, and where. Drag a widget to move it</p>
</header>

<div class="flex min-h-0 flex-1 gap-5">
  <div class="flex flex-col items-center gap-2">
    <KindleScreen
      width={330}
      mode="layout"
      focus={view}
      zoom={view.zoom}
      {lightenDrafts}
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
            <div class="flex items-center gap-3">
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
                <div class="join ml-auto" role="group" aria-label={option.label}>
                  {#each option.choices as choice (choice.value)}
                    {@const ChoiceIcon = CHOICE_ICONS[choice.value]}
                    <button
                      class="btn join-item btn-sm {ChoiceIcon ? 'btn-square' : 'min-w-[64px]'} {value === choice.value
                        ? 'border-[#ff9900] bg-[rgba(255,153,0,.16)] text-[#febd69]'
                        : 'btn-ghost border-white/[.12]'}"
                      title="{option.label}: {choice.label}"
                      aria-label="{option.label}: {choice.label}"
                      onclick={(event) => {
                        event.stopPropagation()
                        chooseOption(kind.kind, option.key, choice.value)
                      }}
                    >
                      {#if ChoiceIcon}<ChoiceIcon size={16} />{:else}{choice.label}{/if}
                    </button>
                  {/each}
                </div>
              {/each}
            </div>
            {#if kind.kind === 'apps'}
              <div class="mt-2.5 flex flex-col gap-1" role="presentation" onclick={(event) => event.stopPropagation()}>
                {#each widgets.apps ?? [] as item, index (item.id)}
                  <div class="flex items-center gap-2.5 rounded-[8px] bg-white/[.03] px-2 py-1.5">
                    {#if item.icon}
                      <img class="size-[24px] rounded-[4px] bg-white" src={localUrl(`/apps/${item.id}.png?v=${item.icon}`)} alt="" />
                    {:else}
                      <span class="grid size-[24px] place-items-center text-base-content/60"><AppWindow size={16} /></span>
                    {/if}
                    <span class="min-w-0 flex-1 truncate text-sm">{item.name}</span>
                    <button class="btn btn-ghost btn-xs btn-square" aria-label="Move {item.name} up" disabled={index === 0} onclick={() => call(() => MoveApp(item.id, -1))}>
                      <ChevronUp size={14} />
                    </button>
                    <button
                      class="btn btn-ghost btn-xs btn-square"
                      aria-label="Move {item.name} down"
                      disabled={index === widgets.apps.length - 1}
                      onclick={() => call(() => MoveApp(item.id, 1))}
                    >
                      <ChevronDown size={14} />
                    </button>
                    <button class="btn btn-ghost btn-xs btn-square" aria-label="Remove {item.name}" onclick={() => call(() => RemoveApp(item.id))}>
                      <Trash2 size={14} />
                    </button>
                  </div>
                {/each}
                <div class="flex items-center gap-3">
                  <button class="btn btn-sm gap-1.5" onclick={() => call(AddApp)}>
                    <Plus size={15} />
                    Add app
                  </button>
                  <span class="text-sm text-base-content/60">A tap on the Kindle opens the program, or brings its window to the front</span>
                </div>
              </div>
            {/if}
            {#if current !== 'none'}
              <div class="mt-2.5 flex items-center gap-3" role="presentation" onclick={(event) => event.stopPropagation()}>
                <span class="w-[150px] shrink-0 text-sm text-base-content/70">Panel transparency</span>
                <input
                  type="range"
                  class="range range-primary range-xs flex-1"
                  min="0"
                  max="100"
                  step="5"
                  value={transparency(lightenOf(kind.kind))}
                  disabled={!background.set}
                  title={background.set ? 'Higher shows more of the picture and makes text harder to read' : 'Only applies once a background is chosen on the Screen page'}
                  oninput={(e) => (lightenDrafts = { ...lightenDrafts, [kind.kind]: (100 - Number(e.currentTarget.value)) / 100 })}
                  onchange={() => commitLighten(kind.kind)}
                />
                <span class="w-[40px] text-right text-sm text-base-content/80">{transparency(lightenOf(kind.kind))}%</span>
              </div>
            {/if}
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
