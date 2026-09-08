import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ShieldCheck, UserPlus } from "lucide-react";
import { z } from "zod";
import { useAuth } from "./auth";
import { actionSchema, errorMessage } from "./api";
import { PageHeader } from "./shell";

const schema = z.object({ owners: z.array(z.string()), items: z.array(z.object({ email: z.string(), enabled: z.boolean(), created_at: z.string(), updated_at: z.string(), sessions: z.number() })) });
export function AdminMembers() {
  const { api, session } = useAuth(); const client = useQueryClient();
  const [email, setEmail] = useState("");
  const query = useQuery({ queryKey: ["admin", "admins"], enabled: session?.can_manage_admins === true, queryFn: async ({ signal }) => schema.parse(await api("admins", signal)) });
  const mutation = useMutation({ mutationFn: async (value: { email: string; action: string }) => actionSchema.parse(await api("admins", undefined, value)), onSuccess: async (_, value) => { if (value.action === "add") setEmail(""); await client.invalidateQueries({ queryKey: ["admin"] }); } });
  if (!session?.can_manage_admins) return <section className="panel"><h1>管理员管理</h1><p>仅超级管理员可以管理后台访问权限。</p></section>;
  const act = (address: string, action: string) => {
    const labels: Record<string, string> = { add: "添加管理员", enable: "重新启用管理员", disable: "停用管理员并撤销全部会话", revoke: "撤销管理员全部会话" };
    if (window.confirm(`${labels[action]}：${address}？${action === "add" || action === "enable" ? "\n该账号将可以查看运营数据、撤销用户会话及删除社区内容，但不能管理管理员权限。" : ""}`)) mutation.mutate({ email: address, action });
  };
  return <><PageHeader page="admins" busy={query.isFetching} refresh={() => void query.refetch()} />
    {query.isError && <p className="notice error" role="alert">{errorMessage(query.error)}</p>}
    {mutation.isError && <p className="notice error" role="alert">{errorMessage(mutation.error)}</p>}
    {mutation.isSuccess && <p className="notice" role="status">权限更新成功，已记录审计日志。</p>}
    <section className="panel"><h2><ShieldCheck size={20} aria-hidden="true" /> 超级管理员</h2><p className="muted">由部署白名单指定，不能在此停用；调整需由运维更新配置。</p>{query.data?.owners.map(owner => <p key={owner}>{owner}<span className="badge">超级管理员</span></p>)}</section>
    <section className="list"><form className="toolbar" onSubmit={event => { event.preventDefault(); act(email.trim().toLowerCase(), "add"); }}><div><label className="sr-only" htmlFor="admin-email">Google 账号邮箱</label><input id="admin-email" type="email" required maxLength={254} value={email} onChange={event => setEmail(event.target.value)} placeholder="Google 账号邮箱" /><button type="submit" disabled={mutation.isPending || !query.data}><UserPlus size={16} aria-hidden="true" />添加管理员</button></div></form>
      <p className="muted small admin-members-help">仅支持 Google 登录，最多 100 名普通管理员。停用后立即拒绝后续请求；重新启用需重新登录。</p>
      <div className="table-wrap"><table aria-busy={query.isFetching}><thead><tr>{["邮箱", "状态", "有效会话", "添加时间", "操作"].map(label => <th key={label} scope="col">{label}</th>)}</tr></thead><tbody>{query.data?.items.map(member => <tr key={member.email}><td>{member.email}</td><td><span className={member.enabled ? "badge" : "badge warning"}>{member.enabled ? "启用" : "停用"}</span></td><td>{member.sessions}</td><td>{new Date(member.created_at).toLocaleString("zh-CN")}</td><td className="actions"><button type="button" disabled={mutation.isPending} onClick={() => act(member.email, member.enabled ? "disable" : "enable")}>{member.enabled ? "停用" : "启用"}</button><button type="button" disabled={mutation.isPending || !member.sessions} onClick={() => act(member.email, "revoke")}>撤销会话</button></td></tr>)}{!query.data?.items.length && <tr><td colSpan={5} className="empty">{query.isPending ? "正在加载…" : query.isError ? "加载失败，请重试。" : "尚未添加普通管理员。"}</td></tr>}</tbody></table></div>
    </section></>;
}
