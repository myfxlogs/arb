// Auto-generated Wails v3 bindings for arb/desk.App
// In production, regenerate with: wails3 generate bindings

const callByName = async (name, ...args) => {
  try {
    const { Call } = await import('/wails/runtime.js')
    return Call.ByName(name, ...args)
  } catch {
    // Fallback for dev/preview without Wails runtime
    console.warn(`Wails binding "${name}" called without runtime`)
    return null
  }
}

export function SubmitOrder(req) {
  return callByName('desk.App.SubmitOrder', req)
}
export function ClosePosition(req) {
  return callByName('desk.App.ClosePosition', req)
}
export function CancelOrder(req) {
  return callByName('desk.App.CancelOrder', req)
}
export function GetSignalHistory(req) {
  return callByName('desk.App.GetSignalHistory', req)
}
export function GetOrderHistory(req) {
  return callByName('desk.App.GetOrderHistory', req)
}
export function GetDailySummary(req) {
  return callByName('desk.App.GetDailySummary', req)
}
export function GetAccountSnapshots(req) {
  return callByName('desk.App.GetAccountSnapshots', req)
}
export function GetStrategyStatus(req) {
  return callByName('desk.App.GetStrategyStatus', req)
}
export function ToggleStrategy(req) {
  return callByName('desk.App.ToggleStrategy', req)
}
export function ResumeStrategy(req) {
  return callByName('desk.App.ResumeStrategy', req)
}
export function ResetGlobalCircuitBreaker(req) {
  return callByName('desk.App.ResetGlobalCircuitBreaker', req)
}
export function GetKillSwitchStatus(req) {
  return callByName('desk.App.GetKillSwitchStatus', req)
}
export function Kill(req) {
  return callByName('desk.App.Kill', req)
}
export function Resume(req) {
  return callByName('desk.App.Resume', req)
}
export function SearchBroker(req) {
  return callByName('desk.App.SearchBroker', req)
}
export function AddBroker(req) {
  return callByName('desk.App.AddBroker', req)
}
export function RemoveBroker(req) {
  return callByName('desk.App.RemoveBroker', req)
}
export function SubscribeSymbols(req) {
  return callByName('desk.App.SubscribeSymbols', req)
}
export function UnsubscribeSymbols(req) {
  return callByName('desk.App.UnsubscribeSymbols', req)
}
export function ListSubscribedSymbols(req) {
  return callByName('desk.App.ListSubscribedSymbols', req)
}
