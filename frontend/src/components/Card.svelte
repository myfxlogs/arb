<script>
  let { title, children } = $props()
</script>

<div class="card">
  {#if title}
    <div class="card-title">{title}</div>
  {/if}
  {@render children?.()}
</div>
