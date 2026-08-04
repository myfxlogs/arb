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

  // Wizard state
  let showWizard = $state(false)
  let wizardStep = $state(0)
  let wizardPlatform = $state('MT5')
  let wizardKeyword = $state('')
  let wizardStatus = $state('')
  let searchResult = $state(null)
  let wizardCompanies = $state([])
  let wizardCompany = $state('')
  let wizardServers = $state([])
  let wizardServer = $state('')
  let wizardUser = $state('')
  let wizardPassword = $state('')
  let wizardName = $state('')
  let serverFilter = $state('')

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

  async function wizardSearch() {
    wizardStatus = '搜索中...'
    wizardCompanies = []
    wizardCompany = ''
    const platform = wizardPlatform === 'MT5' ? 1 : 0
    try {
      const reply = await backend.searchBroker({ company: wizardKeyword, platform })
      if (reply.error) {
        wizardStatus = reply.error
        return
      }
      searchResult = reply
      if (!reply.companies || reply.companies.length === 0) {
        wizardStatus = '未找到匹配的经纪商'
        return
      }
      wizardCompanies = reply.companies.map(c => `${c.company_name} (${c.servers.length}个服务器)`)
      wizardCompany = wizardCompanies[0]
      wizardStatus = `找到 ${reply.companies.length} 个经纪商`
    } catch (err) {
      wizardStatus = `搜索失败: ${err}`
    }
  }

  function onCompanyChange(selected) {
    if (!searchResult) return
    for (const c of searchResult.companies) {
      const label = `${c.company_name} (${c.servers.length}个服务器)`
      if (label !== selected) continue
      const servers = c.servers.map(s => s.name).sort()
      wizardServers = servers
      if (servers.length > 0) wizardServer = servers[0]
      return
    }
  }

  function wizardNext() {
    if (wizardStep === 0) {
      if (!searchResult || wizardCompanies.length === 0) {
        alert('请先搜索并选择经纪商公司')
        return
      }
      if (!wizardCompany) {
        alert('请选择一个公司')
        return
      }
      onCompanyChange(wizardCompany)
    } else if (wizardStep === 1) {
      if (!wizardServer) {
        alert('请选择一个服务器')
        return
      }
    }
    wizardStep++
  }

  function wizardPrev() {
    if (wizardStep > 0) wizardStep--
  }

  async function wizardSubmit() {
    if (!wizardUser) { alert('请输入交易账号'); return }
    if (!wizardPassword) { alert('请输入密码'); return }

    const platform = wizardPlatform === 'MT5' ? 1 : 0
    let companyName = ''
    for (const c of searchResult.companies) {
      const label = `${c.company_name} (${c.servers.length}个服务器)`
      if (label === wizardCompany) { companyName = c.company_name; break }
    }

    let host = ''
    let port = 443
    let serverName = wizardServer
    for (const c of searchResult.companies) {
      if (c.company_name !== companyName) continue
      for (const s of c.servers) {
        if (s.name === wizardServer) {
          if (s.access) {
            const parts = s.access.split(':')
            host = parts[0]
            if (parts.length > 1) port = parseInt(parts[1]) || 443
          }
          break
        }
      }
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
      if (!reply.success) { alert(reply.error); return }
      alert(`经纪商 ${name} 添加成功`)
      showWizard = false
      resetWizard()
      refreshBrokers()
    } catch (err) {
      alert(`错误: ${err}`)
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
    wizardUser = ''
    wizardPassword = ''
    wizardName = ''
    serverFilter = ''
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

      <div class="wizard-step-title">
        步骤 {wizardStep + 1}/3 —
        {wizardStep === 0 ? '搜索经纪商' : wizardStep === 1 ? '选择服务器' : '输入账号'}
      </div>

      {#if wizardStep === 0}
        <div class="form-group">
          <span class="form-label">选择平台</span>
          <select class="form-select" bind:value={wizardPlatform}>
            <option value="MT4">MT4</option>
            <option value="MT5">MT5</option>
          </select>
        </div>
        <div class="form-group">
          <span class="form-label">经纪商关键词</span>
          <input class="form-input" bind:value={wizardKeyword} placeholder="如 OctaFX, RoboForex" />
        </div>
        <button class="btn btn-primary" onclick={wizardSearch}>搜索</button>
        {#if wizardStatus}
          <div style="margin-top: 8px; font-size: 13px; color: var(--text-dim);">{wizardStatus}</div>
        {/if}
        {#if wizardCompanies.length > 0}
          <div class="form-group" style="margin-top: 12px;">
            <span class="form-label">选择公司</span>
            <select class="form-select" bind:value={wizardCompany} onchange={(e) => onCompanyChange(e.target.value)}>
              {#each wizardCompanies as c}
                <option value={c}>{c}</option>
              {/each}
            </select>
          </div>
        {/if}
      {:else if wizardStep === 1}
        <div class="form-group">
          <span class="form-label">选择服务器 ({wizardServers.length} 个)</span>
          {#if wizardServers.length > 10}
            <input class="form-input" bind:value={serverFilter} placeholder="过滤服务器名称..." style="margin-bottom: 8px;" />
          {/if}
          <select class="form-select" bind:value={wizardServer}>
            {#each wizardServers.filter(s => !serverFilter || s.toLowerCase().includes(serverFilter.toLowerCase())) as s}
              <option value={s}>{s}</option>
            {/each}
          </select>
        </div>
        <div style="margin-top: 8px; font-size: 13px; color: var(--text-dim);">已选择: {wizardServer}</div>
      {:else}
        <div class="form-group">
          <span class="form-label">交易账号</span>
          <input class="form-input" bind:value={wizardUser} placeholder="交易账号" />
        </div>
        <div class="form-group">
          <span class="form-label">密码</span>
          <input class="form-input" type="text" bind:value={wizardPassword} placeholder="密码（明文显示）" />
        </div>
        <div class="form-group">
          <span class="form-label">自定义名称（可选）</span>
          <input class="form-input" bind:value={wizardName} placeholder="留空则用公司名" />
        </div>
      {/if}

      <div class="wizard-nav">
        {#if wizardStep > 0}
          <button class="btn btn-secondary" onclick={wizardPrev}>上一步</button>
        {/if}
        {#if wizardStep < 2}
          <button class="btn btn-primary" onclick={wizardNext}>下一步</button>
        {:else}
          <button class="btn btn-primary" onclick={wizardSubmit}>确认添加</button>
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
  .wizard-step-title {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 20px;
    color: var(--accent);
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
