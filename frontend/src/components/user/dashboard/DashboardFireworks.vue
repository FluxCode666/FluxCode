<template>
  <div class="dashboard-fireworks" aria-hidden="true">
    <span
      class="dashboard-fireworks__emitter dashboard-fireworks__emitter--left"
    >
      <span class="dashboard-fireworks__flash" />
    </span>
    <span
      class="dashboard-fireworks__emitter dashboard-fireworks__emitter--right"
    >
      <span class="dashboard-fireworks__flash" />
    </span>
    <span
      v-for="piece in confetti"
      :key="`left-${piece.index}`"
      :class="[
        'dashboard-fireworks__confetti',
        'dashboard-fireworks__confetti--left',
        `dashboard-fireworks__confetti--${piece.shape}`,
      ]"
      :style="confettiStyle(piece, 'left')"
    />
    <span
      v-for="piece in confetti"
      :key="`right-${piece.index}`"
      :class="[
        'dashboard-fireworks__confetti',
        'dashboard-fireworks__confetti--right',
        `dashboard-fireworks__confetti--${piece.shape}`,
      ]"
      :style="confettiStyle(piece, 'right')"
    />
  </div>
</template>

<script setup lang="ts">
type FireworkSide = 'left' | 'right'
type ConfettiShape = 'strip' | 'square' | 'dot'

interface ConfettiPiece {
  index: number
  originY: number
  travelX: number
  travelY: number
  fall: number
  delay: number
  width: number
  height: number
  spin: number
  color: string
  shape: ConfettiShape
}

const confettiColors = ['#dc2626', '#ef4444', '#ffffff', '#fee2e2', '#f59e0b']

const confetti: ConfettiPiece[] = Array.from({ length: 36 }, (_, index) => {
  const lane = index % 11
  const shape: ConfettiShape = index % 7 === 0 ? 'dot' : index % 5 === 0 ? 'square' : 'strip'

  return {
    index,
    originY: ((index * 13) % 30) - 15,
    travelX: 240 + ((index * 53) % 430),
    travelY: (lane - 5) * 17 + ((index * 19) % 26) - 13,
    fall: 72 + ((index * 23) % 78),
    delay: (index % 9) * 0.026 + Math.floor(index / 9) * 0.045,
    width: index % 4 === 0 ? 7 : 5,
    height: index % 3 === 0 ? 16 : 11,
    spin: 190 + ((index * 37) % 410),
    color: confettiColors[index % confettiColors.length],
    shape,
  }
})

function confettiStyle(piece: ConfettiPiece, side: FireworkSide): Record<string, string> {
  const direction = side === 'left' ? 1 : -1
  const sideDelay = side === 'left' ? 0 : 0.08
  const width = piece.shape === 'dot' ? 7 : piece.shape === 'square' ? 8 : piece.width
  const height = piece.shape === 'dot' ? 7 : piece.shape === 'square' ? 8 : piece.height

  return {
    '--origin-y': `${piece.originY}px`,
    '--travel-x': `${direction * piece.travelX}px`,
    '--travel-y': `${piece.travelY}px`,
    '--end-x': `${direction * (piece.travelX + 42)}px`,
    '--end-y': `${piece.travelY + piece.fall}px`,
    '--delay': `${0.08 + sideDelay + piece.delay}s`,
    '--confetti-color': piece.color,
    '--confetti-width': `${width}px`,
    '--confetti-height': `${height}px`,
    '--spin': `${direction * piece.spin}deg`,
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

.dashboard-fireworks__emitter {
  position: absolute;
  top: 50%;
  width: 82px;
  height: 118px;
  opacity: 0;
  background:
    radial-gradient(ellipse at 8% 50%, rgb(254 242 242 / 0.95), rgb(239 68 68 / 0.46) 28%, transparent 68%),
    radial-gradient(ellipse at 22% 50%, rgb(251 191 36 / 0.28), transparent 58%);
  filter: drop-shadow(0 0 18px rgb(239 68 68 / 0.36));
  animation: dashboard-fireworks-emitter 1700ms ease-out both;
}

.dashboard-fireworks__emitter--left {
  left: -34px;
  transform: translateY(-50%);
}

.dashboard-fireworks__emitter--right {
  right: -34px;
  transform: translateY(-50%) scaleX(-1);
}

.dashboard-fireworks__flash {
  position: absolute;
  top: 50%;
  left: 28px;
  width: 48px;
  height: 48px;
  border-radius: 9999px;
  background: radial-gradient(circle, #ffffff 0%, #fef3c7 28%, rgb(239 68 68 / 0.66) 52%, transparent 72%);
  box-shadow: 0 0 20px rgb(255 255 255 / 0.8), 0 0 34px rgb(239 68 68 / 0.6);
  transform: translateY(-50%) scale(0.24);
  animation: dashboard-fireworks-flash 880ms ease-out both;
}

.dashboard-fireworks__confetti {
  position: absolute;
  top: 50%;
  left: 22px;
  width: var(--confetti-width);
  height: var(--confetti-height);
  border-radius: 2px;
  background: var(--confetti-color);
  box-shadow: 0 0 9px rgb(255 255 255 / 0.42);
  opacity: 0;
  transform: translate(0, calc(-50% + var(--origin-y))) rotate(0deg) scale(0.45);
  animation: dashboard-fireworks-confetti 1900ms cubic-bezier(0.12, 0.78, 0.28, 1) var(--delay) both;
  will-change: transform, opacity;
}

.dashboard-fireworks__confetti--right {
  right: 22px;
  left: auto;
}

.dashboard-fireworks__confetti--dot {
  border-radius: 9999px;
}

.dashboard-fireworks__confetti--square {
  border-radius: 1px;
}

@keyframes dashboard-fireworks-emitter {
  0% {
    opacity: 0;
  }
  8%,
  64% {
    opacity: 1;
  }
  100% {
    opacity: 0;
  }
}

@keyframes dashboard-fireworks-flash {
  0% {
    opacity: 0;
    transform: translateY(-50%) scale(0.18);
  }
  18% {
    opacity: 1;
    transform: translateY(-50%) scale(1);
  }
  100% {
    opacity: 0;
    transform: translateY(-50%) scale(1.55);
  }
}

@keyframes dashboard-fireworks-confetti {
  0% {
    opacity: 0;
    transform: translate(0, calc(-50% + var(--origin-y))) rotate(0deg) scale(0.45);
  }
  10% {
    opacity: 1;
  }
  58% {
    opacity: 0.96;
    transform: translate(var(--travel-x), calc(-50% + var(--origin-y) + var(--travel-y))) rotate(var(--spin)) scale(1);
  }
  100% {
    opacity: 0;
    transform: translate(var(--end-x), calc(-50% + var(--origin-y) + var(--end-y))) rotate(var(--spin)) scale(0.82);
  }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-fireworks {
    display: none;
  }
}

@media (max-width: 767px) {
  .dashboard-fireworks {
    display: none;
  }
}
</style>
