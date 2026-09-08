import { useQuery } from "@tanstack/react-query";
import { overviewSchema, errorMessage } from "./api";
import type { Overview } from "./api";
import { useAuth } from "./auth";
import { PageHeader } from "./shell";

export function OverviewPage() {
  const { api } = useAuth();
  const query = useQuery({ queryKey: ["admin", "overview"], queryFn: async ({ signal }) => overviewSchema.parse(await api("overview", signal)) });
  return <><PageHeader page="overview" refresh={() => void query.refetch()} busy={query.isFetching} />
    {query.isPending && <p className="notice" role="status">正在加载数据…</p>}
    {query.isError && <p className="notice error" role="alert">{errorMessage(query.error)}</p>}
    {query.data && <OverviewData data={query.data} />}</>;
}
function OverviewData({ data }: { data: Overview }) {
  const cards = [
    ["users", "注册用户", "累计注册账户"], ["downloads", "安装包下载", "累计上报事件"],
    ["open_crashes", "待处理崩溃", `累计 ${data.crashes} 条报告`], ["skins", "社区皮肤", `${data.skin_downloads} 次用户去重下载`],
  ] as const;
  const stats = [["new_users_30d", "近30天新用户"], ["session_users", "持有有效会话的用户"], ["dictionaries", "共享词库"], ["replies", "回复模板"], ["resource_saves", "资源收藏"]] as const;
  const max = Math.max(1, ...data.daily.map(day => day.downloads));
  return <section><div className="cards">{cards.map(([key, title, hint]) => <article className="metric" key={key}><p className="muted">{title}</p><strong>{data[key].toLocaleString()}</strong><p className="small muted">{hint}</p></article>)}</div>
    <article className="panel"><div className="panel-heading"><h2>近 30 天趋势</h2><span className="badge">下载上报 / 每日</span></div><div className="chart">
      <svg viewBox="0 0 900 160" role="img" aria-label="最近30天每日下载上报柱状图，精确数值见下方每日明细"><title>最近30天下载上报趋势</title>{data.daily.map((day, index) => <rect key={day.day} x={index * 30 + 4} y={150 - day.downloads / max * 145} width={20} height={Math.max(1, day.downloads / max * 145)} rx={3}><title>{day.day}: {day.downloads}</title></rect>)}</svg>
    </div><div className="axis"><span>{data.daily[0].day}</span><span>UTC</span><span>{data.daily.at(-1)?.day}</span></div></article>
    <div className="secondary-stats">{stats.map(([key, title]) => <div key={key}><strong>{data[key].toLocaleString()}</strong><span className="muted small">{title}</span></div>)}</div>
    <details className="panel"><summary>每日数据明细</summary><table><thead><tr>{["日期 (UTC)", "新增用户", "下载上报", "崩溃上报"].map(title => <th key={title} scope="col">{title}</th>)}</tr></thead><tbody>{[...data.daily].reverse().map(day => <tr key={day.day}><td>{day.day}</td><td>{day.users}</td><td>{day.downloads}</td><td>{day.crashes}</td></tr>)}</tbody></table></details>
  </section>;
}
