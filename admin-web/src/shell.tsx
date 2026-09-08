import { Link, Outlet, useLocation } from "@tanstack/react-router";
import { Login, useAuth } from "./auth";
import { pages } from "./pages";
import type { Page } from "./pages";

export function Shell() {
  const { token } = useAuth(); const path = useLocation({ select: value => value.pathname });
  if (!token) return <Login />;
  return <div><aside><Link className="brand" to="/"><span className="brand-icon">杉</span><div>水杉<small>管理控制台</small></div></Link><p className="nav-label">工作空间</p>
    <nav>{Object.entries(pages).map(([key, [title, , icon]]) => key === "overview" ? <Link key={key} to="/" className={path === "/" ? "active" : ""} aria-current={path === "/" ? "page" : undefined}><span className="nav-icon">{icon}</span><span>{title}</span></Link> : <Link key={key} to="/$section" params={{ section: key }} className={path === `/${key}` ? "active" : ""} aria-current={path === `/${key}` ? "page" : undefined}><span className="nav-icon">{icon}</span><span>{title}</span></Link>)}</nav>
    <div className="sidebar-foot"><span className="dot" /> MSIME BACKEND<br /><span className="muted">管理操作已启用审计</span></div>
  </aside><main><Outlet /><footer>水杉管理控制台<span>统计趋势按 UTC 自然日汇总 · 时间显示为本地时间</span></footer></main></div>;
}
export function PageHeader({ page, refresh, busy }: { page: Page; refresh: () => void; busy: boolean }) {
  const { logout } = useAuth(); const [title, subtitle] = pages[page];
  return <header><div><p className="eyebrow">WORKSPACE / {page.toUpperCase()}</p><h1>{title}</h1><p className="muted">{subtitle}</p></div><div className="header-actions"><button type="button" onClick={refresh} disabled={busy}>↻ 刷新</button><button type="button" onClick={logout}>退出</button></div></header>;
}
