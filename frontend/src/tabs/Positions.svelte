<script>
  import { positions } from '../lib/stores.js'
  import Card from '../components/Card.svelte'
  import StatCard from '../components/StatCard.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let loaded = $state(false)
  let data = $state({ broker_positions: [] })

  positions.subscribe((v) => {
    if (v && v.broker_positions) {
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
  {#if data.broker_positions.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">📊</div>
      <div class="empty-state-text">暂无持仓数据</div>
    </div>
  {:else}
    {#each data.broker_positions as bp}
      <Card style="margin-bottom: 16px;">
        <div class="broker-header">
          <div class="broker-title">
            <span class="broker-name">{bp.broker_name}</span>
            {#if bp.login}
              <span class="stat">#{bp.login}</span>
            {/if}
            {#if bp.platform}
              <span class="badge badge-blue">{bp.platform}</span>
            {/if}
            {#if bp.leverage}
              <span class="stat">1:{bp.leverage}</span>
            {/if}
          </div>
          <div class="broker-stats">
            <span class="stat">余额: <strong>{fmt(bp.balance)}</strong></span>
            <span class="stat">净值: <strong>{fmt(bp.equity)}</strong></span>
            <span class="stat">浮盈: <strong style="color:{pnlColor(bp.total_floating_pnl)}">{fmt(bp.total_floating_pnl)}</strong></span>
            <span class="stat">保证金: <strong>{fmt(bp.margin_used)}</strong></span>
            <span class="stat">可用: <strong>{fmt(bp.margin_free)}</strong></span>
            <span class="stat">保证金率: <strong>{bp.margin_level_pct ? bp.margin_level_pct.toFixed(1) + '%' : '-'}</strong></span>
            {#if bp.credit}
              <span class="stat">信用: <strong>{fmt(bp.credit)}</strong></span>
            {/if}
            {#if bp.currency}
              <span class="stat">{bp.currency}</span>
            {/if}
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
                  <td><span class="badge" class:badge-green={pos.side === 'Buy'} class:badge-red={pos.side === 'Sell'}>{pos.side}</span></td>
                  <td>{fmt(pos.lots)}</td>
                  <td>{pos.open_price ? pos.open_price.toFixed(5) : '-'}</td>
                  <td>{pos.current_price ? pos.current_price.toFixed(5) : '-'}</td>
                  <td style="color: {pnlColor(pos.floating_pnl)}">{fmt(pos.floating_pnl)}</td>
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
  .broker-title {
    display: flex;
    align-items: center;
    gap: 8px;
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
    flex-wrap: wrap;
  }
  .badge-blue {
    background: var(--accent-dim);
    color: var(--accent);
  }
  .no-positions {
    padding: 16px;
    color: var(--text-dim);
    text-align: center;
  }
</style>
