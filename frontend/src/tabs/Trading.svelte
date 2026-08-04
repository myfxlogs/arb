<script>
  import { backend } from '../lib/backend.js'
  import { positions } from '../lib/stores.js'
  import Card from '../components/Card.svelte'

  let brokers = $state([])
  let selectedBrokerName = $state('')
  let symbols = $state([])
  let symbolsLoading = $state(false)
  let symbolSearch = $state('')
  let symbol = $state('')
  let side = $state('BUY')
  let lots = $state('0.1')
  let price = $state('0')
  let slippage = $state('0')
  let result = $state('')
  let resultColor = $state('')
  let loading = $state(false)

  positions.subscribe((v) => {
    if (v && v.broker_positions) {
      brokers = v.broker_positions.map(bp => ({
        broker_name: bp.broker_name,
        login: bp.login,
        platform: bp.platform,
        is_connected: bp.is_connected,
      }))
    }
  })

  let brokerOptions = $derived(
    brokers.filter(b => b.is_connected)
  )

  let filteredSymbols = $derived(
    symbolSearch.trim()
      ? symbols.filter(s => s.toLowerCase().includes(symbolSearch.trim().toLowerCase()))
      : symbols
  )

  async function onBrokerChange(e) {
    selectedBrokerName = e.target.value
    symbol = ''
    symbolSearch = ''
    symbols = []
    if (!selectedBrokerName) return
    symbolsLoading = true
    try {
      const reply = await backend.getBrokerSymbols({ broker_name: selectedBrokerName })
      if (reply.error) {
        result = `加载品种失败: ${reply.error}`
        resultColor = 'var(--red)'
      } else {
        symbols = reply.symbols || []
      }
    } catch (err) {
      result = `加载品种失败: ${err}`
      resultColor = 'var(--red)'
    }
    symbolsLoading = false
  }

  async function submitOrder() {
    if (!selectedBrokerName) { alert('请选择交易账号'); return }
    if (!symbol) { alert('请选择品种'); return }
    loading = true
    result = ''
    try {
      const reply = await backend.submitOrder({
        broker_name: selectedBrokerName,
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
      <span class="form-label">交易账号</span>
      <select class="form-select" value={selectedBrokerName} onchange={onBrokerChange}>
        <option value="">请选择...</option>
        {#each brokerOptions as b}
          <option value={b.broker_name}>{b.login} · {b.platform}</option>
        {/each}
      </select>
    </div>

    <div class="form-group">
      <span class="form-label">品种</span>
      {#if !selectedBrokerName}
        <input class="form-input" placeholder="请先选择交易账号" disabled />
      {:else if symbolsLoading}
        <input class="form-input" placeholder="加载品种中..." disabled />
      {:else}
        <input class="form-input" placeholder="输入品种名称搜索..."
          bind:value={symbolSearch}
          oninput={(e) => { symbolSearch = e.target.value }}
          style="margin-bottom: 8px;" />
        {#if filteredSymbols.length > 0}
          <div class="symbol-list">
            {#each filteredSymbols.slice(0, 50) as s}
              <div class="symbol-item" class:selected={symbol === s}
                onclick={() => { symbol = s; symbolSearch = '' }}>
                {s}
              </div>
            {/each}
            {#if filteredSymbols.length > 50}
              <div style="padding: 4px 8px; font-size: 12px; color: var(--text-dim);">
                还有 {filteredSymbols.length - 50} 个品种，请输入关键词筛选
              </div>
            {/if}
          </div>
        {:else}
          <div style="font-size: 13px; color: var(--text-dim); padding: 8px;">
            {symbols.length === 0 ? '无可用品种' : '未找到匹配品种'}
          </div>
        {/if}
        {#if symbol}
          <div style="margin-top: 8px; font-size: 14px;">
            已选: <strong>{symbol}</strong>
          </div>
        {/if}
      {/if}
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

    <button class="btn btn-primary" style="width: 100%;" onclick={submitOrder}
      disabled={loading || !selectedBrokerName || !symbol}>
      {loading ? '提交中...' : '提交订单'}
    </button>

    {#if result}
      <div style="margin-top: 12px; font-size: 14px; color: {resultColor};">{result}</div>
    {/if}
  </Card>
</div>

<style>
  .symbol-list {
    max-height: 200px;
    overflow-y: auto;
    border: 1px solid var(--separator);
    border-radius: 8px;
  }
  .symbol-item {
    padding: 6px 12px;
    cursor: pointer;
    font-size: 13px;
    transition: background 0.1s;
  }
  .symbol-item:hover {
    background: var(--accent-dim);
  }
  .symbol-item.selected {
    background: var(--accent);
    color: #fff;
  }
</style>
