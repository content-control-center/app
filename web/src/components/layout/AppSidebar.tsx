import * as React from 'react'
import { Link, useLocation } from '@tanstack/react-router'
import { CaretDoubleLeftIcon, GearSixIcon, MegaphoneIcon, SignOutIcon, VaultIcon, XIcon } from '@phosphor-icons/react'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar.tsx'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore } from '@/stores/authStore'
import { useSettingsStore } from '@/stores/settingsStore'
import { useNavigate } from '@tanstack/react-router'
import { Logo } from '@/components/Logo'
import { cn } from '@/lib'
import {
  AppSidebarButtonMenu,
} from '@/components/layout/AppSiderButton.tsx'

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { state, isMobile, setOpen, toggleSidebar } = useSidebar()
  const location = useLocation()
  const isCollapsed = isMobile ? false : state === 'collapsed'
  const { user } = useAuthStore()
  const { closeSecondaryNavbar } = useSettingsStore()
  const navigate = useNavigate()

  const handleLogout = () => {
    navigate({ to: '/auth/logout' })
  }


  const initials = `${user!.firstName[0]}${user!.lastName[0]}`.toUpperCase()

  return (
    <>
      <Sidebar collapsible="icon" className={'select-none'} {...props}>
        <SidebarHeader>
          <div className="flex items-center justify-between">
            {isMobile ? (
              <Button
                variant="default"
                size="smIcon"
                onClick={toggleSidebar}
                aria-label="Close sidebar"
              >
                <XIcon weight="bold" className="size-5" />
              </Button>
            ) : (
              <Link
                to="/"
                className={cn('flex items-center gap-2 font-semibold text-lg transition-all')}
                onClick={closeSecondaryNavbar}
              >
                <Logo className="size-10 shrink-0" />
              </Link>
            )}
            {isMobile ? (
              <Logo className={'size-8'} />
            ) : (
              <Button
                variant="ghost"
                size="xsIcon"
                className={cn(
                  'flex group/button h-full transition-all duration-150',
                  isCollapsed && 'opacity-0 pointer-events-none',
                  !isCollapsed && 'opacity-100 delay-100'
                )}
                onClick={() => setOpen(false)}
              >
                <CaretDoubleLeftIcon
                  weight="bold"
                  className="size-3 text-quaternary-foreground group-hover/button:text-primary-foreground transition-colors"
                />
              </Button>
            )}
          </div>
        </SidebarHeader>
        <SidebarContent>
          <nav
            className={cn('flex flex-col gap-2 px-3 py-0 lg:p-6', isCollapsed && 'items-center')}
          >
            <AppSidebarButtonMenu
              icon={MegaphoneIcon}
              text="Campaigns"
              isActive={location.pathname.startsWith('/campaigns')}
              to="/campaigns"
              onClick={closeSecondaryNavbar}
            />
            <AppSidebarButtonMenu
              icon={VaultIcon}
              text="Content Bank"
              isActive={location.pathname.startsWith('/content-bank')}
              to="/content-bank"
              onClick={closeSecondaryNavbar}
            />
            <AppSidebarButtonMenu
              icon={GearSixIcon}
              text="Instance Settings"
              isActive={location.pathname.startsWith('/instance-settings')}
              to="/instance-settings"
              onClick={closeSecondaryNavbar}
            />
          </nav>
        </SidebarContent>

        <SidebarFooter>
          <div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <div
                  role="button"
                  tabIndex={0}
                  className={cn('flex w-full items-center justify-start gap-6 p-0 cursor-pointer select-none overflow-hidden')}
                >
                  <div className="relative shrink-0">
                    <Avatar className="size-10">
                      <AvatarFallback>{initials}</AvatarFallback>
                    </Avatar>

                  </div>
                  <div className="flex flex-col items-start flex-1 min-w-0">
                    <p className="text-sm font-regular truncate">{`${user!.firstName} ${user!.lastName}`}</p>
                    <p className="text-xs text-tertiary-foreground truncate">
                      { user!.email}
                    </p>
                  </div>
                </div>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                className="w-68 px-6 pt-6 pb-4 shadow-md"
                side="right"
                align="end"
                sideOffset={8}
              >
                <DropdownMenuLabel className="font-normal p-0" asChild>
                  <div className="flex flex-col space-y-1">
                    <div className="h-8 text-xl font-display font-medium truncate">{`${user!.firstName} ${user!.lastName}`}</div>
                    <div className="text-sm leading-none text-tertiary-foreground">
                      {user!.email}
                    </div>
                  </div>
                </DropdownMenuLabel>


                  <DropdownMenuItem onClick={handleLogout} size="lg">
                    <SignOutIcon />
                    <span>Log out</span>
                  </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

    </>
  )
}
