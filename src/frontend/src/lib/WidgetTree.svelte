<script>
  import { layout } from './tree.js'
  import { localUrl } from './store.svelte.js'

  // placed is one entry of screen.widgets; icons maps icon names to hashes.
  // Coordinates are reference pixels on the 1236 px wide Kindle screen.
  let { placed, icons, now, measure } = $props()

  const PADDING = 24

  let boxes = $derived.by(() => {
    const r = placed.rect
    const inner = { x: r.x + PADDING, y: r.y + PADDING, w: r.w - 2 * PADDING, h: r.h - 2 * PADDING }
    return layout(placed.tree, inner, measure, now)
  })

  const iconUrl = (name) => localUrl(`/icons/${name}.svg?v=${icons?.[name] ?? ''}`)
  const pos = (b) => `left:${b.x}px;top:${b.y}px;width:${b.w}px;height:${b.h}px`
</script>

{#each boxes as b, i (i)}
  {@const node = b.node}
  {#if b.text !== undefined}
    <div
      class="absolute overflow-hidden text-ellipsis whitespace-nowrap {node.dim ? 'text-[#555]' : 'text-[#111]'}"
      style="{pos(b)};font-size:{node.size ?? 20}px;line-height:{b.h}px;font-weight:{node.bold ? 700 : 400}"
    >
      {b.text}
    </div>
  {:else if node.kind === 'bar'}
    <div class="absolute overflow-hidden border-2 border-[#111] bg-white" style={pos(b)}>
      <div class="h-full bg-[#111]" style="width:{Math.min(Math.max(node.percent ?? 0, 0), 100)}%"></div>
    </div>
  {:else if node.kind === 'rule'}
    <div class="absolute bg-[#555]" style={pos(b)}></div>
  {:else if node.kind === 'icon'}
    <img class="absolute object-contain {node.dim ? 'opacity-40' : ''}" style={pos(b)} src={iconUrl(node.name)} alt="" draggable="false" />
  {:else if node.kind === 'image'}
    <img class="absolute object-cover" style={pos(b)} src={localUrl(node.src)} alt="" draggable="false" />
  {:else if node.kind === 'button'}
    <div class="absolute grid place-items-center" style={pos(b)}>
      {#if node.plain}
        <img
          class={node.disabled ? 'opacity-40' : ''}
          style="width:{Math.min(b.w, b.h) * 0.63}px;height:{Math.min(b.w, b.h) * 0.63}px"
          src={iconUrl(node.icon)}
          alt=""
          draggable="false"
        />
      {:else}
        <div
          class="grid place-items-center rounded-full border-4 bg-white {node.disabled ? 'border-[#999]' : 'border-[#111]'}"
          style="width:{Math.min(b.w, b.h)}px;height:{Math.min(b.w, b.h)}px"
        >
          <img
            class={node.disabled ? 'opacity-40' : ''}
            style="width:{Math.min(b.w, b.h) * 0.42}px;height:{Math.min(b.w, b.h) * 0.42}px"
            src={iconUrl(node.icon)}
            alt=""
            draggable="false"
          />
        </div>
      {/if}
    </div>
  {/if}
{/each}
