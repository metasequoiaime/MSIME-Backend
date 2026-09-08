import { z } from "zod";

export const overviewSchema = z.object({
  users: z.number(), new_users_30d: z.number(), session_users: z.number(),
  downloads: z.number(), crashes: z.number(), open_crashes: z.number(),
  skins: z.number(), skin_downloads: z.number(), dictionaries: z.number(),
  replies: z.number(), resource_saves: z.number(),
  daily: z.array(z.object({ day: z.string(), users: z.number(), downloads: z.number(), crashes: z.number() })).length(30),
});
export type Overview = z.infer<typeof overviewSchema>;
export const listSchema = z.object({
  items: z.array(z.record(z.string(), z.union([z.string(), z.number(), z.boolean(), z.null()]))),
  page: z.number().int().positive(), has_more: z.boolean(),
});
export type Row = z.infer<typeof listSchema>["items"][number];
export const actionSchema = z.object({ ok: z.literal(true), affected: z.number() });

export class APIError extends Error {
  readonly status: number;
  constructor(status: number, message: string) { super(message); this.status = status; }
}
export async function requestAPI(token: string, path: string, signal?: AbortSignal, body?: unknown): Promise<unknown> {
  const response = await fetch(`/api/${path}`, {
    method: body ? "POST" : "GET", signal,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
  if (!response.ok) throw new APIError(response.status, response.status === 401 ? "管理员密钥无效，请重新登录。" : response.status === 429 ? "请求过于频繁，请稍后重试。" : `请求失败 (${response.status})，请检查服务和数据库状态。`);
  return response.json();
}
export function errorMessage(error: unknown): string {
  if (error instanceof z.ZodError) return "服务返回的数据格式不正确，请检查前后端版本是否一致。";
  return error instanceof Error ? error.message : "请求失败，请重试。";
}
