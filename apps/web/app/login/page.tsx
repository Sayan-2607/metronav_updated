"use client";
import { useState } from "react";
import { api, saveSession } from "@/lib/api";
import type { User } from "@/lib/types";

export default function Login() {
  const [mode, setMode] = useState<"signin" | "register">("signin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      const path = mode === "signin" ? "/api/v1/auth/login" : "/api/v1/auth/register";
      const body = mode === "signin" ? { email, password } : { email, password, name };
      const r = await api<{ token: string; user: User }>(path, { method: "POST", body: JSON.stringify(body) });
      saveSession(r.token, r.user);
      const next = new URLSearchParams(window.location.search).get("next");
      window.location.href = next && next.startsWith("/") ? next : r.user.role === "PASSENGER" ? "/" : "/admin";
    } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  }

  return (
    <div className="mx-auto max-w-sm">
      <h1 className="font-sign text-4xl font-bold">{mode === "signin" ? "Sign in" : "Create an account"}</h1>
      <form onSubmit={submit} className="mt-5 space-y-3 rounded-lg bg-panel p-5">
        {mode === "register" && (
          <label className="block"><span className="mb-1 block text-sm text-mute">Name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} className="w-full rounded-md border border-rule px-3 py-2" autoComplete="name" /></label>
        )}
        <label className="block"><span className="mb-1 block text-sm text-mute">Email</span>
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} className="w-full rounded-md border border-rule px-3 py-2" autoComplete="email" /></label>
        <label className="block"><span className="mb-1 block text-sm text-mute">Password{mode === "register" && " (at least 8 characters)"}</span>
          <input type="password" required minLength={mode === "register" ? 8 : 1} value={password} onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded-md border border-rule px-3 py-2" autoComplete={mode === "signin" ? "current-password" : "new-password"} /></label>
        {error && <p role="alert" className="text-sm text-[#C8352E]">{error}</p>}
        <button disabled={busy} className="w-full rounded-md bg-ink py-2.5 font-sign text-lg font-semibold text-panel disabled:opacity-50">
          {busy ? "Please wait…" : mode === "signin" ? "Sign in" : "Create account"}
        </button>
      </form>
      <button onClick={() => setMode(mode === "signin" ? "register" : "signin")} className="mt-3 text-sm underline underline-offset-2">
        {mode === "signin" ? "New here? Create an account" : "Already registered? Sign in"}
      </button>
      <p className="mt-6 text-xs text-mute">Local demo staff accounts are listed in the README. Change their password before sharing a deployment.</p>
    </div>
  );
}
