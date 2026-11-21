<script setup lang="ts">
/**
 * 服务状态卡（深蓝 hero）：状态大字 + LIVE 胶囊 + 五指标带 + 三个操作按钮。
 * 数据全部来自父级，组件只管呈现。
 */
import { computed } from 'vue';
import { Play, Square, RotateCw, Stethoscope } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

const props = defineProps<{
  running: boolean;
  pid?: number;
  uptime: string;
  ports: string;
  monitorHealthy: boolean;
  busy: boolean;
}>();
const emit = defineEmits<{ toggle: []; reload: []; diagnose: [] }>();
const { t } = useI18n();

const DASH = '\u2014';

const facts = computed(() => [
  { k: 'PROCESS ID', v: props.pid != null && props.pid > 0 ? String(props.pid) : DASH, dim: !(props.pid && props.pid > 0) },
  { k: 'UPTIME', v: props.uptime || DASH, dim: !props.uptime },
  { k: 'LISTENING', v: props.ports || DASH, dim: !props.ports },
  { k: 'INSTANCE', v: t('nav.account'), dim: false },
  { k: 'MONITOR', v: props.monitorHealthy ? `\u25cf ${t('monitor.healthy')}` : DASH, ok: props.monitorHealthy, dim: !props.monitorHealthy },
]);

const subtitle = computed(() =>
  props.running
    ? `${t('status.subRunning')}${props.monitorHealthy ? ' \u00b7 Monitor ' + t('monitor.statusRunning') : ''}`
    : t('status.subStopped'),
);
</script>

<template>
  <section class="hero" :class="{ 'hero--off': !running }">
    <div class="hero__row">
      <div class="hero__main">
        <div class="hero__kicker"><span class="hero__kicker-line" />SERVICE STATUS</div>
        <h2 class="hero__title">
          {{ running ? t('status.running') : t('status.stopped') }}
          <span v-if="running" class="live-pill">LIVE</span>
        </h2>
        <p class="hero__sub">{{ subtitle }}</p>
        <div class="hero__actions">
          <button class="hbtn hbtn--main" :class="{ 'hbtn--stop': running }" :disabled="busy" @click="emit('toggle')">
            <component :is="running ? Square : Play" :size="13" />
            <span>{{ running ? t('overview.stop') : t('overview.start') }}</span>
          </button>
          <button class="hbtn" :disabled="busy" @click="emit('reload')">
            <RotateCw :size="13" /><span>{{ t('overview.reload') }}</span>
          </button>
          <button class="hbtn" @click="emit('diagnose')">
            <Stethoscope :size="13" /><span>{{ t('overview.diagnose') }}</span>
          </button>
        </div>
      </div>

      <!-- 雷达：运行时扫描线旋转 + 回波闪烁；停止时静止发灰。纯装饰。 -->
      <div class="radar" aria-hidden="true">
        <span class="radar__ring radar__ring--1" />
        <span class="radar__ring radar__ring--2" />
        <span class="radar__ring radar__ring--3" />
        <span class="radar__cross" />
        <span v-if="running" class="radar__sweep" />
        <span v-if="running" class="radar__blip radar__blip--a" />
        <span v-if="running" class="radar__blip radar__blip--b" />
        <span v-if="running" class="radar__blip radar__blip--c" />
        <span class="radar__core" />
      </div>
    </div>

    <div class="hero__facts">
      <div v-for="(f, i) in facts" :key="f.k" class="hf" :style="{ '--i': i }">
        <div class="hf__k">{{ f.k }}</div>
        <div class="hf__v" :class="{ 'is-dim': f.dim, 'is-ok': f.ok }">{{ f.v }}</div>
      </div>
    </div>
  </section>
</template>
