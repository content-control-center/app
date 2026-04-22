import { createFileRoute } from '@tanstack/react-router'
import { PageContainer } from '@/components/page-primitives/PageContainer'
import { PageHeader } from '@/components/page-primitives/PageHeader'
import { PageLoader } from '@/components/page-primitives/PageLoader'
import { PageError } from '@/components/page-primitives/PageError'
import { usePlatforms } from '@/hooks/usePlatforms'
import { PlatformsSection } from '@/components/instance-settings/PlatformsSection'

export const Route = createFileRoute('/_authenticated/instance-settings/')({
  component: InstanceSettings,
})

function InstanceSettings() {
  const { data: platforms, isLoading, isError } = usePlatforms()

  if (isLoading) {
    return (
      <PageContainer>
        <PageLoader />
      </PageContainer>
    )
  }

  if (isError) {
    return (
      <PageContainer>
        <PageError header="Failed to load settings" />
      </PageContainer>
    )
  }

  return (
    <PageContainer>
      <PageHeader title="Settings" className="pt-6" />
      <div className="px-3 lg:px-6 pb-10 space-y-6">
        <PlatformsSection platforms={platforms ?? []} />
      </div>
    </PageContainer>
  )
}
