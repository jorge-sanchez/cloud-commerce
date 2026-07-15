import { useEffect, useState } from "react";
import { ApiError, orders } from "../api";
import type { AnalyticsSummaryResponse } from "../types/orders";

function money(cents: number, currency: string): string {
  if (!currency) return (cents / 100).toFixed(2);
  return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(cents / 100);
}

export default function Analytics() {
  const [summary, setSummary] = useState<AnalyticsSummaryResponse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    orders
      .get<AnalyticsSummaryResponse>("/v1/analytics/summary?days=30")
      .then(setSummary)
      .catch((err) => setError(err instanceof ApiError ? err.message : "failed to load"));
  }, []);

  if (error) return <p className="error">{error}</p>;
  if (!summary) return <p>Loading…</p>;

  const totalRevenue = summary.days.reduce((sum, d) => sum + d.revenue_cents, 0);
  const totalOrders = summary.days.reduce((sum, d) => sum + d.orders, 0);

  return (
    <section>
      <h2>Analytics — last 30 days</h2>
      <div className="card">
        <strong>
          {money(totalRevenue, summary.currency)} across {totalOrders} paid orders
        </strong>
      </div>

      <h3>Revenue by day</h3>
      <table className="card">
        <thead>
          <tr>
            <th>Date</th>
            <th>Orders</th>
            <th>Revenue</th>
          </tr>
        </thead>
        <tbody>
          {summary.days.map((d) => (
            <tr key={d.date}>
              <td>{d.date}</td>
              <td>{d.orders}</td>
              <td>{money(d.revenue_cents, summary.currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3>Top products</h3>
      <table className="card">
        <thead>
          <tr>
            <th>SKU</th>
            <th>Title</th>
            <th>Units</th>
            <th>Revenue</th>
          </tr>
        </thead>
        <tbody>
          {summary.top_products.map((t) => (
            <tr key={t.sku}>
              <td>{t.sku}</td>
              <td>{t.title}</td>
              <td>{t.units}</td>
              <td>{money(t.revenue_cents, summary.currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
