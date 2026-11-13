<script setup lang="ts">
import TitleBar from './TitleBar.vue';
import Sidebar from './Sidebar.vue';
</script>

<template>
  <div class="window">
    <TitleBar />
    <div class="app">
      <Sidebar />
      <main class="main">
        <slot />
      </main>
    </div>
  </div>
</template>
