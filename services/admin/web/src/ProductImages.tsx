import { ChangeEvent, useState } from "react";
import { ApiError, catalog } from "./api";
import type { ProductResponse, SignImageUploadResponse } from "./types/catalog";

// Mirrors the server caps (catalog domain.Image) so we fail fast before an
// upload the backend would reject anyway.
const ALLOWED_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif", "image/avif"];
const MAX_BYTES = 5 * 1024 * 1024;
const MAX_DIMENSION = 4096;

function readDimensions(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve({ width: img.naturalWidth, height: img.naturalHeight });
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("could not read image"));
    };
    img.src = url;
  });
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "something went wrong";
}

export default function ProductImages({
  product,
  onChange,
}: {
  product: ProductResponse;
  onChange: (p: ProductResponse) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const base = `/v1/products/${product.id}/images`;

  async function guard<T>(fn: () => Promise<T>) {
    setError("");
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function upload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // let the same file be re-picked after an error
    if (!file) return;
    if (!ALLOWED_TYPES.includes(file.type)) {
      setError("Unsupported type — use JPEG, PNG, WebP, GIF, or AVIF.");
      return;
    }
    if (file.size > MAX_BYTES) {
      setError("Image exceeds 5 MB.");
      return;
    }
    await guard(async () => {
      const { width, height } = await readDimensions(file);
      if (width > MAX_DIMENSION || height > MAX_DIMENSION) {
        throw new Error(`Image exceeds ${MAX_DIMENSION}px on a side.`);
      }
      // 1) mint a signed URL, 2) PUT the bytes straight to storage (no auth
      // header, Content-Type must match what was signed), 3) finalize.
      const signed = await catalog.post<SignImageUploadResponse>(`${base}:sign`, {
        content_type: file.type,
      });
      const put = await fetch(signed.upload_url, {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      });
      if (!put.ok) throw new Error("upload to storage failed");
      const updated = await catalog.post<ProductResponse>(base, {
        storage_key: signed.storage_key,
        alt_text: "",
        width,
        height,
      });
      onChange(updated);
    });
  }

  function move(index: number, dir: -1 | 1) {
    const target = index + dir;
    if (target < 0 || target >= product.images.length) return;
    const ids = product.images.map((i) => i.id);
    [ids[index], ids[target]] = [ids[target], ids[index]];
    void guard(async () => onChange(await catalog.patch<ProductResponse>(`${base}/reorder`, { image_ids: ids })));
  }

  function remove(imageId: string) {
    void guard(async () => onChange(await catalog.del<ProductResponse>(`${base}/${imageId}`)));
  }

  function saveAlt(imageId: string, altText: string, current: string) {
    if (altText === current) return;
    void guard(async () => onChange(await catalog.put<ProductResponse>(`${base}/${imageId}`, { alt_text: altText })));
  }

  return (
    <div className="gallery">
      {product.images.length === 0 && <p className="muted">No photos yet.</p>}
      <div className="thumbs">
        {product.images.map((img, i) => (
          <figure key={img.id} className="thumb">
            <img src={img.url} alt={img.alt_text} width={120} height={120} loading="lazy" />
            {i === 0 && <figcaption className="badge active">Primary</figcaption>}
            <input
              className="alt"
              defaultValue={img.alt_text}
              placeholder="Alt text"
              disabled={busy}
              onBlur={(e) => saveAlt(img.id, e.target.value, img.alt_text)}
            />
            <div className="thumb-actions">
              <button type="button" disabled={busy || i === 0} onClick={() => move(i, -1)} title="Move earlier">
                ◀
              </button>
              <button
                type="button"
                disabled={busy || i === product.images.length - 1}
                onClick={() => move(i, 1)}
                title="Move later"
              >
                ▶
              </button>
              <button type="button" className="danger" disabled={busy} onClick={() => remove(img.id)}>
                Delete
              </button>
            </div>
          </figure>
        ))}
      </div>
      <label className="upload">
        {busy ? "Working…" : "Add photo"}
        <input
          type="file"
          accept={ALLOWED_TYPES.join(",")}
          disabled={busy || product.images.length >= 10}
          onChange={upload}
          hidden
        />
      </label>
      {product.images.length >= 10 && <span className="muted"> Limit of 10 reached.</span>}
      {error && <p className="error">{error}</p>}
    </div>
  );
}
