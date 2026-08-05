import { useEffect, useRef, useState } from 'react'
import { SlidingSelector } from './SlidingSelector'

interface HistoryRangeOption {
  value: string
  label: string
}

interface HistoryRangeSelectorProps {
  ariaLabel: string
  options: ReadonlyArray<HistoryRangeOption>
  value: string
  onChange: (value: string) => void
  className?: string
  commitDelayMs?: number
}

export function HistoryRangeSelector({ ariaLabel, options, value, onChange, className = '', commitDelayMs = 0 }: HistoryRangeSelectorProps) {
  const [displayValue, setDisplayValue] = useState(value)
  const commitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (commitTimerRef.current !== null) {
      clearTimeout(commitTimerRef.current)
      commitTimerRef.current = null
    }
    setDisplayValue(value)
  }, [value])

  useEffect(() => () => {
    if (commitTimerRef.current !== null) clearTimeout(commitTimerRef.current)
    commitTimerRef.current = null
  }, [])

  const selectRange = (nextValue: string) => {
    if (nextValue === displayValue) return
    setDisplayValue(nextValue)
    if (commitTimerRef.current !== null) clearTimeout(commitTimerRef.current)
    if (commitDelayMs <= 0) {
      onChange(nextValue)
      return
    }
    commitTimerRef.current = setTimeout(() => {
      commitTimerRef.current = null
      onChange(nextValue)
    }, commitDelayMs)
  }

  return (
    <SlidingSelector
      ariaLabel={ariaLabel}
      role="group"
      className={`detail-range-row sliding-selector--compact${className ? ` ${className}` : ''}`}
      options={options.map((option) => ({ value: option.value, content: option.label }))}
      value={displayValue}
      onChange={selectRange}
    />
  )
}
