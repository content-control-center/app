import { type SVGProps, type ComponentType } from 'react'
import { cn } from '@/lib'

/**
 * Icon Component - All icons eager-loaded
 *
 * All icons are bundled immediately for optimal performance.
 *
 * @see ICONS.md for complete documentation on:
 * - Available icons and their categories
 * - How to add new icons
 * - Usage examples and best practices
 */

// All icons (eagerly loaded)
import ArrowDownPointedIcon from '@/assets/icons/arrow_down_pointed.svg?react'
import ArrowLeftIcon from '@/assets/icons/arrow_left.svg?react'
import ArrowRightIcon from '@/assets/icons/arrow_right.svg?react'
import ArrowRightTopIcon from '@/assets/icons/arrow_right_top.svg?react'
import ArrowUpPointedIcon from '@/assets/icons/arrow_up_pointed.svg?react'
import BurgerIcon from '@/assets/icons/burger.svg?react'
import CalendarIcon from '@/assets/icons/calendar.svg?react'
import ChangeIcon from '@/assets/icons/change.svg?react'
import CheckIcon from '@/assets/icons/check.svg?react'
import ChevronDoubleLeftIcon from '@/assets/icons/chevron_double_left.svg?react'
import ChevronDoubleRightIcon from '@/assets/icons/chevron_double_right.svg?react'
import ChevronDownIcon from '@/assets/icons/chevron_down.svg?react'
import ChevronLeftIcon from '@/assets/icons/chevron_left.svg?react'
import ChevronRightIcon from '@/assets/icons/chevron_right.svg?react'
import ChevronUpIcon from '@/assets/icons/chevron_up.svg?react'
import CollapseTopIcon from '@/assets/icons/collapse_top.svg?react'
import CommentIcon from '@/assets/icons/comment.svg?react'
import Dots2VerticalIcon from '@/assets/icons/dots_2_vertical.svg?react'
import DotsDragVerticalIcon from '@/assets/icons/dots_drag_vertical.svg?react'
import EditIcon from '@/assets/icons/edit.svg?react'
import ExitIcon from '@/assets/icons/exit.svg?react'
import EmptyIcon from '@/assets/icons/empty.svg?react'
import FilterEmptyIcon from '@/assets/icons/filter_empty.svg?react'
import LayoutIcon from '@/assets/icons/layout.svg?react'
import NavDashboardIcon from '@/assets/icons/nav_dashboard.svg?react'
import NavIdeasIcon from '@/assets/icons/nav_ideas.svg?react'
import NavJournalIcon from '@/assets/icons/nav_journal.svg?react'
import NavPortfoliosIcon from '@/assets/icons/nav_portfolios.svg?react'
import NavScreeningIcon from '@/assets/icons/nav_screening.svg?react'
import NavStrategyIcon from '@/assets/icons/nav_strategy.svg?react'
import NavSettingsIcon from '@/assets/icons/nav_settings.svg?react'
import NavWatchlistIcon from '@/assets/icons/nav_watchlist.svg?react'
import PlusIcon from '@/assets/icons/plus.svg?react'
import SearchIcon from '@/assets/icons/search.svg?react'
import SearchAnimateIcon from '@/assets/icons/search_animate.svg?react'
import SettingsIcon from '@/assets/icons/settings.svg?react'
import StrategyIcon from '@/assets/icons/nav_strategy.svg?react'
import TrashBinIcon from '@/assets/icons/trash_bin.svg?react'
import TrendDownIcon from '@/assets/icons/trend_down.svg?react'
import TrendStableIcon from '@/assets/icons/trend_stable.svg?react'
import TrendUpIcon from '@/assets/icons/trend_up.svg?react'
import UncollapseTopIcon from '@/assets/icons/uncollapse_top.svg?react'
import WidgetMaximizeIcon from '@/assets/icons/widget_maximize.svg?react'
import WidgetMinimizeIcon from '@/assets/icons/widget_minimize.svg?react'
import XMarkIcon from '@/assets/icons/x_mark.svg?react'

// All icon names (alphabetically sorted)
type CriticalIconName =
  | 'arrow_down_pointed'
  | 'arrow_left'
  | 'arrow_right'
  | 'arrow_right_top'
  | 'arrow_up_pointed'
  | 'burger'
  | 'calendar'
  | 'change'
  | 'check'
  | 'chevron_double_left'
  | 'chevron_double_right'
  | 'chevron_down'
  | 'chevron_left'
  | 'chevron_right'
  | 'chevron_up'
  | 'collapse_top'
  | 'comment'
  | 'dots_2_vertical'
  | 'dots_drag_vertical'
  | 'edit'
  | 'exit'
  | 'empty'
  | 'filter_empty'
  | 'layout'
  | 'nav_dashboard'
  | 'nav_ideas'
  | 'nav_journal'
  | 'nav_portfolios'
  | 'nav_screening'
  | 'nav_settings'
  | 'nav_strategy'
  | 'nav_watchlist'
  | 'plus'
  | 'search'
  | 'search_animate'
  | 'settings'
  | 'strategy'
  | 'trash_bin'
  | 'trend_down'
  | 'trend_stable'
  | 'trend_up'
  | 'uncollapse_top'
  | 'widget_maximize'
  | 'widget_minimize'
  | 'x_mark'

export type IconName = CriticalIconName

export type IconProps = {
  name: IconName
  className?: string
} & Omit<SVGProps<SVGSVGElement>, 'ref'>

// Type for SVG components
type SvgComponent = ComponentType<SVGProps<SVGSVGElement>>

// Map of all icons (eagerly loaded)
const icons: Record<CriticalIconName, SvgComponent> = {
  arrow_down_pointed: ArrowDownPointedIcon,
  arrow_left: ArrowLeftIcon,
  arrow_right: ArrowRightIcon,
  arrow_right_top: ArrowRightTopIcon,
  arrow_up_pointed: ArrowUpPointedIcon,
  burger: BurgerIcon,
  calendar: CalendarIcon,
  change: ChangeIcon,
  check: CheckIcon,
  chevron_double_left: ChevronDoubleLeftIcon,
  chevron_double_right: ChevronDoubleRightIcon,
  chevron_down: ChevronDownIcon,
  chevron_left: ChevronLeftIcon,
  chevron_right: ChevronRightIcon,
  chevron_up: ChevronUpIcon,
  collapse_top: CollapseTopIcon,
  comment: CommentIcon,
  dots_2_vertical: Dots2VerticalIcon,
  dots_drag_vertical: DotsDragVerticalIcon,
  edit: EditIcon,
  exit: ExitIcon,
  empty: EmptyIcon,
  filter_empty: FilterEmptyIcon,
  layout: LayoutIcon,
  nav_dashboard: NavDashboardIcon,
  nav_ideas: NavIdeasIcon,
  nav_journal: NavJournalIcon,
  nav_portfolios: NavPortfoliosIcon,
  nav_screening: NavScreeningIcon,
  nav_settings: NavSettingsIcon,
  nav_strategy: NavStrategyIcon,
  nav_watchlist: NavWatchlistIcon,
  plus: PlusIcon,
  search: SearchIcon,
  search_animate: SearchAnimateIcon,
  settings: SettingsIcon,
  strategy: StrategyIcon,
  trash_bin: TrashBinIcon,
  trend_down: TrendDownIcon,
  trend_stable: TrendStableIcon,
  trend_up: TrendUpIcon,
  uncollapse_top: UncollapseTopIcon,
  widget_maximize: WidgetMaximizeIcon,
  widget_minimize: WidgetMinimizeIcon,
  x_mark: XMarkIcon,
}

export function Icon({ name, className, ...props }: IconProps) {
  const IconComponent = icons[name]
  if (!IconComponent) {
    if (import.meta.env.DEV) {
      console.warn(`[Icon] Unknown icon name: "${name}"`)
    }
    return null
  }
  return <IconComponent className={cn('inline-block', className)} {...props} />
}
