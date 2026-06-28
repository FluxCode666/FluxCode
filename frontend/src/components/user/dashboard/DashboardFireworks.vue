<template>
  <div class="dashboard-fireworks" aria-hidden="true">
    <span class="dashboard-fireworks__comet dashboard-fireworks__comet--left" />
    <span class="dashboard-fireworks__comet dashboard-fireworks__comet--right" />
    <span
      v-for="spark in sparks"
      :key="`left-${spark.index}`"
      class="dashboard-fireworks__spark"
      :style="sparkStyle(spark, 0)"
    />
    <span
      v-for="spark in sparks"
      :key="`right-${spark.index}`"
      class="dashboard-fireworks__spark"
      :style="sparkStyle(spark, 0.14)"
    />
  </div>
</template>

<script setup lang="ts">
interface Spark {
  index: number
  tx: number
  ty: number
  hue: number
}

const sparks: Spark[] = Array.from({ length: 18 }, (_, index) => {
  const angle = (Math.PI * 2 * index) / 18
  const radius = 46 + (index % 5) * 14
  return {
    index,
    tx: Math.round(Math.cos(angle) * radius),
    ty: Math.round(Math.sin(angle) * radius),
    hue: [42, 164, 196, 268, 330][index % 5],
  }
})

function sparkStyle(spark: Spark, delayOffset: number): Record<string, string> {
  return {
    '--tx': `${spark.tx}px`,
    '--ty': `${spark.ty}px`,
    '--delay': `${0.74 + delayOffset + spark.index * 0.012}s`,
    '--spark-color': `hsl(${spark.hue} 92% 58%)`,
  }
}
</script>

<style scoped>
.dashboard-fireworks {
  pointer-events: none;
  position: fixed;
  inset: 0;
  z-index: 70;
  overflow: hidden;
}

.dashboard-fireworks__comet {
  position: absolute;
  top: 50%;
  width: 96px;
  height: 4px;
  border-radius: 9999px;
  background: linear-gradient(90deg, transparent, #fef3c7 36%, #22d3ee 70%, #ffffff);
  box-shadow: 0 0 14px rgb(34 211 238 / 0.75), 0 0 30px rgb(251 191 36 / 0.4);
  opacity: 0;
}

.dashboard-fireworks__comet--left {
  left: 0;
  transform-origin: right center;
  animation: dashboard-fireworks-shoot-left 780ms cubic-bezier(0.14, 0.78, 0.28, 1) both;
}

.dashboard-fireworks__comet--right {
  right: 0;
  transform-origin: left center;
  animation: dashboard-fireworks-shoot-right 780ms cubic-bezier(0.14, 0.78, 0.28, 1) 90ms both;
}

.dashboard-fireworks__spark {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 7px;
  height: 7px;
  border-radius: 9999px;
  background: var(--spark-color);
  box-shadow: 0 0 12px var(--spark-color), 0 0 22px rgb(255 255 255 / 0.45);
  opacity: 0;
  transform: translate(-50%, -50%) scale(0.2);
  animation: dashboard-fireworks-burst 980ms cubic-bezier(0.16, 0.86, 0.34, 1) var(--delay) both;
}

@keyframes dashboard-fireworks-shoot-left {
  0% {
    opacity: 0;
    transform: translate(-112px, -50%) scaleX(0.45);
  }
  16% {
    opacity: 1;
  }
  82% {
    opacity: 1;
  }
  100% {
    opacity: 0;
    transform: translate(calc(50vw - 94px), -50%) scaleX(0.88);
  }
}

@keyframes dashboard-fireworks-shoot-right {
  0% {
    opacity: 0;
    transform: translate(112px, -50%) rotate(180deg) scaleX(0.45);
  }
  16% {
    opacity: 1;
  }
  82% {
    opacity: 1;
  }
  100% {
    opacity: 0;
    transform: translate(calc(-50vw + 94px), -50%) rotate(180deg) scaleX(0.88);
  }
}

@keyframes dashboard-fireworks-burst {
  0% {
    opacity: 0;
    transform: translate(-50%, -50%) scale(0.2);
  }
  12% {
    opacity: 1;
  }
  72% {
    opacity: 0.95;
    transform: translate(calc(-50% + var(--tx)), calc(-50% + var(--ty))) scale(1);
  }
  100% {
    opacity: 0;
    transform: translate(calc(-50% + var(--tx)), calc(-50% + var(--ty))) scale(0.18);
  }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-fireworks {
    display: none;
  }
}
</style>
