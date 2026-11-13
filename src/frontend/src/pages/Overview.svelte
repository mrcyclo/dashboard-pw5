<script>
  import { Copy, Music, RefreshCw } from '@lucide/svelte'
  import { RefreshUsage } from '../../bindings/claudedash/internal/desk/app'
  import { app, call, localUrl, toast } from '../lib/store.svelte.js'
  import { kindleStatus, mediaHeading, mediaLine, timeText, tokenLabel, usageSections } from '../lib/format.js'
  import UsageSections from '../lib/UsageSections.svelte'

  let refreshing = $state(false)

  let usage = $derived(app.state.usage)
  let sections = $derived(usageSections(usage, app.now))
  let token = $derived(tokenLabel(app.state.token))
  let media = $derived(app.state.media)
  let kindle = $derived(kindleStatus(app.state.kindle, app.now))
  let mediaTitle = $derived(mediaHeading(media))
  let mediaDetail = $derived(mediaLine(media))

  const dots = { ok: 'bg-[#3fb68b]', warn: 'bg-[#ffd814]', error: 'bg-[#ff5b4f]', idle: 'bg-white/30' }

  async function refreshUsage() {
    refreshing = true
    await call(RefreshUsage)
    refreshing = false
  }

  async function copyAddress() {
    try {
      await navigator.clipboard.writeText(app.state.address)
      toast('success', 'Address copied')
    } catch {
      toast('error', 'Could not copy the address')
    }
  }
</script>

<header class="mb-5">
  <h1 class="text-2xl font-semibold">Overview</h1>
  <p class="text-sm text-base-content/60">What the Kindle is showing, and the connection status</p>
</header>

<div class="grid min-h-0 flex-1 grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)] grid-rows-[auto_auto_1fr] gap-4">
  <section class="cd-card row-span-2 p-4">
    <div class="mb-3 flex items-center justify-between">
      <span class="text-xs text-base-content/60">Claude</span>
      <button class="btn btn-ghost btn-xs gap-1.5" onclick={refreshUsage} disabled={refreshing}>
        <RefreshCw size={13} class={refreshing ? 'animate-spin' : ''} />
        Refresh
      </button>
    </div>

    {#if sections.length > 0}
      <UsageSections {sections} />
    {:else}
      <div class="py-2 text-sm text-base-content/70">{usage.error ?? 'No data yet'}</div>
    {/if}

    {#if usage.fetched_at}
      <div class="mt-3 text-xs text-base-content/60">
        Updated at {timeText(usage.fetched_at)}{#if usage.stale}
          <span class="text-[#ffd814]"> · stale data{#if usage.error}: {usage.error}{/if}</span>
        {/if}
      </div>
    {/if}

    <div class="mt-4 flex items-center gap-2 border-t border-white/[.07] pt-3 text-sm">
      <span class="size-2 shrink-0 rounded-full {dots[token.tone]}"></span>
      <span>{token.text}</span>
    </div>
  </section>

  <section class="cd-card p-4">
    <div class="mb-3 text-xs text-base-content/60">
      {mediaTitle}
    </div>
    {#if media.active}
      <div class="flex items-center gap-3">
        {#if media.art}
          <img class="size-12 shrink-0 rounded-md object-cover" src={localUrl(`/media/art.png?v=${media.art}`)} alt="" />
        {:else}
          <div class="grid size-12 shrink-0 place-items-center rounded-md bg-white/[.07] text-base-content/60">
            <Music size={20} />
          </div>
        {/if}
        <div class="min-w-0">
          {#if media.title}<div class="truncate font-semibold">{media.title}</div>{/if}
          {#if mediaDetail}<div class="truncate text-sm text-base-content/60">{mediaDetail}</div>{/if}
        </div>
      </div>
    {:else}
      <div class="text-sm text-base-content/70">Nothing playing</div>
    {/if}
  </section>

  <section class="cd-card p-4">
    <div class="mb-3 text-xs text-base-content/60">Kindle</div>
    <div class="flex items-center gap-2 text-sm">
      <span class="size-2 shrink-0 rounded-full {dots[kindle.tone]}"></span>
      <span class="font-semibold">{kindle.text}</span>
    </div>
    {#if kindle.detail}
      <div class="mt-1 pl-4 text-sm text-base-content/60">{kindle.detail}</div>
    {/if}
    {#if !app.state.listening}
      <div class="mt-3 rounded-lg bg-[rgba(255,91,79,.13)] px-3 py-2 text-sm text-[#ff8278]">
        Cannot open port {app.state.settings.port}, so the Kindle cannot reach this PC. See the Settings page.
      </div>
    {/if}
    <div class="mt-3 flex items-center justify-between gap-2 border-t border-white/[.07] pt-3">
      <div class="min-w-0">
        <div class="text-sm">The Kindle app finds this PC by itself</div>
        <div class="text-xs text-base-content/60">If your network blocks discovery, use this address</div>
        <div class="truncate font-mono text-sm">{app.state.address}</div>
      </div>
      <button class="btn btn-ghost btn-sm btn-square" aria-label="Copy address" onclick={copyAddress}>
        <Copy size={15} />
      </button>
    </div>
  </section>
</div>
