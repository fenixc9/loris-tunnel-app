import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import vm from 'node:vm'
import { reactive } from 'vue'

const source = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function setup(items) {
  const request = deferred()
  const poll = deferred()
  const calls = []
  const context = vm.createContext({
    tunnelActionRevision: 0, pendingTunnelActions: reactive(new Map()), tunnels: { value: items },
    configMessage: { value: '' }, tunnelErrorAiDebugStates: {},
    initialStateLoading: { value: true },
    normalizeTunnelFromBackend: (item) => item,
    jumpers: { value: [] }, tunnelGroups: { value: [] },
    GetState: () => poll.promise,
    ToggleTunnel: (id) => { calls.push(id); return request.promise },
    patchTunnelLocal: (id, updated) => {
      context.tunnels.value = context.tunnels.value.map((item) => item.id === id ? { ...item, ...updated } : item)
    },
    t: (key) => key,
    logEvent: () => {}, errorMessage: (err) => err.message
  })
  for (const [start, end] of [
    ['async function toggleTunnel(', '\nfunction openActionDialog('],
    ['async function loadStateFromBackend(', '\nfunction syncStateSilently(']
  ]) {
    vm.runInContext(source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start))), context)
  }
  return { context, request, poll, calls }
}

for (const status of ['running', 'stopped', 'error']) {
  test(`${status}: immediate feedback, duplicate guard, successful completion`, async () => {
    const tunnel = { id: 1, name: 'test', status, lastError: 'previous', latencyMs: 200 }
    const { context: c, request, calls } = setup([tunnel])
    const done = c.toggleTunnel(tunnel)
    assert.equal(c.pendingTunnelActions.get(1), status === 'running' ? 'stop' : 'start')
    assert.equal(c.tunnels.value[0].status, 'busy')
    await c.toggleTunnel(tunnel)
    assert.equal(calls.length, 1)
    request.resolve({ ...tunnel, status: status === 'running' ? 'stopped' : 'running', lastError: '' })
    await done
    assert.equal(c.pendingTunnelActions.size, 0)
    assert.equal(c.tunnels.value[0].status, status === 'running' ? 'stopped' : 'running')
  })
  test(`${status}: failure preserves original state and clears feedback`, async () => {
    const tunnel = { id: 1, name: 'test', status, lastError: 'previous', latencyMs: 200 }
    const { context: c, request } = setup([tunnel])
    const done = c.toggleTunnel(tunnel)
    request.reject(new Error('unavailable'))
    await done
    assert.equal(c.pendingTunnelActions.size, 0)
    assert.deepEqual(c.tunnels.value[0], tunnel)
  })
}

test('polling preserves pending state and cannot overwrite a completed action', async () => {
  const tunnel = { id: 1, name: 'test', status: 'running' }
  const { context: c, request, poll } = setup([tunnel])
  const done = c.toggleTunnel(tunnel)
  const syncing = c.loadStateFromBackend()
  request.resolve({ ...tunnel, status: 'stopped' })
  await done
  poll.resolve({ tunnels: [tunnel] })
  await syncing
  assert.equal(c.tunnels.value[0].status, 'stopped')
  assert.equal(c.pendingTunnelActions.size, 0)
})

test('polling during a request keeps its busy status and loading record', async () => {
  const tunnel = { id: 1, name: 'test', status: 'running' }
  const { context: c, request, poll } = setup([tunnel])
  const done = c.toggleTunnel(tunnel)
  const syncing = c.loadStateFromBackend()
  poll.resolve({ tunnels: [{ ...tunnel, status: 'stopped' }] })
  await syncing
  assert.equal(c.tunnels.value[0].status, 'busy')
  assert.equal(c.pendingTunnelActions.get(1), 'stop')
  request.resolve({ ...tunnel, status: 'stopped' })
  await done
})

test('a fourth tunnel can start without an active license', async () => {
  const running = [1, 2, 3].map((id) => ({ id, name: `t${id}`, status: 'running' }))
  const target = { id: 4, name: 't4', status: 'stopped' }
  const { context: c, calls, poll, request } = setup([...running, target])
  const syncing = c.loadStateFromBackend()
  poll.resolve({ tunnels: [...running, target], activeSlots: 3, activeSlotLimit: 3 })
  await syncing
  const done = c.toggleTunnel(target)
  assert.deepEqual(calls, [4])
  request.resolve({ ...target, status: 'running' })
  await done
  assert.equal(c.tunnels.value.filter(({ status }) => status === 'running').length, 4)
})

test('poll begun before a toggle cannot overwrite its completed result', async () => {
  const tunnel = { id: 1, name: 'test', status: 'running' }
  const { context: c, request, poll } = setup([tunnel])
  const syncing = c.loadStateFromBackend()
  const done = c.toggleTunnel(tunnel)
  request.resolve({ ...tunnel, status: 'stopped' })
  await done
  poll.resolve({ tunnels: [tunnel] })
  await syncing
  assert.equal(c.tunnels.value[0].status, 'stopped')
})
