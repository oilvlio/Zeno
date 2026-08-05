import type { AriaRole, ReactNode } from 'react'
import { slidingSelectorStyle } from '../lib/slidingSelector'

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
}: SlidingSelectorProps<T>) {
  const activeIndex = Math.max(0, options.findIndex((option) => option.value === value))
  const activeValue = options[activeIndex]?.value
  const Tag = as

  return (
    <Tag
      className={`sliding-selector${className ? ` ${className}` : ''}`}
      aria-label={ariaLabel}
      role={role}
      style={slidingSelectorStyle(options.length, activeIndex)}
    >
      {options.map((option) => {
        const active = option.value === activeValue
        return (
          <button
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
