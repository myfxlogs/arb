import { Events } from '@wailsio/runtime'

let serviceBindings = null

async function getBindings() {
  if (serviceBindings) return serviceBindings
  try {
    const mod = await import('../../bindings/arb/desk/app.js')
    serviceBindings = mod
  } catch {
    serviceBindings = null
  }
  return serviceBindings
}

export const backend = {
  // === Unary calls (frontend → Go) ===

  async submitOrder(req) {
    const b = await getBindings()
    return b.SubmitOrder(req)
  },

  async closePosition(req) {
    const b = await getBindings()
    return b.ClosePosition(req)
  },

  async cancelOrder(req) {
    const b = await getBindings()
    return b.CancelOrder(req)
  },

  async getSignalHistory(req) {
    const b = await getBindings()
    return b.GetSignalHistory(req)
  },

  async getOrderHistory(req) {
    const b = await getBindings()
    return b.GetOrderHistory(req)
  },

  async getDailySummary(req) {
    const b = await getBindings()
    return b.GetDailySummary(req)
  },

  async getAccountSnapshots(req) {
    const b = await getBindings()
    return b.GetAccountSnapshots(req)
  },

  async getStrategyStatus(req) {
    const b = await getBindings()
    return b.GetStrategyStatus(req)
  },

  async toggleStrategy(req) {
    const b = await getBindings()
    return b.ToggleStrategy(req)
  },

  async resumeStrategy(req) {
    const b = await getBindings()
    return b.ResumeStrategy(req)
  },

  async resetCircuitBreaker(req) {
    const b = await getBindings()
    return b.ResetGlobalCircuitBreaker(req)
  },

  async getKillSwitchStatus(req) {
    const b = await getBindings()
    return b.GetKillSwitchStatus(req)
  },

  async kill(req) {
    const b = await getBindings()
    return b.Kill(req)
  },

  async resume(req) {
    const b = await getBindings()
    return b.Resume(req)
  },

  async searchBroker(req) {
    const b = await getBindings()
    return b.SearchBroker(req)
  },

  async addBroker(req) {
    const b = await getBindings()
    return b.AddBroker(req)
  },

  async removeBroker(req) {
    const b = await getBindings()
    return b.RemoveBroker(req)
  },

  async getBrokerSymbols(req) {
    const b = await getBindings()
    return b.GetBrokerSymbols(req)
  },

  async getBrokerOrderHistory(req) {
    const b = await getBindings()
    return b.GetBrokerOrderHistory(req)
  },

  async subscribeSymbols(req) {
    const b = await getBindings()
    return b.SubscribeSymbols(req)
  },

  async unsubscribeSymbols(req) {
    const b = await getBindings()
    return b.UnsubscribeSymbols(req)
  },

  async listSubscribedSymbols(req) {
    const b = await getBindings()
    return b.ListSubscribedSymbols(req)
  },

  // === Event subscriptions (Go → frontend) ===

  onSpreadMatrix(callback) {
    return Events.On('spread-matrix', (event) => callback(event.data))
  },

  onPositions(callback) {
    return Events.On('positions', (event) => callback(event.data))
  },
}
