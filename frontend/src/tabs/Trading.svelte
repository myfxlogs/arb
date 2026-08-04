<script>
  import { backend } from '../lib/backend.js'
  import Card from '../components/Card.svelte'

  let brokerName = $state('')
  let symbol = $state('EURUSD')
  let side = $state('BUY')
  let lots = $state('0.1')
  let price = $state('0')
  let slippage = $state('0')
  let result = $state('')
  let resultColor = $state('')
  let loading = $state(false)

  async function submitOrder() {
    loading = true
    result = ''
    try {
      const reply = await backend.submitOrder({
        brokerName,
        symbol,
        side,
        lots: parseFloat(lots) || 0,
        price: parseFloat(price) || 0,
        slippage: parseInt(slippage) || 0,
      })
      result = `状态: ${reply.status}  订单号: ${reply.ticket}`
      resultColor = 'var(--green)'
    } catch (err) {
      result = `错误: ${err}`
      resultColor = 'var(--red)'
    }
    loading = false
  }
</script>

<div style="max-width: 480px;">
  <Card title="手动下单">
    <div class="form-group">
      <span class="form-label">经纪商名称</span>
      <input class="form-input" bind:value={brokerName} placeholder="输入经纪商名称" />
    </div>

    <div class="form-group">
      <span class="form-label">品种</span>
      <input class="form-input" bind:value={symbol} placeholder="EURUSD" />
    </div>

    <div class="form-group">
      <span class="form-label">方向</span>
      <select class="form-select" bind:value={side}>
        <option value="BUY">买入</option>
        <option value="SELL">卖出</option>
      </select>
    </div>

    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px;">
      <div class="form-group">
        <span class="form-label">手数</span>
        <input class="form-input" type="number" step="0.01" bind:value={lots} />
      </div>
      <div class="form-group">
        <span class="form-label">价格 (0=市价)</span>
        <input class="form-input" type="number" step="0.00001" bind:value={price} />
      </div>
    </div>

    <div class="form-group">
      <span class="form-label">滑点</span>
      <input class="form-input" type="number" bind:value={slippage} />
    </div>

    <button class="btn btn-primary" style="width: 100%;" onclick={submitOrder} disabled={loading}>
      {loading ? '提交中...' : '提交订单'}
    </button>

    {#if result}
      <div style="margin-top: 12px; font-size: 14px; color: {resultColor};">{result}</div>
    {/if}
  </Card>
</div>
