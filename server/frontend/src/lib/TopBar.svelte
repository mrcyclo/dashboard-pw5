<script>
  import { app } from './store.svelte.js'
  import { kindleStatus } from './format.js'

  let { pages, current, onselect } = $props()

  let kindle = $derived(app.state ? kindleStatus(app.state.kindle, app.now) : null)
  const tones = {
    ok: 'bg-[rgba(63,182,139,.16)] text-[#5fd0a6]',
    warn: 'bg-[rgba(255,216,20,.14)] text-[#ffd814]',
    idle: 'bg-white/[.07] text-[#a1a1a6]',
  }
</script>

<header class="flex h-[52px] shrink-0 items-center gap-[22px] border-b border-white/[.07] px-[18px]">
  <span class="font-bold">Claude Dashboard</span>

  <nav class="flex h-full items-center gap-[22px]">
    {#each pages as p (p.id)}
      <button
        class="h-full cursor-pointer text-[13px] transition-colors {current === p.id
          ? 'text-base-content shadow-[inset_0_-2px_#ff9900]'
          : 'text-[#a1a1a6] hover:text-base-content'}"
        aria-current={current === p.id ? 'page' : undefined}
        onclick={() => onselect(p.id)}
      >
        {p.label}
      </button>
    {/each}
  </nav>

  {#if kindle}
    <span class="ml-auto rounded-full px-2.5 py-1 text-xs {tones[kindle.tone]}">
      {kindle.tone === 'ok' ? '● Kindle connected' : kindle.tone === 'warn' ? '● Kindle disconnected' : '○ No Kindle yet'}
    </span>
  {/if}
</header>
