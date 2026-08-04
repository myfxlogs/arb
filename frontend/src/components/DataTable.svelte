<script>
  let { headers = [], rows = [], loaded = true } = $props()

  function cellColor(row, colIdx) {
    if (row._colors && row._colors[colIdx]) return row._colors[colIdx]
    return ''
  }
</script>

{#if !loaded}
  <div class="card">
    {#each Array(5) as _}
      <div style="display: flex; gap: 16px; margin-bottom: 12px;">
        {#each headers as _}
          <div class="skeleton" style="flex:1; height: 20px;"></div>
        {/each}
      </div>
    {/each}
  </div>
{:else if rows.length === 0}
  <div class="card">
    <div class="empty-state">
      <div class="empty-state-icon">📭</div>
      <div class="empty-state-text">暂无数据</div>
    </div>
  </div>
{:else}
  <div class="card" style="padding: 0; overflow: hidden;">
    <table class="data-table">
      <thead>
        <tr>
          {#each headers as h}
            <th>{h}</th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each rows as row}
          <tr>
            {#each row._cells as cell, colIdx}
              <td style={cellColor(row, colIdx) ? `color: ${cellColor(row, colIdx)}` : ''}>
                {cell}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
