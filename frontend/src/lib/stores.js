import { writable } from 'svelte/store'
import { backend } from './backend.js'

export const spreadMatrix = writable({ rows: [] })
export const positions = writable({ broker_positions: [] })
export const activeTab = writable('matrix')

export const strategyStatus = writable({ items: [] })
export const killSwitchActive = writable(false)
export const brokers = writable([])

export function initStreams() {
  backend.onSpreadMatrix((data) => {
    spreadMatrix.set(data)
  })
  backend.onPositions((data) => {
    positions.set(data)
  })
}
