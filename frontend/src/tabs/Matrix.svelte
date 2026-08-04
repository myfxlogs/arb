<script>
  import { spreadMatrix } from '../lib/stores.js'
  import Card from '../components/Card.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let loaded = $state(false)
  let data = $state({ rows: [], totalSymbols: 0 })

  spreadMatrix.subscribe((v) => {
    if (v && v.rows && v.rows.length > 0) {
      data = v
      loaded = true
    }
  })

  function fmtBps(v) {
    return v.toFixed(1)
  }

  function cellColor(cell) {
    if (cell.isArbitrageable) return 'var(--green)'
    if (cell.estimatedNetProfitBps > 0) return 'var(--yellow)'
    if (cell.spreadToBestAskBps < 0) return 'var(--red)'
    return 'var(--text-primary)'
  }
</script>

{#if !loaded}
  <div class="grid">
    {#each Array(6) as _}
      <Card>
        <Skeleton width="60%" height="24px" />
        <div style="margin-top: 12px; display: flex; gap: 8px; flex-wrap: wrap;">
          {#each Array(5) as _}
            <Skeleton width="60px" height="28px" />
          {/each}
        </div>
      </Card>
    {/each}
  </div>
{:else}
  <div class="matrix-grid">
    {#each data.rows as row}
      <Card>
        <div class="broker-name">{row.brokerName}</div>
        <div class="cells">
          {#each row.cells as cell}
            <div
              class="cell"
              class:arb={cell.isArbitrageable}
              style="color: {cellColor(cell)}"
              title={cell.symbol + ': ' + fmtBps(cell.spreadToBestAskBps) + ' bps'}
            >
              <span class="cell-symbol">{cell.symbol}</span>
              <span class="cell-value">{fmtBps(cell.spreadToBestAskBps)}</span>
            </div>
          {/each}
        </div>
      </Card>
    {/each}
  </div>
{/if}

<style>
  .matrix-grid {
    display: grid;
    gap: 12px;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  }
  .broker-name {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 12px;
  }
  .cells {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .cell {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    background: rgba(255, 255, 255, 0.5);
    min-width: 64px;
    transition: transform var(--transition), background var(--transition);
  }
  .cell:hover {
    transform: scale(1.05);
    background: rgba(255, 255, 255, 0.8);
  }
  .cell.arb {
    background: rgba(52, 199, 89, 0.1);
  }
  .cell-symbol {
    font-size: 11px;
    color: var(--text-dim);
  }
  .cell-value {
    font-size: 14px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
</style>
