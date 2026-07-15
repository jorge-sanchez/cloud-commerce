import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, merchants, setToken } from "../api";
import type { SessionResponse } from "../types/merchants";

export default function Login() {
  const [mode, setMode] = useState<"login" | "signup">("login");
  const [storeName, setStoreName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const session =
        mode === "login"
          ? await merchants.post<SessionResponse>("/v1/auth/login", { email, password })
          : await merchants.post<SessionResponse>("/v1/auth/signup", {
              store_name: storeName,
              email,
              password,
            });
      setToken(session.token);
      navigate("/store");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <form onSubmit={submit} className="card">
        <h1>Cloud Commerce</h1>
        <div className="tabs">
          <button
            type="button"
            className={mode === "login" ? "active" : ""}
            onClick={() => setMode("login")}
          >
            Sign in
          </button>
          <button
            type="button"
            className={mode === "signup" ? "active" : ""}
            onClick={() => setMode("signup")}
          >
            Create store
          </button>
        </div>
        {mode === "signup" && (
          <label>
            Store name
            <input value={storeName} onChange={(e) => setStoreName(e.target.value)} required />
          </label>
        )}
        <label>
          Email
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            required
          />
        </label>
        {error && <p className="error">{error}</p>}
        <button disabled={busy}>{mode === "login" ? "Sign in" : "Create store"}</button>
      </form>
    </div>
  );
}
