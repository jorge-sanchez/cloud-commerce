import { useCallback, useEffect, useState } from "react";
import { ApiError, catalog, merchants, orders } from "../api";
import type { ListProductsResponse, ProductResponse, VariantResponse } from "../types/catalog";
import type { StoreResponse } from "../types/merchants";
import type { OrderResponse } from "../types/orders";

// Offline queue (ADR-010): sales wait in localStorage with a client-generated
// ID; the endpoint is idempotent on it, so flushing retries is always safe.
const QUEUE_KEY = "cc_pos_queue";

interface QueuedSale {
  client_sale_id: string;
  currency: string;
  email: string;
  lines: { variant_id: string; qty: number }[];
}

function readQueue(): QueuedSale[] {
  return JSON.parse(localStorage.getItem(QUEUE_KEY) ?? "[]");
}

function writeQueue(q: QueuedSale[]) {
  localStorage.setItem(QUEUE_KEY, JSON.stringify(q));
}

interface CartLine {
  variant: VariantResponse;
  title: string;
  qty: number;
}

export default function POS() {
  const [products, setProducts] = useState<ProductResponse[]>([]);
  const [store, setStore] = useState<StoreResponse | null>(null);
  const [cart, setCart] = useState<CartLine[]>([]);
  const [queued, setQueued] = useState(readQueue().length);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const flush = useCallback(async () => {
    let queue = readQueue();
    while (queue.length > 0) {
      try {
        await orders.post<OrderResponse>("/v1/pos/sales", queue[0]);
        queue = queue.slice(1);
        writeQueue(queue);
        setQueued(queue.length);
      } catch (err) {
        if (err instanceof ApiError && err.status === 422) {
          queue = queue.slice(1); // poison sale — drop, never block the queue
          writeQueue(queue);
          setQueued(queue.length);
          continue;
        }
        break; // offline or upstream down — retry on next flush
      }
    }
  }, []);

  useEffect(() => {
    merchants.get<StoreResponse>("/v1/store").then(setStore);
    catalog
      .get<ListProductsResponse>("/v1/products?page_size=50")
      .then((r) => setProducts(r.items.filter((p) => p.status === "active")));
    flush();
    window.addEventListener("online", flush);
    return () => window.removeEventListener("online", flush);
  }, [flush]);

  function add(product: ProductResponse, variant: VariantResponse) {
    setCart((c) => {
      const existing = c.find((l) => l.variant.id === variant.id);
      if (existing) {
        return c.map((l) => (l.variant.id === variant.id ? { ...l, qty: l.qty + 1 } : l));
      }
      return [...c, { variant, title: product.title, qty: 1 }];
    });
  }

  const total = cart.reduce((sum, l) => sum + l.variant.price_cents * l.qty, 0);

  async function charge() {
    if (!store || cart.length === 0) return;
    const sale: QueuedSale = {
      client_sale_id: crypto.randomUUID(),
      currency: store.currency,
      email: "",
      lines: cart.map((l) => ({ variant_id: l.variant.id, qty: l.qty })),
    };
    setError("");
    setCart([]);
    try {
      const order = await orders.post<OrderResponse>("/v1/pos/sales", sale);
      setMessage(`Sale #${order.number} — ${(order.total_cents / 100).toFixed(2)} ${order.currency}`);
    } catch {
      const queue = [...readQueue(), sale];
      writeQueue(queue);
      setQueued(queue.length);
      setMessage("Offline — sale queued; it will sync automatically.");
    }
  }

  return (
    <section>
      <h2>Point of sale</h2>
      {queued > 0 && (
        <p className="hint">
          {queued} sale(s) queued offline — <button className="linklike" onClick={flush}>sync now</button>
        </p>
      )}
      <div className="card">
        {products.map((p) =>
          p.variants.map((v) => (
            <button key={v.id} onClick={() => add(p, v)} style={{ margin: "0.25rem" }}>
              {p.title} {v.option_values.join("/")} — {(v.price_cents / 100).toFixed(2)}
            </button>
          )),
        )}
      </div>

      <h3>Current sale</h3>
      <table className="card">
        <tbody>
          {cart.map((l) => (
            <tr key={l.variant.id}>
              <td>
                {l.qty}× {l.title} ({l.variant.sku})
              </td>
              <td>{((l.variant.price_cents * l.qty) / 100).toFixed(2)}</td>
            </tr>
          ))}
          <tr>
            <th>Total</th>
            <th>{(total / 100).toFixed(2)} {store?.currency}</th>
          </tr>
        </tbody>
      </table>
      <button onClick={charge} disabled={cart.length === 0}>
        Charge (cash)
      </button>
      {message && <p className="ok">{message}</p>}
      {error && <p className="error">{error}</p>}
    </section>
  );
}
