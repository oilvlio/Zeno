import { type CSSProperties, type AriaRole, type ReactNode, useLayoutEffect, useRef, useState } from 'react'
import { slidingSelectorGeometry, slidingSelectorStyle, type SlidingSelectorGeometry } from '../lib/slidingSelector'

export interface SlidingSelectorOption<T extends string> {
  value: T
  content: ReactNode
  ariaLabel?: string
  title?: string
  className?: string
}

interface SlidingSelectorProps<T extends string> {
  ariaLabel: string
  options: ReadonlyArray<SlidingSelectorOption<T>>
  value: T
  onChange: (value: T) => void
  className?: string
  as?: 'div' | 'nav'
  role?: AriaRole
  selectionMode?: 'pressed' | 'current'
  maxColumns?: number
}

export function SlidingSelector<T extends string>({
  ariaLabel,
  options,
  value,
  onChange,
  className = '',
  as = 'div',
  role,
  selectionMode = 'pressed',
  maxColumns,
}: SlidingSelectorProps<T>) {
  const activeIndex = Math.max(0, options.findIndex((option) => option.value === value))
  const activeValue = options[activeIndex]?.value
  const Tag = as
  const selectorRef = useRef<HTMLElement | null>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const [geometry, setGeometry] = useState<SlidingSelectorGeometry | null>(null)

  useLayoutEffect(() => {
    const selector = selectorRef.current
    const option = optionRefs.current[activeIndex]
    if (!selector || !option) return undefined

    let frame = 0
    const measureNow = () => {
      const selectorRect = selector.getBoundingClientRect()
      const optionRect = option.getBoundingClientRect()
      setGeometry(slidingSelectorGeometry(selectorRect, optionRect))
    }
    const scheduleMeasure = () => {
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(measureNow)
    }
    measureNow()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(scheduleMeasure)
    observer?.observe(selector)
    observer?.observe(option)
    window.addEventListener('resize', scheduleMeasure)
    return () => {
      window.cancelAnimationFrame(frame)
      observer?.disconnect()
      window.removeEventListener('resize', scheduleMeasure)
    }
  }, [activeIndex, maxColumns, options.length])

  const baseStyle = slidingSelectorStyle(options.length, activeIndex, maxColumns ?? options.length)
  const style = geometry ? { ...baseStyle, ...geometry, '--slider-measured': 1 } as CSSProperties : baseStyle

  return (
    <Tag
      ref={(element) => { selectorRef.current = element }}
      className={`sliding-selector${className ? ` ${className}` : ''}`}
      aria-label={ariaLabel}
      role={role}
      style={style}
    >
      {options.map((option, index) => {
        const active = option.value === activeValue
        return (
          <button
            ref={(element) => { optionRefs.current[index] = element }}
            key={option.value}
            type="button"
            className={option.className}
            data-active={active}
            data-value={option.value}
            aria-label={option.ariaLabel}
            aria-pressed={selectionMode === 'pressed' ? active : undefined}
            aria-current={selectionMode === 'current' && active ? 'page' : undefined}
            title={option.title}
            onClick={() => onChange(option.value)}
          >
            {option.content}
          </button>
        )
      })}
    </Tag>
  )
}
