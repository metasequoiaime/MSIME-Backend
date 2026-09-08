import { createContext, useContext, useState, useCallback } from "react";
import type { ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { APIError, errorMessage, overviewSchema, requestAPI } from "./api";

type Auth = { token: string; logout: () => void; login: (token: string) => Promise<void>; api: (path: string, signal?: AbortSignal, body?: unknown) => Promise<unknown> };
const AuthContext = createContext<Auth | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState("");
  const client = useQueryClient();
  const logout = useCallback(() => { setToken(""); client.clear(); }, [client]);
  const login = async (value: string) => {
    const data = overviewSchema.parse(await requestAPI(value, "overview"));
    client.clear(); client.setQueryData(["admin", "overview"], data); setToken(value);
  };
  const api = async (path: string, signal?: AbortSignal, body?: unknown) => {
    try { return await requestAPI(token, path, signal, body); }
    catch (error) { if (error instanceof APIError && error.status === 401) logout(); throw error; }
  };
  return <AuthContext.Provider value={{ token, logout, login, api }}>{children}</AuthContext.Provider>;
}
export function useAuth() { const value = useContext(AuthContext); if (!value) throw new Error("AuthProvider required"); return value; }

export function Login() {
  const { login } = useAuth();
  const [value, setValue] = useState(""); const [pending, setPending] = useState(false); const [error, setError] = useState("");
  return <section className="login"><div className="login-panel"><span className="brand-icon">杉</span><p className="eyebrow">MSIME / ADMIN</p><h1>水杉管理控制台</h1><p className="muted">了解产品使用情况，管理社区内容。</p>
    <form onSubmit={async (event) => { event.preventDefault(); setPending(true); setError(""); try { await login(value.trim()); setValue(""); } catch (e) { setError(errorMessage(e)); } finally { setPending(false); } }}>
      <label htmlFor="token">管理员密钥</label><input id="token" type="password" required autoComplete="off" value={value} onChange={event => setValue(event.target.value)} placeholder="输入管理员密钥" />
      <button className="primary" type="submit" disabled={pending}>{pending ? "正在验证…" : "进入控制台"}<span>→</span></button>
    </form><p className="muted small">密钥仅保留在当前页面内存中，关闭或刷新后需重新登录。</p>{error && <p className="error" role="alert">{error}</p>}
  </div><div className="login-art"><div className="rings" /><p>让每一次输入<br />都更自然。</p><span>水杉输入法 · MSIME</span></div></section>;
}
