<script>
  import { onMount } from 'svelte'
  import { CircleAlert, CircleCheck } from '@lucide/svelte'
  import { app, init } from './lib/store.svelte.js'
  import TopBar from './lib/TopBar.svelte'
  import Overview from './pages/Overview.svelte'
  import Kindle from './pages/Kindle.svelte'
  import Settings from './pages/Settings.svelte'
  import Logs from './pages/Logs.svelte'

  const pages = [
    { id: 'overview', label: 'Overview', component: Overview },
    { id: 'kindle', label: 'Kindle', component: Kindle },
    { id: 'settings', label: 'Settings', component: Settings },
    { id: 'logs', label: 'Logs', component: Logs },
  ]
  const requested = location.hash.slice(1)
  let current = $state(pages.some((p) => p.id === requested) ? requested : 'overview')
  let Page = $derived(pages.find((p) => p.id === current).component)

  onMount(init)
</script>

<div class="cd-glow flex h-full flex-col">
  <TopBar {pages} {current} onselect={(id) => (current = id)} />

  <main class="flex min-h-0 flex-1 flex-col px-6 py-5">
    {#if app.state}
      <Page />
    {:else}
      <div class="grid flex-1 place-items-center text-sm text-base-content/60">Loading...</div>
    {/if}
  </main>
</div>

<div class="toast toast-top toast-center z-[1000]">
  {#each app.toasts as t (t.id)}
    <div class="alert cd-toast {t.type === 'error' ? 'cd-toast-error' : 'cd-toast-success'} max-w-sm py-2 text-sm">
      {#if t.type === 'error'}<CircleAlert size={18} />{:else}<CircleCheck size={18} />{/if}
      <span class="whitespace-normal">{t.message}</span>
    </div>
  {/each}
</div>
