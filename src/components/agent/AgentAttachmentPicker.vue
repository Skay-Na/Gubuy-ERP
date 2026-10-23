<script setup>
import { ref } from 'vue'
import { AGENT_ATTACHMENT_EXTENSIONS } from '../../utils/agentAttachments'

defineProps({
  disabled: { type: Boolean, default: false }
})

const emit = defineEmits(['select'])
const menuOpen = ref(false)
const fileInput = ref(null)
const folderInput = ref(null)
const acceptedTypes = AGENT_ATTACHMENT_EXTENSIONS.join(',')

function chooseFiles(kind) {
  menuOpen.value = false
  const input = kind === 'folder' ? folderInput.value : fileInput.value
  input?.click()
}

function collectFiles(event) {
  const files = Array.from(event.target.files || [])
  event.target.value = ''
  if (files.length) emit('select', files)
}
</script>

<template>
  <div class="agent-attachment-picker" @keydown.esc="menuOpen = false">
    <button
      type="button"
      class="agent-attachment-trigger"
      aria-label="添加文件或文件夹"
      aria-haspopup="menu"
      :aria-expanded="menuOpen"
      :disabled="disabled"
      @click="menuOpen = !menuOpen"
    >
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>
    </button>
    <div v-if="menuOpen" class="agent-attachment-menu" role="menu">
      <button type="button" role="menuitem" @click="chooseFiles('files')">选择文件</button>
      <button type="button" role="menuitem" @click="chooseFiles('folder')">选择文件夹</button>
    </div>
    <input
      ref="fileInput"
      class="agent-attachment-native-input"
      type="file"
      multiple
      :accept="acceptedTypes"
      @change="collectFiles"
    />
    <input
      ref="folderInput"
      class="agent-attachment-native-input"
      type="file"
      multiple
      webkitdirectory
      directory
      :accept="acceptedTypes"
      @change="collectFiles"
    />
  </div>
</template>

<style scoped>
.agent-attachment-picker { position: relative; flex: 0 0 auto; }
.agent-attachment-trigger { display: grid; width: 42px; height: 42px; place-items: center; border: 0; border-radius: 999px; background: #f1f5f9; color: #334155; transition: background .16s ease, color .16s ease; }
.agent-attachment-trigger:hover { background: #e2e8f0; color: #0f172a; }
.agent-attachment-trigger:disabled { cursor: not-allowed; opacity: .45; }
.agent-attachment-trigger svg { width: 22px; height: 22px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-width: 2; }
.agent-attachment-menu { position: absolute; bottom: calc(100% + 10px); left: 0; z-index: 20; display: grid; width: 184px; gap: 3px; padding: 6px; border: 1px solid #e2e8f0; border-radius: 14px; background: #fff; box-shadow: 0 14px 32px rgba(15, 23, 42, .15); }
.agent-attachment-menu button { padding: 10px 12px; border: 0; border-radius: 9px; background: transparent; color: #334155; text-align: left; font-size: 13px; }
.agent-attachment-menu button:hover { background: #f1f5f9; }
.agent-attachment-native-input { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; clip-path: inset(50%); }
</style>
