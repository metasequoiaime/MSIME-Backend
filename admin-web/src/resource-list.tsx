import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { actionSchema, errorMessage, listSchema } from "./api";
import type { Row } from "./api";
import { useAuth } from "./auth";
import { columns, actionLabels } from "./pages";
import type { ListPage } from "./pages";
import { PageHeader } from "./shell";

export function ResourceList({ section }: { section: ListPage }) {
  const { api } = useAuth(); const client = useQueryClient();
  const [page, setPage] = useState(1); const [search, setSearch] = useState(""); const [query, setQuery] = useState(""); const [detail, setDetail] = useState<Row | null>(null);
  const result = useQuery({ queryKey: ["admin", section, page, query], queryFn: async ({ signal }) => listSchema.parse(await api(`${section}?page=${page}&q=${encodeURIComponent(query)}`, signal)) });
  const mutation = useMutation({
    mutationFn: async ({ action, id }: { action: string; id: string }) => actionSchema.parse(await api("actions", undefined, { action, id })),
    onSuccess: () => client.invalidateQueries({ queryKey: ["admin"] }),
  });
  const fields = columns[section]; const editable = section !== "downloads" && section !== "audit";
  const act = (action: string, item: Row) => {
    if (!window.confirm(`${actionLabels[action]}：${item.name || item.display_name || item.id}？${action.startsWith("delete") ? "\n将永久删除内容及关联下载/收藏/评分记录，无法恢复。" : ""}`)) return;
    mutation.mutate({ action, id: String(item.id) });
  };
  return <><PageHeader page={section} busy={result.isFetching} refresh={() => void result.refetch()} />
    {result.isError && <p className="notice error" role="alert">{errorMessage(result.error)}</p>}
    {mutation.isError && <p className="notice error" role="alert">{errorMessage(mutation.error)}</p>}
    {mutation.isSuccess && <p className="notice" role="status">操作已完成，并已记录审计日志。</p>}
    <section className="list"><form className="toolbar" onSubmit={event => { event.preventDefault(); setQuery(search.trim()); setPage(1); }}><div><label className="sr-only" htmlFor="search">搜索记录</label><input id="search" type="search" maxLength={200} placeholder="搜索名称、ID、版本…" value={search} onChange={event => setSearch(event.target.value)} /><button type="submit">搜索</button></div><span className="muted small">每页 50 条 · 最新优先</span></form>
      <div className="table-wrap"><table aria-busy={result.isFetching}><thead><tr>{fields.map(([key, title]) => <th key={key} scope="col">{title}</th>)}{editable && <th scope="col">操作</th>}</tr></thead><tbody>
        {result.data?.items.map(item => <tr key={String(item.id)}>{fields.map(([key]) => <td key={key}><Cell item={item} field={key} /></td>)}{editable && <td className="actions">{section === "crashes" && <button type="button" onClick={() => setDetail(item)}>详情</button>}<ActionButton section={section} item={item} pending={mutation.isPending} act={act} /></td>}</tr>)}
        {!result.data?.items.length && <tr><td className="empty" colSpan={fields.length + Number(editable)}>{result.isPending ? "正在加载数据…" : result.isError ? "数据加载失败，请点击刷新重试。" : query ? "没有匹配的记录，请尝试其他关键词。" : "暂无数据。数据接入后将在这里展示。"}</td></tr>}
      </tbody></table></div><div className="pagination"><span>第 {page} 页 · 本页 {result.data?.items.length ?? 0} 条</span><div><button type="button" disabled={page === 1 || result.isFetching} onClick={() => setPage(page - 1)}>上一页</button><button type="button" disabled={!result.data?.has_more || result.isFetching || page >= 10000} onClick={() => setPage(page + 1)}>下一页</button></div></div>
    </section>{detail && <CrashDetail item={detail} close={() => setDetail(null)} />}
  </>;
}
function Cell({ item, field }: { item: Row; field: string }) {
  if (field === "resolved") return <span className={item.resolved ? "badge" : "badge warning"}>{item.resolved ? "已处理" : "待处理"}</span>;
  let value = String(item[field] ?? "—");
  if (field === "created_at") value = new Date(value).toLocaleString("zh-CN");
  if (field === "action") value = actionLabels[value] || value;
  return <span className="cell-text" title={value}>{value}</span>;
}
function ActionButton({ section, item, pending, act }: { section: ListPage; item: Row; pending: boolean; act: (action: string, item: Row) => void }) {
  const action = { users: "revoke_sessions", skins: "delete_skin", dictionaries: "delete_dictionary", replies: "delete_reply", crashes: item.resolved ? "reopen_crash" : "resolve_crash", downloads: "", audit: "" }[section];
  if (!action) return null;
  return <button type="button" className={action.startsWith("delete") ? "danger" : ""} disabled={pending} onClick={() => act(action, item)}>{actionLabels[action]}</button>;
}
function CrashDetail({ item, close }: { item: Row; close: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { const dialog = ref.current; dialog?.showModal(); return () => dialog?.close(); }, []);
  return <dialog ref={ref} aria-labelledby="detail-title" onCancel={close}><div className="dialog-heading"><h2 id="detail-title">崩溃详情</h2><button type="button" onClick={close}>关闭</button></div><pre>{`${item.message}\n\n${item.platform} / ${item.version}\n事件 ID: ${item.id}\n\n${item.stack || "未提供堆栈"}`}</pre></dialog>;
}
