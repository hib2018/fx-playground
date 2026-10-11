import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { createFxTerminal, supportsJspi, xtermAdapter } from 'libfx/browser'
import '@xterm/xterm/css/xterm.css'
import './style.css'

type Runtime = Awaited<ReturnType<typeof createFxTerminal>>
type Task = { id: string; title: string; description: string; status: 'todo' | 'doing' | 'done'; tag: string }
type Status = { gatewayConfigured: boolean; workspace: string; mode: string }

const app = document.querySelector<HTMLDivElement>('#app')!
app.innerHTML = `
  <aside class="sidebar">
    <a class="brand" href="/">w<span>Workbench</span></a>
    <div class="workspace-label">LOCAL WORKSPACE</div>
    <div class="workspace-name"><span class="square">W</span><div>terminal-mockup<small>Go + TypeScript</small></div></div>
    <nav aria-label="ナビゲーション"><a class="active" href="#board">▦ &nbsp; Overview</a><a href="#terminal-panel">›_ &nbsp; Agent terminal</a></nav>
    <div class="sidebar-bottom"><span class="live-dot"></span> Local sandbox<small>実機のコマンドは実行しません</small></div>
  </aside>
  <main>
    <header><span>Workspace <span class="muted">/</span> Overview</span><span class="pill">PROTOTYPE · 01</span></header>
    <section class="intro"><div class="eyebrow">YOUR LOCAL CONTROL ROOM</div><h1>少しずつ、形にしよう。</h1><p>タスクを整理して、fxと作業する。Go × TypeScript の小さな実験場。</p></section>
    <section class="metrics" aria-label="ワークスペース概要">
      <article><span>PROJECT TASKS</span><strong id="task-count">—</strong><small>Go APIから取得</small></article>
      <article><span>COMPLETED</span><strong id="done-count">—</strong><small>クリックでステータスを変更</small></article>
      <article><span>MODEL CONNECTION</span><strong class="metric-text" id="connection">確認中</strong><small>認証情報はGoサーバー側に保持</small></article>
    </section>
    <section id="board" class="board"><div class="section-heading"><h2>Project board</h2><span class="muted">タスクをクリックして進める</span></div><div class="columns"><div><h3><span class="dot gray"></span>Todo</h3><div id="todo" class="task-list"></div></div><div><h3><span class="dot amber"></span>In progress</h3><div id="doing" class="task-list"></div></div><div><h3><span class="dot green"></span>Done</h3><div id="done" class="task-list"></div></div></div></section>
    <section id="terminal-panel" class="terminal-panel"><div class="terminal-heading"><div><span class="terminal-icon">›_</span><h2>Agent terminal</h2><span class="pill">fx / WASM</span></div><div class="terminal-actions"><button id="demo" class="secondary">モックを試す</button><button id="start">fxを起動</button><button id="stop" class="secondary" disabled>停止</button></div></div>
      <div class="terminal-info"><span id="runtime-status" role="status">fxは未起動です。モック操作にはAPIキー不要です。</span><span id="support"></span></div>
      <div id="terminal" aria-label="fxターミナル"></div>
      <div class="quick-actions"><span>QUICK INPUT</span><button data-input="/help">/help</button><button data-input="/status">/status</button><button data-input="pwd">pwd</button><button data-input="ls">ls</button><button data-input="go test ./...">go test ./...</button></div>
    </section>
    <footer><span id="notice" role="status">ローカルAPIに接続中…</span><span>Go API · TypeScript UI · libfx</span></footer>
  </main>`

const el = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id)! as T
const startButton = el<HTMLButtonElement>('start')
const stopButton = el<HTMLButtonElement>('stop')
const demoButton = el<HTMLButtonElement>('demo')
const terminal = new Terminal({
  fontFamily: '"SFMono-Regular", Consolas, monospace', fontSize: 13, lineHeight: 1.4,
  cursorBlink: true, theme: { background: '#111318', foreground: '#d8dce5', cursor: '#b8d8a0', selectionBackground: '#364331' },
})
const fit = new FitAddon()
terminal.loadAddon(fit)
terminal.open(el('terminal'))
fit.fit()
terminal.writeln('\x1b[38;5;150mWorkbench / terminal-mockup\x1b[0m')
terminal.writeln('モックを試す: GoのAPIが固定のコマンド結果を返します。')
terminal.writeln('fxを起動: createFxTerminal()で本物のWASMターミナルを起動します。')
terminal.writeln('AI応答にはGoサーバーのAI_GATEWAY_API_KEYが必要です。\r\n')

let runtime: Runtime | undefined
let starting = false
let status: Status | undefined
let demoBusy = false
let disposed = false
const jspi = supportsJspi()
el('support').textContent = jspi ? 'JSPI available' : 'JSPI非対応 · 対応ブラウザが必要です'
startButton.disabled = !jspi
const observer = new ResizeObserver(() => { fit.fit(); runtime?.resize() })
observer.observe(el('terminal'))

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) throw new Error(`API ${response.status}: ${await response.text()}`)
  return response.json() as Promise<T>
}

async function loadTasks() {
  const tasks = await api<Task[]>('/api/tasks')
  for (const column of ['todo', 'doing', 'done']) el(column).replaceChildren()
  el('task-count').textContent = String(tasks.length)
  el('done-count').textContent = String(tasks.filter(task => task.status === 'done').length)
  for (const task of tasks) {
    const card = document.createElement('button')
    card.className = 'task-card'
    const tag = document.createElement('span'); tag.className = 'tag'; tag.textContent = task.tag
    const title = document.createElement('strong'); title.textContent = task.title
    const description = document.createElement('p'); description.textContent = task.description
    const bottom = document.createElement('small'); bottom.textContent = `${task.id}  →`
    card.append(tag, title, description, bottom)
    card.addEventListener('click', async () => {
      card.disabled = true
      try {
        await api(`/api/tasks/${encodeURIComponent(task.id)}/advance`, { method: 'POST' })
        await loadTasks()
      } catch (error) { report(error); card.disabled = false }
    })
    el(task.status).append(card)
  }
}

function report(error: unknown) {
  el('notice').textContent = error instanceof Error ? error.message : String(error)
}

async function refreshStatus() {
  status = await api<Status>('/api/status')
  el('connection').textContent = status.gatewayConfigured ? 'Gateway ready' : 'Not connected'
  el('notice').textContent = 'Go API connected · ワークスペースは仮想モックです'
}

async function mockCommand(command: string, signal?: AbortSignal) {
  return api<{ stdout: string; stderr: string; exitCode: number }>('/api/workspace/exec', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ command }), signal,
  })
}

// All model HTTP requests go through the local Go server. This is NOT a real credential.
// The server replaces Authorization and never exposes AI_GATEWAY_API_KEY to the browser.
const gatewayFetch: typeof fetch = async (input, init) => {
  const request = new Request(input, init)
  const url = new URL(request.url)
  if (url.origin !== 'https://ai-gateway.vercel.sh' || url.search || url.hash) {
    throw new Error('このモックはAI Gatewayのみ接続できます')
  }
  const headers = new Headers(request.headers)
  headers.delete('authorization')
  return fetch(`/api/gateway?path=${encodeURIComponent(url.pathname)}`, {
    method: request.method, headers, signal: request.signal,
    body: request.method === 'GET' || request.method === 'HEAD' ? undefined : await request.arrayBuffer(),
  })
}

// Only non-secret preferences are persisted. Conversations and OAuth tokens are intentionally ephemeral.
const configStore = {
  get(id: string) { return localStorage.getItem(`workbench.config.${id}`) },
  set(id: string, value: string) { localStorage.setItem(`workbench.config.${id}`, value) },
}

startButton.addEventListener('click', async () => {
  if (runtime || starting || disposed) return
  starting = true
  startButton.disabled = true; demoButton.disabled = true
  el('runtime-status').textContent = 'fx WASMを読み込み中…'
  terminal.clear()
  try {
    await refreshStatus()
    const created = await createFxTerminal({
      terminal: xtermAdapter(terminal), configStore, fetch: gatewayFetch,
      env: { FX_PROVIDER: 'gateway', FX_MODEL: 'openai/gpt-4.1-mini', AI_GATEWAY_API_KEY: 'local-server-proxy', FX_PERMISSION_MODE: 'auto' },
      workspace: {
        info: { version: 1, root: '/mock/workspace', cwd: '/mock/workspace', home: '/mock/workspace', gitAvailable: false, ephemeral: true },
        permission: 'allow-sandboxed',
        exec: ({ command, signal }) => mockCommand(command, signal),
      },
    })
    runtime = created
    if (disposed) { created.abort(); await created.exited; return }
    stopButton.disabled = false
    void created.exited.then(code => {
      if (runtime === created) runtime = undefined
      stopButton.disabled = true
      if (!starting) { startButton.disabled = !jspi; demoButton.disabled = false }
      el('runtime-status').textContent = `fx終了 · exit ${code}`
    })
    await created.interactive
    terminal.focus()
    el('runtime-status').textContent = status?.gatewayConfigured
      ? 'fx起動済み · モデルへの送信は課金対象です'
      : 'fx起動済み · AI応答にはGo側でAPIキーを設定してください'
  } catch (error) {
    runtime?.abort()
    if (runtime) await runtime.exited
    runtime = undefined
    report(error)
    el('runtime-status').textContent = 'fxの起動に失敗しました。下部のエラーを確認してください。'
  } finally {
    starting = false
    if (!runtime) { startButton.disabled = !jspi; demoButton.disabled = false; stopButton.disabled = true }
  }
})
stopButton.addEventListener('click', () => runtime?.abort())

async function runDemo(commands: string[]) {
  if (runtime || starting || demoBusy) return
  demoBusy = true; demoButton.disabled = true; startButton.disabled = true
  try {
    for (const command of commands) {
      terminal.writeln(`\x1b[38;5;150m[mock] $ ${command}\x1b[0m`)
      const result = await mockCommand(command)
      terminal.write((result.stdout || result.stderr).replace(/\r?\n/g, '\r\n'))
      terminal.writeln(`\x1b[90mexit ${result.exitCode}\x1b[0m\r\n`)
    }
    el('runtime-status').textContent = 'モック結果を表示しました。AI・実機シェルは使用していません。'
  } catch (error) { report(error) }
  finally { demoBusy = false; demoButton.disabled = false; startButton.disabled = !jspi }
}
demoButton.addEventListener('click', () => void runDemo(['pwd', 'ls', 'go test ./...']))
for (const button of document.querySelectorAll<HTMLButtonElement>('[data-input]')) {
  button.addEventListener('click', () => {
    const command = button.dataset.input!
    if (starting) return
    if (runtime) {
      // Slash commands are sent directly; shell examples become agent requests.
      runtime.write(`${command.startsWith('/') ? command : `モックワークスペースで「${command}」を実行してください`}\r`)
      terminal.focus()
    } else if (!command.startsWith('/')) void runDemo([command])
    else el('runtime-status').textContent = 'スラッシュコマンドを使うにはfxを起動してください。'
  })
}
window.addEventListener('pagehide', () => {
  disposed = true; observer.disconnect(); runtime?.abort(); terminal.dispose()
})
void Promise.all([refreshStatus(), loadTasks()]).catch(report)
