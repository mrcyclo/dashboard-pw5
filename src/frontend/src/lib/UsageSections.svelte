<script>
  let { sections } = $props()
</script>

{#each sections as section (section.title)}
  <div class="mb-3 last:mb-0">
    <div class="mb-2 flex items-baseline justify-between gap-3">
      <span class="text-sm font-semibold">{section.title}</span>
      {#if section.refresh}
        <span class="text-xs text-base-content/60">{section.refresh}</span>
      {/if}
    </div>
    {#each section.rows as row (row.label)}
      <div class="mb-2.5 last:mb-0">
        <div class="flex items-baseline justify-between gap-3 text-sm">
          <span>{row.label}</span>
          <span class="text-base-content/80">{row.used}</span>
        </div>
        <progress class="progress progress-primary mt-1 h-1.5 w-full" value={row.percent} max="100"></progress>
        {#if row.refresh}
          <div class="mt-0.5 text-xs text-base-content/60">{row.refresh}</div>
        {/if}
      </div>
    {/each}
  </div>
{/each}
