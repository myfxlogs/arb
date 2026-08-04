import { mount } from 'svelte'
import App from './App.svelte'
import { initStreams } from './lib/stores.js'

initStreams()

const app = mount(App, {
  target: document.getElementById('app'),
})

export default app
