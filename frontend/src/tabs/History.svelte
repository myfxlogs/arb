<script>
  import { backend } from '../lib/backend.js'
  import Card from '../components/Card.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let loaded = $state(false)
  let signals = $state([])
  let orders = $state([])
  let subTab = $state('signals')

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
      订单
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
