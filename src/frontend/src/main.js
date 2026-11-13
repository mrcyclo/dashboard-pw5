import '@fontsource/noto-sans/400.css'
import '@fontsource/noto-sans/700.css'
import './app.css'
import { mount } from 'svelte'
import App from './App.svelte'

export default mount(App, { target: document.getElementById('app') })
