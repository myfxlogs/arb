<script>
  import { backend } from '../lib/backend.js'
  import { positions } from '../lib/stores.js'
  import Card from '../components/Card.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let loaded = $state(false)
  let signals = $state([])
  let orders = $state([])
  let brokerOrders = $state([])
  let subTab = $state('signals')
  let brokerNames = $state([])

  // Track broker names from position stream
  positions.subscribe((v) => {
    if (v && v.broker_positions) {
      brokerNames = v.broker_positions.map(bp => bp.broker_name)
    }
  })

  async function refresh() {
    loaded = false
    const now = Date.now()
    const from = now - 30 * 24 * 60 * 60 * 1000

    try {
      const [sigReply, ordReply] = await Promise.all([
        backend.getSignalHistory({ from_unix_ms: from, to_unix_ms: now, limit: 100 }),
        backend.getOrderHistory({ from_unix_ms: from, to_unix_ms: now, limit: 100 }),
      ])
      signals = sigReply.items || []
      orders = ordReply.items || []

      // Fetch broker order history for each connected broker
      const names = brokerNames.length > 0 ? brokerNames : []
      const brokerReplies = await Promise.all(
        names.map(name =>
          backend.getBrokerOrderHistory({
            broker_name: name,
            from_unix_ms: from,
            to_unix_ms: now,
          }).then(r => ({ name, orders: r.orders || [], error: r.error })).catch(e => ({ name, orders: [], error: String(e) }))
        )
      )
      brokerOrders = brokerReplies.flatMap(r =>
        (r.orders || []).map(o => ({ ...o, broker_name: r.name }))
      )
    } catch (err) {
      console.error('history fetch', err)
    }
    loaded = true
  }

  refresh()
</script>

<div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px;">
  <div style="display: flex; gap: 8px;">
    <button class="btn btn-secondary" class:btn-primary={subTab === 'signals'} onclick={() => subTab = 'signals'}>
      信号
    </button>
    <button class="btn btn-secondary" class:btn-primary={subTab === 'orders'} onclick={() => subTab = 'orders'}>
      策略订单
    </button>
    <button class="btn btn-secondary" class:btn-primary={subTab === 'broker'} onclick={() => subTab = 'broker'}>
      交易历史
    </button>
  </div>
  <button class="btn btn-secondary" onclick={refresh}>刷新</button>
</div>

{#if !loaded}
  <Card>
    {#each Array(8) as _}
      <Skeleton width="100%" height="20px" />
      <div style="height: 8px;"></div>
    {/each}
  </Card>
{:else if subTab === 'signals'}
  {#if signals.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">📭</div>
      <div class="empty-state-text">暂无信号记录</div>
    </div>
  {:else}
    <Card style="padding: 0; overflow: hidden;">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>策略</th>
            <th>时间</th>
            <th>已执行</th>
          </tr>
        </thead>
        <tbody>
          {#each signals as sig}
            <tr>
              <td>{sig.id}</td>
              <td>{sig.strategy}</td>
              <td>{new Date(sig.ts_unix_ms).toLocaleString('zh-CN')}</td>
              <td>
                <span class="badge" class:badge-green={sig.executed} class:badge-red={!sig.executed}>
                  {sig.executed ? '已执行' : '未执行'}
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </Card>
  {/if}
{:else if subTab === 'broker'}
  {#if brokerOrders.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">📭</div>
      <div class="empty-state-text">暂无交易历史</div>
    </div>
  {:else}
    <Card style="padding: 0; overflow: hidden;">
      <table class="data-table">
        <thead>
          <tr>
            <th>经纪商</th>
            <th>订单号</th>
            <th>品种</th>
            <th>方向</th>
            <th>手数</th>
            <th>开仓价</th>
            <th>平仓价</th>
            <th>盈亏</th>
            <th>开仓时间</th>
            <th>平仓时间</th>
          </tr>
        </thead>
        <tbody>
          {#each brokerOrders as ord}
            <tr>
              <td>{ord.broker_name}</td>
              <td>{ord.ticket}</td>
              <td>{ord.symbol}</td>
              <td><span class="badge" class:badge-green={ord.side === 'Buy'} class:badge-red={ord.side === 'Sell'}>{ord.side}</span></td>
              <td>{ord.lots ? ord.lots.toFixed(2) : '-'}</td>
              <td>{ord.open_price ? ord.open_price.toFixed(5) : '-'}</td>
              <td>{ord.close_price ? ord.close_price.toFixed(5) : '-'}</td>
              <td style="color: {ord.profit > 0 ? 'var(--green)' : ord.profit < 0 ? 'var(--red)' : ''}">{ord.profit ? ord.profit.toFixed(2) : '-'}</td>
              <td>{ord.open_time_unix_ms ? new Date(ord.open_time_unix_ms).toLocaleString('zh-CN') : '-'}</td>
              <td>{ord.close_time_unix_ms ? new Date(ord.close_time_unix_ms).toLocaleString('zh-CN') : '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </Card>
  {/if}
{:else}
  {#if orders.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">📭</div>
      <div class="empty-state-text">暂无订单记录</div>
    </div>
  {:else}
    <Card style="padding: 0; overflow: hidden;">
      <table class="data-table">
        <thead>
          <tr>
            <th>ClientID</th>
            <th>经纪商</th>
            <th>品种</th>
            <th>手数</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          {#each orders as ord}
            <tr>
              <td>{ord.client_id}</td>
              <td>{ord.broker}</td>
              <td>{ord.symbol}</td>
              <td>{ord.volume ? ord.volume.toFixed(2) : '-'}</td>
              <td>{new Date(ord.ts_unix_ms).toLocaleString('zh-CN')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </Card>
  {/if}
{/if}
