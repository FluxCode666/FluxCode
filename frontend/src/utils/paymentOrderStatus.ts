import type { OrderStatus } from '@/types/payment'

export function isOrderCredited(status?: OrderStatus | string | null): boolean {
  return status === 'COMPLETED'
}

export function isOrderProcessing(status?: OrderStatus | string | null): boolean {
  return status === 'PAID' || status === 'RECHARGING'
}

export function isOrderFailed(status?: OrderStatus | string | null): boolean {
  return status === 'EXPIRED' || status === 'CANCELLED' || status === 'FAILED'
}
