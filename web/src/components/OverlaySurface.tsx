import { forwardRef, type HTMLAttributes } from 'react'

export const OverlaySurface = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(function OverlaySurface({ className = '', ...props }, ref) {
  return <div {...props} ref={ref} className={['zeno-overlay-surface', className].filter(Boolean).join(' ')} />
})
