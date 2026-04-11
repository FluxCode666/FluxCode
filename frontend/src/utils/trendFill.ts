import type { TrendDataPoint } from '@/types'

/**
 * Zero-value template for TrendDataPoint.
 */
const ZERO_POINT: Omit<TrendDataPoint, 'date'> = {
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0
}

/**
 * Generate all expected date labels for 'day' granularity.
 * @param startDate - "YYYY-MM-DD"
 * @param endDate   - "YYYY-MM-DD"
 * @returns Array of "YYYY-MM-DD" strings from startDate to endDate (inclusive).
 */
function generateDayLabels(startDate: string, endDate: string): string[] {
  const labels: string[] = []
  const [sy, sm, sd] = startDate.split('-').map(Number)
  const [ey, em, ed] = endDate.split('-').map(Number)
  const start = new Date(sy, sm - 1, sd)
  const end = new Date(ey, em - 1, ed)

  const current = new Date(start)
  while (current <= end) {
    const y = current.getFullYear()
    const m = String(current.getMonth() + 1).padStart(2, '0')
    const d = String(current.getDate()).padStart(2, '0')
    labels.push(`${y}-${m}-${d}`)
    current.setDate(current.getDate() + 1)
  }
  return labels
}

/**
 * Generate all expected hour labels for 'hour' granularity.
 * Backend returns labels in "YYYY-MM-DD HH:00" format.
 * @param startDate - "YYYY-MM-DD"
 * @param endDate   - "YYYY-MM-DD"
 * @returns Array of "YYYY-MM-DD HH:00" strings covering all hours in the range.
 */
function generateHourLabels(startDate: string, endDate: string): string[] {
  const labels: string[] = []
  const [sy, sm, sd] = startDate.split('-').map(Number)
  const [ey, em, ed] = endDate.split('-').map(Number)
  const start = new Date(sy, sm - 1, sd, 0, 0, 0)
  // End date is inclusive: go up to 23:00 on that day
  const end = new Date(ey, em - 1, ed, 23, 0, 0)

  const current = new Date(start)
  while (current <= end) {
    const y = current.getFullYear()
    const mo = String(current.getMonth() + 1).padStart(2, '0')
    const d = String(current.getDate()).padStart(2, '0')
    const h = String(current.getHours()).padStart(2, '0')
    labels.push(`${y}-${mo}-${d} ${h}:00`)
    current.setTime(current.getTime() + 3600000)
  }
  return labels
}

/**
 * Fill gaps in trend data so that every expected time bucket has a data point.
 * Missing buckets are filled with zero values.
 *
 * @param data        - Raw trend data from the API
 * @param startDate   - Range start "YYYY-MM-DD"
 * @param endDate     - Range end   "YYYY-MM-DD"
 * @param granularity - "day" or "hour"
 * @returns Trend data with gaps filled with zeros, sorted chronologically.
 */
export function fillTrendDataGaps(
  data: TrendDataPoint[],
  startDate: string,
  endDate: string,
  granularity: 'day' | 'hour'
): TrendDataPoint[] {
  const allLabels =
    granularity === 'hour'
      ? generateHourLabels(startDate, endDate)
      : generateDayLabels(startDate, endDate)

  if (allLabels.length === 0) return data

  // Build a lookup map keyed by the date label
  const dataMap = new Map<string, TrendDataPoint>()
  for (const point of data) {
    dataMap.set(point.date, point)
  }

  return allLabels.map((label) => dataMap.get(label) ?? { date: label, ...ZERO_POINT })
}
