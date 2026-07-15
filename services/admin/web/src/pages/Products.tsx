import { FormEvent, useCallback, useEffect, useState } from "react";
import { ApiError, catalog } from "../api";
import type { ListProductsResponse, ProductResponse } from "../types/catalog";

interface VariantDraft {
  sku: string;
  option_values: string;
  price: string;
}

export default function Products() {
  const [products, setProducts] = useState<ProductResponse[]>([]);
  const [title, setTitle] = useState("");
  const [options, setOptions] = useState("");
  const [variants, setVariants] = useState<VariantDraft[]>([
    { sku: "", option_values: "", price: "" },
  ]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    catalog
      .get<ListProductsResponse>("/v1/products?page_size=50")
      .then((r) => setProducts(r.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "failed to load"));
  }, []);

  useEffect(load, [load]);

  async function create(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await catalog.post<ProductResponse>("/v1/products", {
        title,
        options: options
          .split(",")
          .map((o) => o.trim())
          .filter(Boolean),
        variants: variants.map((v) => ({
          sku: v.sku,
          option_values: v.option_values
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean),
          price_cents: Math.round(parseFloat(v.price || "0") * 100),
        })),
      });
      setTitle("");
      setOptions("");
      setVariants([{ sku: "", option_values: "", price: "" }]);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  async function activate(id: string) {
    setError("");
    try {
      await catalog.post<ProductResponse>(`/v1/products/${id}/activate`);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "something went wrong");
    }
  }

  function setVariant(i: number, patch: Partial<VariantDraft>) {
    setVariants((vs) => vs.map((v, j) => (j === i ? { ...v, ...patch } : v)));
  }

  return (
    <section>
      <h2>Products</h2>
      <table className="card">
        <thead>
          <tr>
            <th>Title</th>
            <th>Status</th>
            <th>Variants</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <tr key={p.id}>
              <td>{p.title}</td>
              <td>
                <span className={`badge ${p.status}`}>{p.status}</span>
              </td>
              <td>{p.variants.map((v) => v.sku).join(", ")}</td>
              <td>
                {p.status === "draft" && (
                  <button onClick={() => activate(p.id)}>Activate</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3>New product</h3>
      <form onSubmit={create} className="card">
        <label>
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} required />
        </label>
        <label>
          Options (comma-separated, e.g. Size, Color)
          <input value={options} onChange={(e) => setOptions(e.target.value)} />
        </label>
        <h4>Variants</h4>
        {variants.map((v, i) => (
          <div className="variant-row" key={i}>
            <input
              placeholder="SKU"
              value={v.sku}
              onChange={(e) => setVariant(i, { sku: e.target.value })}
              required
            />
            <input
              placeholder="Option values (S, Red)"
              value={v.option_values}
              onChange={(e) => setVariant(i, { option_values: e.target.value })}
            />
            <input
              placeholder="Price"
              type="number"
              step="0.01"
              min="0"
              value={v.price}
              onChange={(e) => setVariant(i, { price: e.target.value })}
              required
            />
          </div>
        ))}
        <button
          type="button"
          className="linklike"
          onClick={() => setVariants((vs) => [...vs, { sku: "", option_values: "", price: "" }])}
        >
          + variant
        </button>
        {error && <p className="error">{error}</p>}
        <button>Create</button>
      </form>
    </section>
  );
}
