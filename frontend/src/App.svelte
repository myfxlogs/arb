<script>
  import { activeTab } from './lib/stores.js'
  import Matrix from './tabs/Matrix.svelte'
  import Positions from './tabs/Positions.svelte'
  import Trading from './tabs/Trading.svelte'
  import History from './tabs/History.svelte'
  import Admin from './tabs/Admin.svelte'

  const tabs = [
    { id: 'matrix', label: '价差矩阵' },
    { id: 'positions', label: '持仓' },
    { id: 'trading', label: '交易' },
    { id: 'history', label: '历史' },
    { id: 'admin', label: '管理' },
  ]
</script>

<div class="nav-bar">
  {#each tabs as tab}
    <div
      class="nav-item"
      class:active={$activeTab === tab.id}
      role="button"
      tabindex="0"
      onclick={() => activeTab.set(tab.id)}
      onkeydown={(e) => e.key === 'Enter' && activeTab.set(tab.id)}
    >
      {tab.label}
    </div>
  {/each}
</div>

<div class="tab-content">
  {#if $activeTab === 'matrix'}
    <Matrix />
  {:else if $activeTab === 'positions'}
    <Positions />
  {:else if $activeTab === 'trading'}
    <Trading />
  {:else if $activeTab === 'history'}
    <History />
  {:else if $activeTab === 'admin'}
    <Admin />
  {/if}
</div>
