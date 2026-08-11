import {
  Home,
  CircleDot,
  Users,
  MessageSquare,
  Settings,
  Flame,
  ScrollText,
  UserPlus,
  History,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'

/**
 * Each nav entry is keyed by a dotted path into the `nav` namespace of
 * the i18n message catalog (see `frontend/messages/{locale}.json`).
 *
 *   labelKey   — top-level menu label
 *   titleKey   — child entry title (full text)
 *   descKey    — child entry one-line description
 *
 * This file owns structure (paths, icons, ordering) only — copy lives
 * entirely in the locale dictionaries.
 */
export interface NavChild {
  titleKey: string
  descKey: string
  path: string
  icon: LucideIcon
}

export interface NavItem {
  labelKey: string
  path: string
  icon: LucideIcon
  children?: NavChild[]
}

/** 顶部导航配置（Dify 式 mega-dropdown，参考 reference/导航栏.png） */
export const navItems: NavItem[] = [
  { labelKey: 'items.home.label', path: '/', icon: Home },
  {
    labelKey: 'items.pills.label',
    path: '/pills',
    icon: CircleDot,
    children: [
      {
        titleKey: 'items.pills.children.all.title',
        descKey: 'items.pills.children.all.description',
        path: '/pills',
        icon: CircleDot,
      },
      {
        titleKey: 'items.pills.children.new.title',
        descKey: 'items.pills.children.new.description',
        path: '/pills?action=new',
        icon: Flame,
      },
      {
        titleKey: 'items.pills.children.recipes.title',
        descKey: 'items.pills.children.recipes.description',
        path: '/pills?view=recipes',
        icon: ScrollText,
      },
    ],
  },
  {
    labelKey: 'items.agents.label',
    path: '/agents',
    icon: Users,
    children: [
      {
        titleKey: 'items.agents.children.all.title',
        descKey: 'items.agents.children.all.description',
        path: '/agents',
        icon: Users,
      },
      {
        titleKey: 'items.agents.children.invite.title',
        descKey: 'items.agents.children.invite.description',
        path: '/agents?action=new',
        icon: UserPlus,
      },
      {
        titleKey: 'items.agents.children.bind.title',
        descKey: 'items.agents.children.bind.description',
        path: '/agents?view=bind',
        icon: Sparkles,
      },
    ],
  },
  {
    labelKey: 'items.chat.label',
    path: '/chat',
    icon: MessageSquare,
    children: [
      {
        titleKey: 'items.chat.children.new.title',
        descKey: 'items.chat.children.new.description',
        path: '/chat',
        icon: MessageSquare,
      },
      {
        titleKey: 'items.chat.children.history.title',
        descKey: 'items.chat.children.history.description',
        path: '/chat?view=history',
        icon: History,
      },
    ],
  },
  { labelKey: 'items.settings.label', path: '/settings', icon: Settings },
]
