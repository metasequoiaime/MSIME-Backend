import { ArrowRight, BookOpen, CircleAlert, Download, History, LayoutDashboard, MessageSquare, RefreshCw, Shirt, Users } from "lucide-react";

const icons = {
  overview: LayoutDashboard,
  users: Users,
  downloads: Download,
  crashes: CircleAlert,
  skins: Shirt,
  dictionaries: BookOpen,
  replies: MessageSquare,
  audit: History,
  refresh: RefreshCw,
  "arrow-right": ArrowRight,
} as const;

export type IconName = keyof typeof icons;

export function Icon({ name, className }: { name: IconName; className?: string }) {
  const Component = icons[name];
  return <Component className={className} size={20} strokeWidth={1.75} aria-hidden="true" focusable="false" />;
}
