import { useCallback, useEffect, useState } from "react";
import { ApiError, inventory } from "../api";
import type { ListStockResponse, StockLevelResponse } from "../types/inventory";

export default function Stock() {
  const [levels, setLevels] = useState<StockLevelResponse[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    inventory
      .get<ListStockResponse>("/v1/stock?page_size=100")
      .then((r) => setLevels(r.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "failed to load"));
  }, []);

  useEffect(load, [load]);

  async function adjust(level: StockLevelResponse, delta: number) {
    setError("");
    try {
      await inventory.post<StockLevelResponse>("/v1/stock/adjust", {
        location_id: level.location_id,
        variant_id: level.variant_id,
        delta,
      });
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  return (
    <section>
      <h2>Stock</h2>
      {error && <p className="error">{error}</p>}
      <table className="card">
        <thead>
          <tr>
            <th>SKU</th>
            <th>On hand</th>
            <th>Adjust</th>
          </tr>
        </thead>
        <tbody>
          {levels.map((l) => (
            <tr key={l.id}>
              <td>{l.sku}</td>
              <td>{l.on_hand}</td>
              <td className="adjust">
                <button onClick={() => adjust(l, -1)}>-1</button>
                <button onClick={() => adjust(l, 1)}>+1</button>
                <button onClick={() => adjust(l, 10)}>+10</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="hint">
        Stock rows appear automatically when catalog products are created
        (initialized at zero via the event backbone).
      </p>
    </section>
  );
}
