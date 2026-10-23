<script setup>
import AIChatAssistant from '../components/AIChatAssistant.vue'
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-5 p-4 pb-20 md:p-8">
    <header>
      <p class="text-sm font-medium text-blue-700">AI 店长</p>
      <h1 class="mt-1 text-3xl font-bold text-slate-900">经营助手</h1>
      <p class="mt-2 text-sm text-slate-500">直接描述你想了解或办理的事情。</p>
    </header>
    <AIChatAssistant />
  </div>
</template>
