import { useState, useEffect, type ReactNode } from 'react'
import { ZIndex } from '@/config/zIndex.ts'

type ScreenOverlayProps = {
  isLoading: boolean
  loadingTime?: number
  children: ReactNode
}

export const ScreenOverlay = ({ isLoading, children, loadingTime }: ScreenOverlayProps) => {
  const [shouldRender, setShouldRender] = useState(true)
  const [isVisible, setIsVisible] = useState(true)

  useEffect(() => {
    if (!isLoading) {
      // Start fade out
      setIsVisible(false)

      // Remove from DOM after transition completes
      const timer = setTimeout(() => {
        setShouldRender(false)
      }, loadingTime) // Match the transition duration

      return () => clearTimeout(timer)
    }
  }, [isLoading])

  if (!shouldRender) {
    return null
  }

  return (
    <div
      id="app-loader"
      className={`absolute inset-0 flex items-center justify-center bg-popover transition-opacity ${
        isVisible ? 'opacity-100' : 'opacity-0'
      }`}
      style={{
        zIndex: ZIndex.appLoader,
        transitionDuration: `${loadingTime}ms`,
      }}
    >
      {children}
    </div>
  )
}
