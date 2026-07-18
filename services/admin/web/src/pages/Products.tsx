import { Fragment, FormEvent, useCallback, useEffect, useState } from "react";
import { ApiError, catalog } from "../api";
import type { ListProductsResponse, ProductResponse } from "../types/catalog";
import ProductImages from "../ProductImages";

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
  const [expanded, setExpanded] = useState<string | null>(null);
  const [error, setError] = useState("");

  // Splice an updated product back into the list without a full reload, so the
  // gallery editor's changes are reflected in the row's thumbnail immediately.
  function replace(updated: ProductResponse) {
    setProducts((ps) => ps.map((p) => (p.id === updated.id ? updated : p)));
  }

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
            <th />
            <th>Title</th>
            <th>Status</th>
            <th>Variants</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <Fragment key={p.id}>
              <tr>
                <td>
                  {p.images.length > 0 ? (
                    <img className="row-thumb" src={p.images[0].url} alt={p.images[0].alt_text} width={40} height={40} />
                  ) : (
                    <span className="row-thumb placeholder" aria-hidden="true" />
                  )}
                </td>
                <td>{p.title}</td>
                <td>
                  <span className={`badge ${p.status}`}>{p.status}</span>
                </td>
                <td>{p.variants.map((v) => v.sku).join(", ")}</td>
                <td>
                  <button className="linklike" onClick={() => setExpanded(expanded === p.id ? null : p.id)}>
                    {expanded === p.id ? "Hide photos" : `Photos (${p.images.length})`}
                  </button>
                  {p.status === "draft" && <button onClick={() => activate(p.id)}>Activate</button>}
                </td>
              </tr>
              {expanded === p.id && (
                <tr className="expand">
                  <td colSpan={5}>
                    <ProductImages product={p} onChange={replace} />
                  </td>
                </tr>
              )}
            </Fragment>
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
