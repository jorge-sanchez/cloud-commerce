import { FormEvent, useCallback, useEffect, useState } from "react";
import { ApiError, merchants } from "../api";
import type { ListStaffResponse, UserResponse } from "../types/merchants";

export default function Staff() {
  const [users, setUsers] = useState<UserResponse[]>([]);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(() => {
    merchants
      .get<ListStaffResponse>("/v1/staff")
      .then((r) => setUsers(r.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "failed to load"));
  }, []);

  useEffect(load, [load]);

  async function addStaff(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await merchants.post<UserResponse>("/v1/staff", { email, password });
      setEmail("");
      setPassword("");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  async function remove(id: string) {
    setError("");
    try {
      await merchants.del(`/v1/staff/${id}`);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  return (
    <section>
      <h2>Staff</h2>
      <table className="card">
        <thead>
          <tr>
            <th>Email</th>
            <th>Role</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id}>
              <td>{u.email}</td>
              <td>{u.role}</td>
              <td>
                {u.role !== "owner" && (
                  <button className="danger" onClick={() => remove(u.id)}>
                    Remove
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3>Add staff</h3>
      <form onSubmit={addStaff} className="card">
        <label>
          Email
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
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
        <button>Add</button>
      </form>
    </section>
  );
}
