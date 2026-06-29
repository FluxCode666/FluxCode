<template>
  <div class="dashboard-fireworks" aria-hidden="true">
    <span
      v-for="piece in pieces"
      :key="piece.id"
      class="dashboard-fireworks__piece"
      :class="[
        `dashboard-fireworks__piece--${piece.side}`,
        `dashboard-fireworks__piece--${piece.shape}`,
      ]"
      :style="piece.style"
    />
  </div>
</template>

<script setup lang="ts">
type FireworkSide = 'left' | 'right'
type FireworkShape = 'strip' | 'shard' | 'triangle' | 'dot' | 'tiny'

interface FireworkPiece {
  id: string
  side: FireworkSide
  shape: FireworkShape
  style: Record<string, string>
}

const PIECES_PER_SIDE = 130
const palette = ['#dc2626', '#ef4444', '#b91c1c', '#ffffff', '#fee2e2', '#f59e0b']
const shapes: FireworkShape[] = ['strip', 'strip', 'strip', 'shard', 'triangle', 'dot', 'tiny']
const samples = [
  ['05', 0.05],
  ['12', 0.12],
  ['22', 0.22],
  ['36', 0.36],
  ['52', 0.52],
  ['72', 0.72],
  ['90', 0.9],
  ['100', 1],
] as const

function seededRandom(seed: number): () => number {
  let state = seed
  return () => {
    state += 0x6d2b79f5
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

function randRange(random: () => number, min: number, max: number): number {
  return random() * (max - min) + min
}

function pick<T>(random: () => number, list: T[]): T {
  return list[Math.floor(random() * list.length) % list.length]
}

function projectilePoint(
  v0x: number,
  v0y: number,
  gravity: number,
  time: number,
  wind: number
): { x: number; y: number } {
  return {
    x: v0x * time + 0.5 * wind * time * time,
    y: v0y * time + 0.5 * gravity * time * time,
  }
}

function createPiece(side: FireworkSide, index: number, total: number): FireworkPiece {
  const random = seededRandom((side === 'left' ? 0x5f3759df : 0x9e3779b9) + index * 101)
  const shape = pick(random, shapes)
  const direction = side === 'left' ? 1 : -1
  const launchAngle = randRange(random, 56, 66) * Math.PI / 180
  const speed = randRange(random, 760, 1080)
  const v0x = Math.cos(launchAngle) * speed * direction
  const v0y = -Math.sin(launchAngle) * speed
  const gravity = randRange(random, 1500, 1900)
  const wind = randRange(random, -55, 75) * direction
  const originY = randRange(random, -42, 42)
  const tiny = shape === 'tiny'
  const dot = shape === 'dot'
  const strip = shape === 'strip'
  const width = tiny
    ? randRange(random, 2, 4)
    : dot
      ? randRange(random, 4, 7)
      : strip
        ? randRange(random, 3, 6)
        : randRange(random, 7, 13)
  const height = tiny
    ? randRange(random, 2, 4)
    : dot
      ? width
      : strip
        ? randRange(random, 12, 24)
        : randRange(random, 7, 15)
  const delay = (index % 28) * randRange(random, 0.004, 0.012) + Math.floor(index / 28) * 0.026
  const duration = randRange(random, 1850, 2500)
  const durationSeconds = duration / 1000
  const spin = direction * randRange(random, 260, 1040)
  const style: Record<string, string> = {
    '--start-x': side === 'left' ? '18px' : 'calc(100vw - 18px)',
    '--origin-y': `${originY.toFixed(1)}px`,
    '--w': `${width.toFixed(1)}px`,
    '--h': `${height.toFixed(1)}px`,
    '--radius': dot ? '999px' : `${randRange(random, 0.5, 2.5).toFixed(1)}px`,
    '--c': pick(random, palette),
    '--delay': `${delay.toFixed(3)}s`,
    '--dur': `${duration.toFixed(0)}ms`,
    '--r0': `${randRange(random, -48, 48).toFixed(1)}deg`,
    '--spin': `${spin.toFixed(1)}deg`,
    'z-index': String(total - index),
  }

  samples.forEach(([name, fraction]) => {
    const point = projectilePoint(v0x, v0y, gravity, durationSeconds * fraction, wind)
    style[`--p${name}-x`] = `${point.x.toFixed(1)}px`
    style[`--p${name}-y`] = `${point.y.toFixed(1)}px`
  })

  return {
    id: `${side}-${index}`,
    side,
    shape,
    style,
  }
}

function createPieces(): FireworkPiece[] {
  const pieces: FireworkPiece[] = []

  for (let index = 0; index < PIECES_PER_SIDE; index += 1) {
    pieces.push(createPiece('left', index, PIECES_PER_SIDE))
    pieces.push(createPiece('right', index, PIECES_PER_SIDE))
  }

  return pieces
}

const pieces = createPieces()
</script>

<style scoped>
.dashboard-fireworks {
  position: fixed;
  inset: 0;
  z-index: 80;
  overflow: hidden;
  pointer-events: none;
}

.dashboard-fireworks__piece {
  position: absolute;
  top: 62vh;
  left: var(--start-x);
  width: var(--w);
  height: var(--h);
  border-radius: var(--radius);
  background: var(--c);
  opacity: 0;
  transform: translate(0, var(--origin-y)) rotate(var(--r0)) scale(0.55);
  transform-origin: center;
  box-shadow: 0 0 8px rgb(255 255 255 / 0.36);
  animation: dashboard-fireworks-free-fall var(--dur) linear var(--delay) both;
  will-change: transform, opacity;
}

.dashboard-fireworks__piece--shard {
  border-radius: 1px;
  clip-path: polygon(0 0, 100% 18%, 64% 100%, 12% 78%);
}

.dashboard-fireworks__piece--triangle {
  border-radius: 1px;
  clip-path: polygon(50% 0, 100% 100%, 0 78%);
}

.dashboard-fireworks__piece--dot {
  border-radius: 999px;
}

.dashboard-fireworks__piece--tiny {
  filter: saturate(1.4);
}

@keyframes dashboard-fireworks-free-fall {
  0% {
    opacity: 0;
    transform: translate(0, var(--origin-y)) rotate(var(--r0)) scale(0.4);
  }

  3% {
    opacity: 1;
  }

  5% {
    opacity: 1;
    transform:
      translate(var(--p05-x), calc(var(--origin-y) + var(--p05-y)))
      rotate(calc(var(--spin) * 0.08))
      scale(0.92);
  }

  12% {
    opacity: 1;
    transform:
      translate(var(--p12-x), calc(var(--origin-y) + var(--p12-y)))
      rotate(calc(var(--spin) * 0.18))
      scale(1);
  }

  22% {
    opacity: 1;
    transform:
      translate(var(--p22-x), calc(var(--origin-y) + var(--p22-y)))
      rotate(calc(var(--spin) * 0.33))
      scale(0.98);
  }

  36% {
    opacity: 0.96;
    transform:
      translate(var(--p36-x), calc(var(--origin-y) + var(--p36-y)))
      rotate(calc(var(--spin) * 0.52))
      scale(0.94);
  }

  52% {
    opacity: 0.94;
    transform:
      translate(var(--p52-x), calc(var(--origin-y) + var(--p52-y)))
      rotate(calc(var(--spin) * 0.7))
      scale(0.94);
  }

  72% {
    opacity: 0.78;
    transform:
      translate(var(--p72-x), calc(var(--origin-y) + var(--p72-y)))
      rotate(calc(var(--spin) * 0.88))
      scale(0.86);
  }

  90% {
    opacity: 0.42;
    transform:
      translate(var(--p90-x), calc(var(--origin-y) + var(--p90-y)))
      rotate(calc(var(--spin) * 0.98))
      scale(0.78);
  }

  100% {
    opacity: 0;
    transform:
      translate(var(--p100-x), calc(var(--origin-y) + var(--p100-y)))
      rotate(var(--spin))
      scale(0.66);
  }
}

@media (max-width: 767px) {
  .dashboard-fireworks {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-fireworks {
    display: none;
  }
}
</style>
