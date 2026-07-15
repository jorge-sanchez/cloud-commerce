import { FormEvent, useEffect, useState } from "react";
import { ApiError, merchants } from "../api";
import type { StoreResponse } from "../types/merchants";

export default function Store() {
  const [store, setStore] = useState<StoreResponse | null>(null);
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState("");
  const [timezone, setTimezone] = useState("");
  const [supportEmail, setSupportEmail] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    merchants.get<StoreResponse>("/v1/store").then((s) => {
      setStore(s);
      setName(s.name);
      setCurrency(s.currency);
      setTimezone(s.timezone);
      setSupportEmail(s.support_email);
    });
  }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    setMessage("");
    setError("");
    try {
      const updated = await merchants.put<StoreResponse>("/v1/store", {
        name,
        currency,
        timezone,
        support_email: supportEmail,
      });
      setStore(updated);
      setMessage("Saved.");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  if (!store) return <p>Loading…</p>;

  return (
    <section>
      <h2>Store settings</h2>
      <form onSubmit={save} className="card">
        <label>
          Store name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Currency (ISO 4217)
          <input
            value={currency}
            onChange={(e) => setCurrency(e.target.value.toUpperCase())}
            maxLength={3}
            required
          />
        </label>
        <label>
          Timezone (IANA)
          <input value={timezone} onChange={(e) => setTimezone(e.target.value)} required />
        </label>
        <label>
          Support email
          <input value={supportEmail} onChange={(e) => setSupportEmail(e.target.value)} />
        </label>
        {message && <p className="ok">{message}</p>}
        {error && <p className="error">{error}</p>}
        <button>Save</button>
      </form>
    </section>
  );
}
