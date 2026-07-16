import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, merchants, setToken } from "../api";
import type { SessionResponse } from "../types/merchants";

export default function Login() {
  const [mode, setMode] = useState<"login" | "signup">("login");
  const [storeName, setStoreName] = useState("");
  const [country, setCountry] = useState("US");
  const [taxMode, setTaxMode] = useState("exclusive");
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
              country,
              tax_mode: taxMode,
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
          <>
            <label>
              Store name
              <input value={storeName} onChange={(e) => setStoreName(e.target.value)} required />
            </label>
            <label>
              Country
              <input
                value={country}
                maxLength={2}
                onChange={(e) => {
                  const c = e.target.value.toUpperCase();
                  setCountry(c);
                  // RFC-002: derived default, confirmed explicitly below.
                  setTaxMode(c === "US" || c === "CA" ? "exclusive" : "inclusive");
                }}
                required
              />
            </label>
            <label>
              <input
                type="radio"
                checked={taxMode === "exclusive"}
                onChange={() => setTaxMode("exclusive")}
              />{" "}
              Tax added at checkout — standard in the US/Canada
            </label>
            <label>
              <input
                type="radio"
                checked={taxMode === "inclusive"}
                onChange={() => setTaxMode("inclusive")}
              />{" "}
              Prices include tax — standard in Peru, LatAm, and the EU
            </label>
          </>
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
