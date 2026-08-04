<script>
  let { label, value, color } = $props()
</script>

<div class="card stat-card">
  <div class="stat-label">{label}</div>
  <div class="stat-value" style={color ? `color: ${color}` : ''}>{value}</div>
</div>

<style>
  .stat-card {
    text-align: center;
    padding: 24px 16px;
  }
  .stat-label {
    font-size: 13px;
    color: var(--text-dim);
    margin-bottom: 8px;
  }
  .stat-value {
    font-size: 28px;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
</style>
