import { ScreenOverlay } from '@/components/ui/screen-overlay.tsx'

type AppLoaderProps = {
  isLoading: boolean
}

export const AppLoader = ({ isLoading }: AppLoaderProps) => {
  return (
    <ScreenOverlay isLoading={isLoading}>
      <div className="font-display text-senary-foreground">Loading</div>
    </ScreenOverlay>
  )
}
