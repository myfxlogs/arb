<script>
  import { positions } from '../lib/stores.js'
  import Card from '../components/Card.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let loaded = $state(false)
  let data = $state({ brokerPositions: [] })

  positions.subscribe((v) => {
    if (v && v.brokerPositions) {
      data = v
      loaded = true
    }
  })

  function fmt(v) {
    return v ? v.toFixed(2) : '0.00'
  }

  function pnlColor(pnl) {
    if (pnl > 0) return 'var(--green)'
    if (pnl < 0) return 'var(--red)'
    return 'var(--text-primary)'
  }
</script>

{#if !loaded}
  <div class="grid">
    {#each Array(4) as _}
      <Card>
        <Skeleton width="50%" height="20px" />
        <div style="margin-top: 12px;">
          {#each Array(3) as _}
            <Skeleton width="100%" height="16px" />
            <div style="height: 8px;"></div>
          {/each}
        </div>
      </Card>
    {/each}
  </div>
{:else}
  {#if data.brokerPositions.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">📊</div>
      <div class="empty-state-text">暂无持仓数据</div>
    </div>
  {:else}
    {#each data.brokerPositions as bp}
      <Card style="margin-bottom: 16px;">
        <div class="broker-header">
          <span class="broker-name">{bp.brokerName}</span>
          <div class="broker-stats">
            <span class="stat">净值: <strong>{fmt(bp.equity)}</strong></span>
            <span class="stat">可用: <strong>{fmt(bp.marginFree)}</strong></span>
          </div>
        </div>

        {#if bp.positions && bp.positions.length > 0}
          <table class="data-table">
            <thead>
              <tr>
                <th>订单号</th>
                <th>品种</th>
                <th>方向</th>
                <th>手数</th>
                <th>开仓价</th>
                <th>现价</th>
                <th>浮盈</th>
              </tr>
            </thead>
            <tbody>
              {#each bp.positions as pos}
                <tr>
                  <td>{pos.ticket}</td>
                  <td>{pos.symbol}</td>
                  <td><span class="badge" class:badge-green={pos.side === 'BUY'} class:badge-red={pos.side === 'SELL'}>{pos.side}</span></td>
                  <td>{fmt(pos.lots)}</td>
                  <td>{pos.openPrice ? pos.openPrice.toFixed(5) : '-'}</td>
                  <td>{pos.currentPrice ? pos.currentPrice.toFixed(5) : '-'}</td>
                  <td style="color: {pnlColor(pos.floatingPnl)}">{fmt(pos.floatingPnl)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <div class="no-positions">无持仓</div>
        {/if}
      </Card>
    {/each}
  {/if}
{/if}

<style>
  .broker-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }
  .broker-name {
    font-size: 15px;
    font-weight: 600;
  }
  .broker-stats {
    display: flex;
    gap: 16px;
    font-size: 13px;
    color: var(--text-dim);
  }
  .no-positions {
    padding: 16px;
    color: var(--text-dim);
    text-align: center;
  }
</style>
