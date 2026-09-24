<script setup>
import { ref, onMounted, watch } from 'vue'
import { StartService, StopService, GetStatus, GetSettings, SaveSettings, SelectDirectory } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

const status = ref({ running: false, proxyPort: 2023, apiPort: 2022, uptime: '', downloadDir: '' })
const settings = ref({ proxyPort: 2023, apiPort: 2022, downloadDir: '', upstreamProxy: '', maxRunning: 3, defaultHighest: false, downloadCover: false, playDoneAudio: true })
const showSettings = ref(false)
const loading = ref(false)
const msg = ref({ text: '', type: '' })
const webUrl = ref('')

async function refresh() {
  try {
    status.value = await GetStatus()
    if (status.value.running) {
      webUrl.value = `http://127.0.0.1:${status.value.apiPort}`
    }
  } catch (e) { console.error(e) }
}

async function loadSettings() {
  try { settings.value = await GetSettings() } catch (e) { console.error(e) }
}

async function toggleService() {
  loading.value = true
  try {
    if (status.value.running) {
      await StopService()
      webUrl.value = ''
      showMsg('服务已停止', 'success')
    } else {
      await StartService()
      showMsg('服务启动中...', 'success')
      // 等服务就绪后加载 Web UI
      setTimeout(async () => {
        await refresh()
        if (status.value.running) {
          webUrl.value = `http://127.0.0.1:${status.value.apiPort}`
        }
      }, 3000)
    }
    await refresh()
  } catch (e) {
    showMsg('操作失败: ' + e, 'error')
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  try {
    await SaveSettings(settings.value)
    showMsg('设置已保存', 'success')
    showSettings.value = false
  } catch (e) { showMsg('保存失败: ' + e, 'error') }
}

async function browseDir() {
  try {
    const dir = await SelectDirectory('选择下载目录')
    if (dir) settings.value.downloadDir = dir
  } catch (e) { console.error(e) }
}

function showMsg(text, type) {
  msg.value = { text, type }
  setTimeout(() => { msg.value = { text: '', type: '' } }, 3000)
}

onMounted(() => {
  refresh()
  loadSettings()
  setInterval(refresh, 3000)
  EventsOn('status-changed', (s) => { status.value = s })
})
</script>

<template>
  <div class="app">
    <!-- 顶部控制栏 -->
    <header class="toolbar">
      <div class="toolbar-left">
        <span class="app-name">视频号下载器</span>
        <span :class="['status-tag', status.running ? 'on' : 'off']">
          {{ status.running ? '运行中' : '已停止' }}
        </span>
        <span v-if="status.running && status.uptime" class="uptime">{{ status.uptime }}</span>
      </div>
      <div class="toolbar-right">
        <div v-if="msg.text" :class="['toast', 'toast-' + msg.type]">{{ msg.text }}</div>
        <button class="btn-settings" @click="showSettings = !showSettings" title="设置">⚙️</button>
        <button
          class="btn-toggle"
          :class="status.running ? 'stop' : 'start'"
          @click="toggleService"
          :disabled="loading"
        >
          {{ loading ? '...' : (status.running ? '停止' : '启动') }}
        </button>
      </div>
    </header>

    <!-- 设置面板（下拉） -->
    <transition name="slide">
      <div v-if="showSettings" class="settings-panel">
        <div class="settings-grid">
          <div class="field">
            <label>代理端口</label>
            <input type="number" v-model.number="settings.proxyPort" :disabled="status.running" />
          </div>
          <div class="field">
            <label>API 端口</label>
            <input type="number" v-model.number="settings.apiPort" :disabled="status.running" />
          </div>
          <div class="field wide">
            <label>上游代理</label>
            <input type="text" v-model="settings.upstreamProxy" placeholder="http://127.0.0.1:7890" :disabled="status.running" />
          </div>
          <div class="field wide">
            <label>下载目录</label>
            <div class="dir-row">
              <input type="text" v-model="settings.downloadDir" placeholder="默认下载目录" />
              <button class="btn-sm" @click="browseDir">浏览</button>
            </div>
          </div>
          <div class="field">
            <label>最大同时下载</label>
            <input type="number" v-model.number="settings.maxRunning" min="1" max="10" />
          </div>
          <div class="field checkbox">
            <label><input type="checkbox" v-model="settings.defaultHighest" /> 默认最高画质</label>
          </div>
          <div class="field checkbox">
            <label><input type="checkbox" v-model="settings.downloadCover" /> 下载封面</label>
          </div>
          <div class="field checkbox">
            <label><input type="checkbox" v-model="settings.playDoneAudio" /> 完成提示音</label>
          </div>
        </div>
        <div class="settings-actions">
          <button class="btn-save" @click="saveSettings" :disabled="status.running">
            {{ status.running ? '请先停止服务' : '保存设置' }}
          </button>
        </div>
      </div>
    </transition>

    <!-- 主内容区：嵌入原工具 Web UI -->
    <main class="webview">
      <div v-if="!status.running" class="placeholder">
        <div class="placeholder-icon">📺</div>
        <div class="placeholder-title">微信视频号下载器</div>
        <div class="placeholder-hint">点击上方「启动」按钮开始服务</div>
      </div>
      <iframe
        v-else-if="webUrl"
        :src="webUrl"
        frameborder="0"
        class="webframe"
      ></iframe>
      <div v-else class="placeholder">
        <div class="placeholder-icon">⏳</div>
        <div class="placeholder-title">服务启动中...</div>
        <div class="placeholder-hint">请稍候</div>
      </div>
    </main>
  </div>
</template>

<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { width: 100%; height: 100%; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif; }
</style>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #0d0d12;
  color: #e4e4e7;
}

/* 工具栏 */
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 16px;
  background: #16161e;
  border-bottom: 1px solid #2a2a3a;
  -webkit-app-region: drag;
  flex-shrink: 0;
  height: 44px;
}

.toolbar-left, .toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  -webkit-app-region: no-drag;
}

.app-name {
  font-size: 14px;
  font-weight: 600;
  -webkit-app-region: drag;
}

.status-tag {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
}

.status-tag.on { background: rgba(34,197,94,0.15); color: #22c55e; }
.status-tag.off { background: rgba(113,113,122,0.15); color: #71717a; }

.uptime {
  font-size: 11px;
  color: #71717a;
  font-variant-numeric: tabular-nums;
}

.toast {
  font-size: 12px;
  padding: 3px 10px;
  border-radius: 6px;
}
.toast-success { background: rgba(34,197,94,0.12); color: #22c55e; }
.toast-error { background: rgba(239,68,68,0.12); color: #ef4444; }

.btn-settings {
  background: none;
  border: none;
  font-size: 18px;
  cursor: pointer;
  padding: 4px;
  border-radius: 6px;
}
.btn-settings:hover { background: #2a2a3a; }

.btn-toggle {
  padding: 5px 20px;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-toggle.start { background: #6366f1; color: #fff; }
.btn-toggle.start:hover { background: #818cf8; }
.btn-toggle.stop { background: #ef4444; color: #fff; }
.btn-toggle.stop:hover { background: #dc2626; }
.btn-toggle:disabled { opacity: 0.5; cursor: not-allowed; }

/* 设置面板 */
.settings-panel {
  background: #1a1a24;
  border-bottom: 1px solid #2a2a3a;
  padding: 16px 20px;
  flex-shrink: 0;
}

.settings-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr 1fr;
  gap: 12px;
  margin-bottom: 12px;
}

.field { display: flex; flex-direction: column; gap: 4px; }
.field.wide { grid-column: span 2; }
.field.checkbox { justify-content: flex-end; }
.field.checkbox label { display: flex; align-items: center; gap: 6px; cursor: pointer; font-size: 13px; }

.field label {
  font-size: 11px;
  color: #a1a1aa;
  font-weight: 500;
}

.field input[type="text"],
.field input[type="number"] {
  background: #222230;
  border: 1px solid #2a2a3a;
  border-radius: 6px;
  color: #e4e4e7;
  padding: 6px 10px;
  font-size: 13px;
  outline: none;
}
.field input:focus { border-color: #6366f1; }
.field input:disabled { opacity: 0.5; }

.field input[type="checkbox"] {
  width: 15px;
  height: 15px;
  accent-color: #6366f1;
}

.dir-row { display: flex; gap: 6px; }
.dir-row input { flex: 1; }

.btn-sm {
  padding: 4px 12px;
  background: #222230;
  border: 1px solid #2a2a3a;
  border-radius: 6px;
  color: #e4e4e7;
  font-size: 12px;
  cursor: pointer;
}
.btn-sm:hover { background: #2a2a3a; }

.settings-actions { display: flex; justify-content: flex-end; }

.btn-save {
  padding: 6px 20px;
  background: #6366f1;
  border: none;
  border-radius: 6px;
  color: #fff;
  font-size: 13px;
  cursor: pointer;
}
.btn-save:hover { background: #818cf8; }
.btn-save:disabled { opacity: 0.5; cursor: not-allowed; }

/* Web UI 区域 */
.webview {
  flex: 1;
  overflow: hidden;
}

.webframe {
  width: 100%;
  height: 100%;
  border: none;
}

.placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #71717a;
}

.placeholder-icon {
  font-size: 64px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.placeholder-title {
  font-size: 18px;
  color: #a1a1aa;
  margin-bottom: 8px;
}

.placeholder-hint {
  font-size: 13px;
}

/* 过渡动画 */
.slide-enter-active, .slide-leave-active {
  transition: all 0.3s ease;
}
.slide-enter-from, .slide-leave-to {
  max-height: 0;
  opacity: 0;
  padding-top: 0;
  padding-bottom: 0;
  overflow: hidden;
}
.slide-enter-to, .slide-leave-from {
  max-height: 300px;
  opacity: 1;
}
</style>
