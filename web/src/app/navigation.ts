import { Home, CalendarDays, CheckSquare, Users, UserRoundPlus, Bell, Archive, Link, Settings } from 'lucide-react';
export const navigation = [
  { path: '/', label: 'Главная', icon: Home }, { path: '/calendar', label: 'Календарь', icon: CalendarDays },
  { path: '/tasks', label: 'Задачи', icon: CheckSquare }, { path: '/meetings', label: 'Встречи', icon: Users },
  { path: '/friends', label: 'Друзья', icon: UserRoundPlus },
  { path: '/reminders', label: 'Напоминания', icon: Bell }, { path: '/archive', label: 'Архив', icon: Archive },
  { path: '/integrations', label: 'Интеграции', icon: Link }, { path: '/settings', label: 'Настройки', icon: Settings },
] as const;
