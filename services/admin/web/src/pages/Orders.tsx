import { useCallback, useEffect, useState } from "react";
import { ApiError, orders } from "../api";
import type { ListOrdersResponse, OrderResponse } from "../types/orders";

function money(cents: number, currency: string): string {
  return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(cents / 100);
}

export default function Orders() {
  const [items, setItems] = useState<OrderResponse[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    orders
      .get<ListOrdersResponse>("/v1/orders?page_size=50")
      .then((r) => setItems(r.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "failed to load"));
  }, []);

  useEffect(load, [load]);

  async function fulfill(order: OrderResponse) {
    const tracking = window.prompt("Tracking number (optional):") ?? "";
    const carrier = tracking ? (window.prompt("Carrier (optional):") ?? "") : "";
    setError("");
    try {
      await orders.post<OrderResponse>(`/v1/orders/${order.id}/fulfill`, {
        tracking_number: tracking,
        carrier,
      });
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  return (
    <section>
      <h2>Orders</h2>
      {error && <p className="error">{error}</p>}
      <table className="card">
        <thead>
          <tr>
            <th>#</th>
            <th>Date</th>
            <th>Buyer</th>
            <th>Items</th>
            <th>Total</th>
            <th>Status</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {items.map((o) => (
            <tr key={o.id}>
              <td>{o.number}</td>
              <td>{new Date(o.created_at).toLocaleDateString()}</td>
              <td>{o.email}</td>
              <td>{o.items.map((it) => `${it.qty}× ${it.sku}`).join(", ")}</td>
              <td>{money(o.total_cents, o.currency)}</td>
              <td>
                <span className={`badge ${o.status}`}>{o.status}</span>
                {o.tracking_number && (
                  <div className="hint">
                    {o.carrier ? `${o.carrier}: ` : ""}
                    {o.tracking_number}
                  </div>
                )}
              </td>
              <td>{o.status === "paid" && <button onClick={() => fulfill(o)}>Fulfill</button>}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
