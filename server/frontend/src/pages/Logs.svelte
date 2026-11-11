<script>
  import { onMount } from 'svelte'
  import { FileText } from '@lucide/svelte'
  import { GetLogs, OpenLogFile } from '../../bindings/claudedash/internal/desk/app'
  import { call } from '../lib/store.svelte.js'
  import { timeText } from '../lib/format.js'

  const POLL_MS = 2000
  const categories = [
    { id: 'all', label: 'All' },
    { id: 'usage', label: 'Usage' },
    { id: 'token', label: 'Token' },
    { id: 'kindle', label: 'Kindle' },
    { id: 'media', label: 'Media' },
    { id: 'system', label: 'System' },
  ]
  const badges = {
    usage: 'bg-[rgba(255,153,0,.16)] text-[#febd69]',
    token: 'bg-[rgba(255,216,20,.14)] text-[#ffd814]',
    kindle: 'bg-[rgba(63,182,139,.16)] text-[#5fd0a6]',
    media: 'bg-[rgba(90,184,201,.16)] text-[#7fcfdd]',
    system: 'bg-white/[.07] text-[#a1a1a6]',
  }
  const names = Object.fromEntries(categories.map((c) => [c.id, c.label]))

  let entries = $state([])
  let filter = $state('all')
  let shown = $derived(filter === 'all' ? entries : entries.filter((e) => e.category === filter))

  async function load() {
    try {
      entries = (await GetLogs()) ?? []
    } catch {
      // Keep what is on screen; the next poll retries.
    }
  }

  function dayText(epoch) {
    const date = new Date(epoch * 1000)
    const pad = (n) => String(n).padStart(2, '0')
    return `${pad(date.getDate())}/${pad(date.getMonth() + 1)}`
  }

  onMount(() => {
    load()
    const timer = setInterval(load, POLL_MS)
    return () => clearInterval(timer)
  })
</script>

<header class="mb-4 flex items-end justify-between">
  <div>
    <h1 class="text-2xl font-semibold">Logs</h1>
    <p class="text-sm text-base-content/60">Latest 500 lines, newest first</p>
  </div>
  <button class="btn btn-ghost btn-sm gap-1.5" onclick={() => call(OpenLogFile)}>
    <FileText size={15} />
    Open log file
  </button>
</header>

<div class="mb-3 flex gap-1.5">
  {#each categories as c (c.id)}
    <button
      class="cursor-pointer rounded-full px-3 py-1 text-xs transition-colors {filter === c.id
        ? 'bg-[#ff9900] font-semibold text-[#131921]'
        : 'bg-white/[.07] text-[#a1a1a6] hover:text-base-content'}"
      onclick={() => (filter = c.id)}
    >
      {c.label}
    </button>
  {/each}
</div>

<div class="cd-card cd-scroll min-h-0 flex-1 overflow-y-auto select-text">
  {#each shown as entry, i (i)}
    <div class="flex items-baseline gap-3 border-t border-white/[.05] px-4 py-2 text-sm first:border-t-0">
      <span class="shrink-0 font-mono text-xs text-base-content/50">{dayText(entry.time)} {timeText(entry.time)}</span>
      <span class="w-[68px] shrink-0 rounded-full px-2 py-0.5 text-center text-[11px] {badges[entry.category] ?? badges.system}">
        {names[entry.category] ?? entry.category}
      </span>
      <span class="min-w-0 break-words">{entry.message}</span>
    </div>
  {:else}
    <div class="grid h-full place-items-center py-10 text-sm text-base-content/60">No entries yet</div>
  {/each}
</div>
