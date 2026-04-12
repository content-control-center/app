import { Loader2 } from 'lucide-react'

type PageLoaderProps = {
  message?: string
}

export function PageLoader({ message }: PageLoaderProps) {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="flex flex-col gap-4 items-center justify-center max-w-xl">
        <Loader2 className="size-8 animate-spin" />
        <span>{message ? message : 'Loading'}</span>
      </div>
    </div>
  )
}
