import { GetState } from '../../bindings/claudedash/internal/desk/app'

const POLL_MS = 2000
const TOAST_MS = 3500

export const app = $state({
  // null until the first answer from the backend, so pages can tell "loading" from "empty".
  state: null,
  // Seconds, ticking locally so countdowns move between polls.
  now: Math.floor(Date.now() / 1000),
  toasts: [],
})

let nextToastId = 1
// Bumped when a change starts and again when it settles. A poll that began
// before the change settled may have read the old values, so its answer is dropped.
let generation = 0

export async function refresh() {
  const startedAt = generation
  try {
    const state = await GetState()
    if (startedAt !== generation) return
    app.state = state
  } catch {
    // The window can outlive a backend call during shutdown; the next poll retries.
  }
}

export function init() {
  refresh()
  const poll = setInterval(refresh, POLL_MS)
  const tick = setInterval(() => (app.now = Math.floor(Date.now() / 1000)), 1000)
  return () => {
    clearInterval(poll)
    clearInterval(tick)
  }
}

export function toast(type, message) {
  const id = nextToastId++
  app.toasts = [...app.toasts, { id, type, message }]
  setTimeout(() => (app.toasts = app.toasts.filter((t) => t.id !== id)), TOAST_MS)
}

// Wails rejects with an Error whose message is either plain text or a JSON object carrying it.
function errorText(error) {
  const raw = error?.message ?? String(error)
  try {
    const parsed = JSON.parse(raw)
    return parsed.message ?? raw
  } catch {
    return raw
  }
}

// Runs a backend call that returns the new state. Shows a toast on failure,
// and on success when a message is given. Resolves to whether it worked.
export async function call(fn, okMessage) {
  generation++
  try {
    const state = await fn()
    generation++
    if (state && typeof state === 'object' && 'settings' in state) {
      app.state = state
    }
    if (okMessage) {
      toast('success', okMessage)
    }
    return true
  } catch (error) {
    generation++
    toast('error', errorText(error))
    refresh()
    return false
  }
}

// Base URL of the HTTP server the Kindle talks to, reached from this machine.
export function localUrl(path) {
  const port = app.state?.settings?.port ?? 8377
  return `http://127.0.0.1:${port}${path}`
}
