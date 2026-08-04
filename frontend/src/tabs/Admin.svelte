<script>
  import { backend } from '../lib/backend.js'
  import Card from '../components/Card.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  let statusItems = $state([])
  let killActive = $state(false)
  let statusLoaded = $state(false)
  let brokers = $state([])
  let brokersLoaded = $state(false)
  let selectedBroker = $state(-1)

  // Wizard state (3-step flow matching ant frontend)
  let showWizard = $state(false)
  let wizardStep = $state(0) // 0=search, 1=credentials, 2=confirm
  let wizardPlatform = $state('MT5')
  let wizardKeyword = $state('')
  let wizardStatus = $state('')
  let searchResult = $state(null)
  let wizardCompanies = $state([])
  let wizardCompany = $state('') // selected company_name
  let wizardServers = $state([])
  let wizardServer = $state('') // selected server name
  let selectedServerObj = $state(null) // full server object with access
  let wizardUser = $state('')
  let wizardPassword = $state('')
  let wizardName = $state('')
  let serverFilter = $state('')
  let wizardLoading = $state(false)
  let wizardError = $state('')

  async function refreshStatus() {
    statusLoaded = false
    try {
      const [ss, ks] = await Promise.all([
        backend.getStrategyStatus({}),
        backend.getKillSwitchStatus({}),
      ])
      statusItems = ss.items || []
      killActive = ks.active || false
    } catch (err) {
      console.error('status', err)
    }
    statusLoaded = true
  }

  async function refreshBrokers() {
    brokersLoaded = false
    try {
      const reply = await backend.getAccountSnapshots({})
      brokers = reply.items || []
    } catch (err) {
      console.error('brokers', err)
    }
    brokersLoaded = true
  }

  async function kill() {
    if (!confirm('确认平仓所有持仓并停止交易？')) return
    try {
      await backend.kill({})
      refreshStatus()
    } catch (err) {
      alert('停止错误: ' + err)
    }
  }

  async function resume() {
    try {
      await backend.resume({})
      refreshStatus()
    } catch (err) {
      alert('恢复错误: ' + err)
    }
  }

  async function resetCB() {
    try {
      await backend.resetCircuitBreaker({})
      refreshStatus()
    } catch (err) {
      alert('重置错误: ' + err)
    }
  }

  async function toggleStrategy(name, enabled) {
    try {
      await backend.toggleStrategy({ name, enabled: !enabled })
      refreshStatus()
    } catch (err) {
      alert('切换错误: ' + err)
    }
  }

  async function removeBroker() {
    if (selectedBroker < 0 || selectedBroker >= brokers.length) return
    const name = brokers[selectedBroker].broker_name
    if (!confirm(`确认删除经纪商 ${name}？`)) return
    try {
      const reply = await backend.removeBroker({ name })
      if (!reply.success) {
        alert(reply.error)
        return
      }
      refreshBrokers()
    } catch (err) {
      alert('删除错误: ' + err)
    }
  }

  // === Wizard ===

  function friendlyError(msg) {
    if (!msg) return ''
    if (msg.includes('SERVICE_NOT_AVAILABLE') || msg.includes('code=11'))
      return '经纪商服务器不可用，请稍后重试'
    if (msg.includes('INVALID_ACCOUNT') || msg.includes('code=1001'))
      return '账号或密码错误'
    if (msg.includes('connection failed') || msg.includes('connect'))
      return '连接失败，请检查服务器和网络'
    if (msg.includes('timeout') || msg.includes('Timed out'))
      return '连接超时，请稍后重试'
    return msg
  }

  async function wizardSearch() {
    if (!wizardKeyword.trim()) { wizardStatus = '请输入经纪商关键词'; return }
    wizardStatus = '搜索中...'
    wizardCompanies = []
    wizardCompany = ''
    wizardServers = []
    wizardServer = ''
    selectedServerObj = null
    const platform = wizardPlatform === 'MT5' ? 1 : 0
    try {
      const reply = await backend.searchBroker({ company: wizardKeyword, platform })
      if (reply.error) { wizardStatus = reply.error; return }
      searchResult = reply
      if (!reply.companies || reply.companies.length === 0) {
        wizardStatus = '未找到匹配的经纪商'
        return
      }
      wizardCompanies = reply.companies.map(c => c.company_name)
      wizardStatus = `找到 ${reply.companies.length} 个经纪商`
    } catch (err) {
      wizardStatus = `搜索失败: ${err}`
    }
  }

  function onCompanyChange(companyName) {
    if (!searchResult) return
    const c = searchResult.companies.find(c => c.company_name === companyName)
    if (!c) return
    wizardServers = [...c.servers].sort((a, b) => a.name.localeCompare(b.name))
    wizardServer = ''
    selectedServerObj = null
  }

  function onServerChange(serverName) {
    selectedServerObj = wizardServers.find(s => s.name === serverName) || null
    if (selectedServerObj && !wizardName) wizardName = selectedServerObj.name
  }

  function wizardNext() {
    if (wizardStep === 0) {
      if (!wizardCompany) { alert('请选择经纪商公司'); return }
      if (!wizardServer || !selectedServerObj) { alert('请选择服务器'); return }
      if (!selectedServerObj.access) { alert('该服务器无可用地址'); return }
    } else if (wizardStep === 1) {
      if (!wizardUser.trim()) { alert('请输入交易账号'); return }
      if (!/^\d+$/.test(wizardUser.trim())) { alert('交易账号只能包含数字'); return }
      if (!wizardPassword) { alert('请输入密码'); return }
      wizardError = ''
    }
    wizardStep++
  }

  function wizardPrev() {
    if (wizardStep > 0) wizardStep--
    wizardError = ''
  }

  async function wizardSubmit() {
    wizardLoading = true
    wizardError = ''
    const platform = wizardPlatform === 'MT5' ? 1 : 0
    const companyName = wizardCompany
    const serverName = wizardServer
    let host = ''
    let port = 443
    if (selectedServerObj && selectedServerObj.access) {
      const parts = selectedServerObj.access.split(':')
      host = parts[0]
      if (parts.length > 1) port = parseInt(parts[1]) || 443
    }
    const name = wizardName || companyName
    try {
      const reply = await backend.addBroker({
        name,
        platform,
        host,
        port,
        user: parseInt(wizardUser) || 0,
        password: wizardPassword,
        server: serverName,
      })
      if (!reply.success) {
        wizardError = friendlyError(reply.error)
        wizardLoading = false
        return
      }
      wizardLoading = false
      showWizard = false
      resetWizard()
      refreshBrokers()
    } catch (err) {
      wizardError = friendlyError(String(err))
      wizardLoading = false
    }
  }

  function resetWizard() {
    wizardStep = 0
    wizardKeyword = ''
    wizardStatus = ''
    searchResult = null
    wizardCompanies = []
    wizardCompany = ''
    wizardServers = []
    wizardServer = ''
    selectedServerObj = null
    wizardUser = ''
    wizardPassword = ''
    wizardName = ''
    serverFilter = ''
    wizardLoading = false
    wizardError = ''
  }

  refreshStatus()
  refreshBrokers()
</script>

<!-- 风控管理 -->
<Card title="风控管理">
  {#if !statusLoaded}
    <Skeleton width="80%" height="20px" />
    <div style="height: 8px;"></div>
    <Skeleton width="60%" height="20px" />
  {:else}
    {#if killActive}
      <div class="badge badge-red" style="font-size: 14px; padding: 6px 16px; margin-bottom: 12px;">
        ⚠ 紧急停止已激活
      </div>
    {/if}

    {#each statusItems as item}
      <div class="strategy-row">
        <span class="strategy-name">{item.name}</span>
        <span class="badge" class:badge-green={item.enabled} class:badge-red={!item.enabled}>
          {item.enabled ? '启用' : '停用'}
        </span>
        {#if item.circuit_breaker_open}
          <span class="badge badge-red">熔断</span>
        {/if}
        <span class="strategy-stat">连亏: {item.consecutive_losses}</span>
        <span class="strategy-stat" style="color: {item.pnl_today >= 0 ? 'var(--green)' : 'var(--red)'}">
          今日: {item.pnl_today ? item.pnl_today.toFixed(2) : '0.00'}
        </span>
        <button class="btn btn-secondary" style="padding: 4px 12px; font-size: 12px;"
          onclick={() => toggleStrategy(item.name, item.enabled)}>
          {item.enabled ? '停用' : '启用'}
        </button>
      </div>
    {/each}

    <div style="display: flex; gap: 8px; margin-top: 16px;">
      <button class="btn btn-secondary" onclick={refreshStatus}>刷新状态</button>
      <button class="btn btn-danger" onclick={kill}>紧急停止</button>
      <button class="btn btn-secondary" onclick={resume}>恢复交易</button>
      <button class="btn btn-secondary" onclick={resetCB}>重置熔断器</button>
    </div>
  {/if}
</Card>

<div style="height: 16px;"></div>

<!-- 经纪商管理 -->
<Card title="经纪商管理">
  <div style="display: flex; gap: 8px; margin-bottom: 16px;">
    <button class="btn btn-primary" onclick={() => { resetWizard(); showWizard = true }}>添加经纪商</button>
    <button class="btn btn-danger" onclick={removeBroker}>删除经纪商</button>
    <button class="btn btn-secondary" onclick={refreshBrokers}>刷新列表</button>
  </div>

  {#if !brokersLoaded}
    {#each Array(3) as _}
      <Skeleton width="100%" height="24px" />
      <div style="height: 8px;"></div>
    {/each}
  {:else if brokers.length === 0}
    <div class="empty-state">
      <div class="empty-state-icon">🔗</div>
      <div class="empty-state-text">暂无经纪商</div>
    </div>
  {:else}
    <table class="data-table">
      <thead>
        <tr>
          <th>名称</th>
          <th>已连接</th>
          <th>净值</th>
          <th>可用保证金</th>
        </tr>
      </thead>
      <tbody>
        {#each brokers as b, i}
          <tr
            onclick={() => selectedBroker = i}
            style="cursor: pointer; background: {selectedBroker === i ? 'var(--accent-dim)' : ''}"
          >
            <td>{b.broker_name}</td>
            <td>
              <span class="badge" class:badge-green={b.is_connected} class:badge-red={!b.is_connected}>
                {b.is_connected ? '在线' : '离线'}
              </span>
            </td>
            <td>{b.equity ? b.equity.toFixed(2) : '-'}</td>
            <td>{b.free_margin ? b.free_margin.toFixed(2) : '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</Card>

<!-- 添加经纪商向导 -->
{#if showWizard}
  <div class="modal-overlay" role="button" tabindex="0" onclick={() => showWizard = false} onkeydown={(e) => e.key === 'Escape' && (showWizard = false)}>
    <div class="modal-content" role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
      <div class="modal-title">添加经纪商</div>

      <!-- Step indicator -->
      <div class="step-indicator">
        {#each ['搜索', '凭据', '确认'] as label, i}
          <div class="step-item">
            <div class="step-circle" class:active={wizardStep >= i} class:done={wizardStep > i}>
              {#if wizardStep > i}✓{:else}{i + 1}{/if}
            </div>
            <span class="step-label" class:active={wizardStep >= i}>{label}</span>
          </div>
          {#if i < 2}<div class="step-line" class:active={wizardStep > i}></div>{/if}
        {/each}
      </div>

      <!-- Step 0: Search + company + server -->
      {#if wizardStep === 0}
        <div class="form-group">
          <span class="form-label">平台</span>
          <div class="platform-tabs">
            {#each ['MT4', 'MT5'] as p}
              <button class="platform-tab" class:active={wizardPlatform === p}
                onclick={() => { wizardPlatform = p; wizardCompanies = []; wizardCompany = ''; wizardServers = []; wizardServer = ''; selectedServerObj = null; searchResult = null; wizardStatus = '' }}>
                {p}
              </button>
            {/each}
          </div>
        </div>
        <div class="form-group">
          <span class="form-label">经纪商关键词</span>
          <div style="display: flex; gap: 8px;">
            <input class="form-input" bind:value={wizardKeyword} placeholder="如 OctaFX, RoboForex, Exness"
              onkeydown={(e) => e.key === 'Enter' && wizardSearch()} />
            <button class="btn btn-primary" onclick={wizardSearch}>搜索</button>
          </div>
        </div>
        {#if wizardStatus}
          <div style="margin-top: 8px; font-size: 13px; color: var(--text-dim);">{wizardStatus}</div>
        {/if}
        {#if wizardCompanies.length > 0}
          <div class="form-group" style="margin-top: 12px;">
            <span class="form-label">公司</span>
            <select class="form-select" bind:value={wizardCompany} onchange={(e) => onCompanyChange(e.target.value)}>
              <option value="">请选择...</option>
              {#each wizardCompanies as c}
                <option value={c}>{c}</option>
              {/each}
            </select>
          </div>
        {/if}
        {#if wizardServers.length > 0}
          <div class="form-group">
            <span class="form-label">服务器 ({wizardServers.length} 个)</span>
            {#if wizardServers.length > 10}
              <input class="form-input" bind:value={serverFilter} placeholder="过滤服务器名称..." style="margin-bottom: 8px;" />
            {/if}
            <select class="form-select" bind:value={wizardServer} onchange={(e) => onServerChange(e.target.value)}>
              <option value="">请选择...</option>
              {#each wizardServers.filter(s => !serverFilter || s.name.toLowerCase().includes(serverFilter.toLowerCase())) as s}
                <option value={s.name}>{s.name}</option>
              {/each}
            </select>
          </div>
          {#if selectedServerObj}
            <div style="margin-top: 6px; font-size: 12px; color: var(--text-dim);">地址: {selectedServerObj.access}</div>
          {/if}
        {/if}

      <!-- Step 1: Credentials -->
      {:else if wizardStep === 1}
        <div class="server-info-box">
          <div style="font-weight: 600;">{wizardServer}</div>
          <div style="font-size: 13px; color: var(--text-dim);">{wizardCompany} · {wizardPlatform}</div>
        </div>
        <div class="form-group">
          <span class="form-label">交易账号</span>
          <input class="form-input" bind:value={wizardUser} placeholder="交易账号（纯数字）" />
          {#if wizardUser && !/^\d+$/.test(wizardUser)}
            <div style="margin-top: 4px; font-size: 12px; color: var(--red);">账号只能包含数字</div>
          {/if}
        </div>
        <div class="form-group">
          <span class="form-label">密码</span>
          <input class="form-input" type="text" bind:value={wizardPassword} placeholder="密码（明文显示）" />
          <div style="margin-top: 4px; font-size: 12px; color: var(--text-dim);">mtapi 要求明文传输密码</div>
        </div>
        <div class="form-group">
          <span class="form-label">自定义名称（可选）</span>
          <input class="form-input" bind:value={wizardName} placeholder="留空则用服务器名" />
        </div>

      <!-- Step 2: Confirm -->
      {:else}
        <div class="server-info-box">
          <div style="font-weight: 600;">{wizardServer}</div>
          <div style="font-size: 13px; color: var(--text-dim);">{wizardCompany} · {wizardPlatform} · {wizardUser}</div>
          {#if selectedServerObj}
            <div style="font-size: 12px; color: var(--text-dim); margin-top: 4px;">{selectedServerObj.access}</div>
          {/if}
        </div>

        {#if wizardError}
          <div class="wizard-error">
            <span style="font-size: 16px;">⚠</span>
            <span>{wizardError}</span>
          </div>
        {/if}
      {/if}

      <div class="wizard-nav">
        {#if wizardStep > 0}
          <button class="btn btn-secondary" onclick={wizardPrev}>上一步</button>
        {/if}
        {#if wizardStep < 2}
          <button class="btn btn-primary" onclick={wizardNext}
            disabled={wizardStep === 0 && !selectedServerObj}>下一步</button>
        {:else}
          <button class="btn btn-primary" onclick={wizardSubmit} disabled={wizardLoading}>
            {wizardLoading ? '连接中...' : '确认添加'}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .strategy-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px solid rgba(220, 226, 234, 0.5);
  }
  .strategy-name {
    font-weight: 500;
    min-width: 120px;
  }
  .strategy-stat {
    font-size: 13px;
    color: var(--text-dim);
  }
  .modal-overlay {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
    background: rgba(0, 0, 0, 0.3);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  .modal-content {
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(40px) saturate(180%);
    border-radius: 16px;
    padding: 32px;
    width: 520px;
    max-height: 600px;
    overflow-y: auto;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
  }
  .modal-title {
    font-size: 20px;
    font-weight: 700;
    margin-bottom: 20px;
  }
  .step-indicator {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    margin-bottom: 24px;
  }
  .step-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }
  .step-circle {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    font-weight: 600;
    background: rgba(220, 226, 234, 0.5);
    color: var(--text-dim);
  }
  .step-circle.active {
    background: var(--accent);
    color: #fff;
  }
  .step-circle.done {
    background: var(--accent);
    color: #fff;
  }
  .step-label {
    font-size: 11px;
    color: var(--text-dim);
  }
  .step-label.active {
    color: var(--text);
  }
  .step-line {
    width: 40px;
    height: 2px;
    background: rgba(220, 226, 234, 0.5);
    margin: 0 4px;
    margin-bottom: 20px;
  }
  .step-line.active {
    background: var(--accent);
  }
  .platform-tabs {
    display: flex;
    gap: 8px;
  }
  .platform-tab {
    flex: 1;
    padding: 10px;
    border-radius: 10px;
    border: 2px solid transparent;
    background: var(--bg-secondary);
    cursor: pointer;
    font-size: 16px;
    font-weight: 700;
    transition: all 0.2s;
  }
  .platform-tab.active {
    background: var(--accent-dim);
    border-color: var(--accent);
    color: var(--accent);
  }
  .server-info-box {
    padding: 12px 16px;
    border-radius: 10px;
    background: var(--bg-secondary);
    margin-bottom: 16px;
  }
  .wizard-error {
    padding: 12px;
    border-radius: 10px;
    background: rgba(229, 57, 53, 0.05);
    border: 1px solid rgba(229, 57, 53, 0.15);
    color: var(--red);
    font-size: 14px;
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
  }
  .wizard-nav {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 24px;
    padding-top: 16px;
    border-top: 1px solid var(--separator);
  }
</style>
