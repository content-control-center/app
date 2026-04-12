import { Icon, type IconName } from '@/components/ui/icon.tsx'
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
        <Icon name="search" className="size-5 flex-none" />
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
  iconName: IconName
  text: string
  isActive: boolean
  to: string
  onClick?: () => void
  className?: string
}

type AppSidebarButtonMenuActionProps = {
  iconName: IconName
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
  const { iconName, text, isActive, className } = props

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
            name={iconName}
            className={cn('size-5 flex-none stroke-1.5', isActive && 'icon-sidebar-active')}
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
        name={iconName}
        className={cn('size-5 flex-none stroke-1.5', isActive && 'icon-sidebar-active')}
      />
      <div className="flex-none text-left w-[108px]">
        <span className="font-mono uppercase">{text}</span>
      </div>
    </Button>
  )
}
