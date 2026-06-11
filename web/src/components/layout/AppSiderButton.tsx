import { type Icon as PhosphorIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button.tsx'
import { Link } from '@tanstack/react-router'
import { cn } from '@/lib'

type AppSidebarButtonSearchProps = {
  onClick?: () => void
}

export function AppSidebarButtonSearch({ onClick }: AppSidebarButtonSearchProps) {
  return (
    <Button variant="searchBar" size={'excluded'} onClick={onClick}>
      <div>
        <MagnifyingGlassIcon className="size-5 flex-none" />
        <div className="flex-none text-left w-[108px]">
          <span className="font-mono uppercase">Search</span>
        </div>
        <div className="hidden lg:block w-16 flex-none">
          <span className="text-right font-mono uppercase">CMD + F</span>
        </div>
      </div>
    </Button>
  )
}

type AppSidebarButtonMenuLinkProps = {
  icon: PhosphorIcon
  text: string
  isActive: boolean
  to: string
  onClick?: () => void
  className?: string
}

type AppSidebarButtonMenuActionProps = {
  icon: PhosphorIcon
  text: string
  isActive: boolean
  onClick?: () => void
  onMouseEnter?: () => void
  onMouseLeave?: () => void
  className?: string
}

type AppSidebarButtonMenuProps = AppSidebarButtonMenuLinkProps | AppSidebarButtonMenuActionProps

function isLinkProps(props: AppSidebarButtonMenuProps): props is AppSidebarButtonMenuLinkProps {
  return 'to' in props
}

export function AppSidebarButtonMenu(props: AppSidebarButtonMenuProps) {
  const { icon: Icon, text, isActive, className } = props

  if (isLinkProps(props)) {
    // Navigation type - uses Link
    return (
      <Button
        variant="menu"
        size={'excluded'}
        asChild
        className={cn(className, isActive && 'text-sidebar-primary-foreground')}
      >
        <Link to={props.to} onClick={props.onClick}>
          <Icon
            weight={isActive ? 'fill' : 'regular'}
            className="size-5 flex-none"
          />
          <div className="flex-none text-left w-[108px]">
            <span className="font-mono uppercase">{text}</span>
          </div>
        </Link>
      </Button>
    )
  }

  // Action type - uses onHandleSettings/hover handlers
  return (
    <Button
      variant="menu"
      size={'excluded'}
      className={cn(className, isActive && 'text-sidebar-primary-foreground')}
      onClick={props.onClick}
      onMouseEnter={props.onMouseEnter}
      onMouseLeave={props.onMouseLeave}
    >
      <Icon
        weight={isActive ? 'fill' : 'regular'}
        className="size-5 flex-none"
      />
      <div className="flex-none text-left w-[108px]">
        <span className="font-mono uppercase">{text}</span>
      </div>
    </Button>
  )
}
