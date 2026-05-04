"use client";

import { useEffect, useState } from "react";
import type { Session } from "@supabase/supabase-js";
import { supabase, type Todo } from "@/lib/supabase";

export default function Page() {
  const [session, setSession] = useState<Session | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => {
      setSession(data.session);
      setReady(true);
    });
    const { data: sub } = supabase.auth.onAuthStateChange((_e, s) => {
      setSession(s);
    });
    return () => sub.subscription.unsubscribe();
  }, []);

  if (!ready) return <main>Loading…</main>;
  return <main>{session ? <TodoList /> : <AuthForm />}</main>;
}

function AuthForm() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(mode: "signin" | "signup") {
    setBusy(true);
    setError(null);
    const fn =
      mode === "signin" ? supabase.auth.signInWithPassword : supabase.auth.signUp;
    const { error } = await fn({ email, password });
    if (error) setError(error.message);
    setBusy(false);
  }

  return (
    <>
      <h1>SupaLite todo example</h1>
      <p className="muted">Sign in or create an account.</p>
      <div className="row">
        <input
          type="email"
          placeholder="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
        />
      </div>
      <div className="row">
        <input
          type="password"
          placeholder="password (≥6 chars)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="current-password"
        />
      </div>
      {error && <div className="error">{error}</div>}
      <div className="row">
        <button onClick={() => submit("signin")} disabled={busy}>
          Sign in
        </button>
        <button
          onClick={() => submit("signup")}
          disabled={busy}
          className="ghost"
        >
          Sign up
        </button>
      </div>
    </>
  );
}

function TodoList() {
  const [todos, setTodos] = useState<Todo[]>([]);
  const [title, setTitle] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    const { data, error } = await supabase
      .from("todos")
      .select("*")
      .order("created_at", { ascending: false });
    if (error) setError(error.message);
    else setTodos(data ?? []);
  }

  useEffect(() => {
    refresh();
  }, []);

  async function add() {
    setError(null);
    const t = title.trim();
    if (!t) return;
    const user = (await supabase.auth.getUser()).data.user;
    if (!user) return;
    const { error } = await supabase.from("todos").insert({
      user_id: user.id,
      title: t,
    });
    if (error) {
      setError(error.message);
      return;
    }
    setTitle("");
    refresh();
  }

  async function toggle(t: Todo) {
    setError(null);
    const { error } = await supabase
      .from("todos")
      .update({ done: !t.done })
      .eq("id", t.id);
    if (error) setError(error.message);
    else refresh();
  }

  async function remove(t: Todo) {
    setError(null);
    const { error } = await supabase.from("todos").delete().eq("id", t.id);
    if (error) setError(error.message);
    else refresh();
  }

  return (
    <>
      <h1>Your todos</h1>
      <div className="row">
        <input
          placeholder="Add a todo…"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && add()}
        />
        <button onClick={add} disabled={!title.trim()}>
          Add
        </button>
      </div>
      {error && <div className="error">{error}</div>}
      {todos.length === 0 ? (
        <p className="muted">No todos yet. Add one above.</p>
      ) : (
        todos.map((t) => (
          <div key={t.id} className="todo">
            <input
              type="checkbox"
              checked={t.done}
              onChange={() => toggle(t)}
            />
            <span className={"title" + (t.done ? " done" : "")}>{t.title}</span>
            <button className="ghost" onClick={() => remove(t)}>
              ×
            </button>
          </div>
        ))
      )}
      <div className="row" style={{ marginTop: "1.5rem" }}>
        <button className="ghost" onClick={() => supabase.auth.signOut()}>
          Sign out
        </button>
      </div>
    </>
  );
}
