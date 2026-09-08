import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { LogOut, RefreshCw, X } from "lucide-react";
import { actionSchema, errorMessage } from "./api";
import { useAuth } from "./auth";

const schema = z.object({
  id: z.string(), display_name: z.string(), created_at: z.string(), providers: z.array(z.string()),
  active_sessions: z.number(), total_sessions: z.number(), skins: z.number(), dictionaries: z.number(), replies: z.number(),
  sessions: z.array(z.object({ id: z.string(), created_at: z.string(), expires_at: z.string(), status: z.enum(["active", "expired", "revoked"]) })),
});
const statusLabels = { active: "有效", expired: "已过期", revoked: "已撤销" };
const date = (value: string) => new Date(value).toLocaleString("zh-CN");
export function UserDetail({ id, close }: { id: string; close: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const { api } = useAuth(); const client = useQueryClient();
  const result = useQuery({ queryKey: ["admin", "user", id], queryFn: async ({ signal }) => schema.parse(await api(`users/${encodeURIComponent(id)}`, signal)) });
  const mutation = useMutation({ mutationFn: async (session: string) => actionSchema.parse(await api("actions", undefined, { action: "revoke_session", id: session, user_id: id })), onSuccess: () => client.invalidateQueries({ queryKey: ["admin"] }) });
  useEffect(() => { const dialog = ref.current; dialog?.showModal(); return () => dialog?.close(); }, []);
  const user = result.data;
  return <dialog ref={ref} aria-labelledby="user-detail-title" onCancel={close}>
    <div className="dialog-heading"><h2 id="user-detail-title">用户详情</h2><div className="actions"><button type="button" disabled={result.isFetching} onClick={() => void result.refetch()}><RefreshCw size={16} aria-hidden="true" />刷新</button><button type="button" onClick={close}><X size={16} aria-hidden="true" />关闭</button></div></div>
    {result.isPending && <p role="status">正在加载用户详情…</p>}
    {result.isError && <p className="notice error" role="alert">{errorMessage(result.error)}</p>}
    {mutation.isError && <p className="notice error" role="alert">{errorMessage(mutation.error)}</p>}
    {mutation.isSuccess && <p className="notice" role="status">会话已撤销，并已记录审计日志。</p>}
    {user && <><dl className="user-facts"><dt>用户 ID</dt><dd>{user.id}</dd><dt>显示名称</dt><dd>{user.display_name || "未设置"}</dd><dt>注册时间</dt><dd>{date(user.created_at)}</dd><dt>登录渠道</dt><dd>{user.providers.join("、") || "无"}</dd><dt>发布内容</dt><dd>皮肤 {user.skins} · 词库 {user.dictionaries} · 快捷回复 {user.replies}</dd></dl>
      <h3>登录会话</h3><p className="muted small">有效 {user.active_sessions} / 共 {user.total_sessions} 条，展示最近 50 条保留记录。撤销后该会话的访问和刷新凭据立即失效。</p>
      <div className="table-wrap"><table><thead><tr><th scope="col">创建时间</th><th scope="col">到期时间</th><th scope="col">状态</th><th scope="col">操作</th></tr></thead><tbody>{user.sessions.map(session => <tr key={session.id}><td>{date(session.created_at)}</td><td>{date(session.expires_at)}</td><td>{statusLabels[session.status]}</td><td><button type="button" disabled={session.status !== "active" || mutation.isPending || result.isFetching} onClick={() => { if (window.confirm("撤销此登录会话？该设备需要重新登录。")) mutation.mutate(session.id); }}><LogOut size={16} aria-hidden="true" />撤销</button></td></tr>)}{!user.sessions.length && <tr><td colSpan={4}>暂无会话记录。</td></tr>}</tbody></table></div></>}
  </dialog>;
}
