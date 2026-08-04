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
  return callByName('arb/desk.App.SubmitOrder', req)
}
export function ClosePosition(req) {
  return callByName('arb/desk.App.ClosePosition', req)
}
export function CancelOrder(req) {
  return callByName('arb/desk.App.CancelOrder', req)
}
export function GetSignalHistory(req) {
  return callByName('arb/desk.App.GetSignalHistory', req)
}
export function GetOrderHistory(req) {
  return callByName('arb/desk.App.GetOrderHistory', req)
}
export function GetDailySummary(req) {
  return callByName('arb/desk.App.GetDailySummary', req)
}
export function GetAccountSnapshots(req) {
  return callByName('arb/desk.App.GetAccountSnapshots', req)
}
export function GetStrategyStatus(req) {
  return callByName('arb/desk.App.GetStrategyStatus', req)
}
export function ToggleStrategy(req) {
  return callByName('arb/desk.App.ToggleStrategy', req)
}
export function ResumeStrategy(req) {
  return callByName('arb/desk.App.ResumeStrategy', req)
}
export function ResetGlobalCircuitBreaker(req) {
  return callByName('arb/desk.App.ResetGlobalCircuitBreaker', req)
}
export function GetKillSwitchStatus(req) {
  return callByName('arb/desk.App.GetKillSwitchStatus', req)
}
export function Kill(req) {
  return callByName('arb/desk.App.Kill', req)
}
export function Resume(req) {
  return callByName('arb/desk.App.Resume', req)
}
export function SearchBroker(req) {
  return callByName('arb/desk.App.SearchBroker', req)
}
export function AddBroker(req) {
  return callByName('arb/desk.App.AddBroker', req)
}
export function RemoveBroker(req) {
  return callByName('arb/desk.App.RemoveBroker', req)
}
export function SubscribeSymbols(req) {
  return callByName('arb/desk.App.SubscribeSymbols', req)
}
export function UnsubscribeSymbols(req) {
  return callByName('arb/desk.App.UnsubscribeSymbols', req)
}
export function ListSubscribedSymbols(req) {
  return callByName('arb/desk.App.ListSubscribedSymbols', req)
}
