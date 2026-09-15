import { useEffect, useRef } from 'react';

// Each icon is drawn on a small pixel grid, animated at 60fps with RAF.
// Colors are the brand ink at varying opacity so the icon sits quietly on cream.

type IconType =
  'notebook' | 'sell' | 'stock' | 'bills' | 'offline' | 'reports' | 'catalogue' | 'people';

interface PixelIconProps {
  type: IconType;
  size?: number; // rendered px size (default 40)
}

const INK = '22,32,26'; // rgb of --jb-ink, used as rgba(INK, alpha)

// ── Notebook icon: connected pages / flow nodes pulsing ──────────────────────
function drawNotebook(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const cx = W / 2,
    cy = W / 2;
  const r = W * 0.34;
  const ps = W / 12;
  const pulse = 0.6 + 0.4 * Math.sin(t * 0.003);
  ctx.fillStyle = `rgba(${INK},${pulse})`;
  const cs = ps * 1.4;
  ctx.fillRect(cx - cs / 2, cy - cs / 2, cs, cs);

  const nodeCount = 5;
  for (let i = 0; i < nodeCount; i++) {
    const angle = (i / nodeCount) * Math.PI * 2 + t * 0.0012;
    const nx = cx + Math.cos(angle) * r;
    const ny = cy + Math.sin(angle) * r;
    const opacity = 0.3 + 0.5 * ((Math.sin(angle * 2 + t * 0.002) + 1) / 2);
    ctx.fillStyle = `rgba(${INK},${opacity})`;
    ctx.fillRect(Math.round(nx / ps) * ps - ps / 2, Math.round(ny / ps) * ps - ps / 2, ps, ps);

    const steps = 5;
    for (let s = 1; s < steps; s++) {
      const lx = cx + (nx - cx) * (s / steps);
      const ly = cy + (ny - cy) * (s / steps);
      const lo = (0.06 + 0.1 * (s / steps)) * pulse;
      ctx.fillStyle = `rgba(${INK},${lo})`;
      ctx.fillRect(Math.round(lx / ps) * ps, Math.round(ly / ps) * ps, ps * 0.7, ps * 0.7);
    }
  }
}

// ── Sell icon: a glass filling and settling, like a poured drink ─────────────
function drawSell(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const ps = Math.floor(W / 12);
  const rows = 9,
    cols = 6;
  const offX = Math.floor((W - cols * ps) / 2);
  const offY = Math.floor((W - rows * ps) / 2);

  // glass outline: wider top, narrower base (trapezoid mask by row)
  const widthAtRow = (r: number) => cols - Math.floor(r / 3.2);

  const period = 2600;
  const fill = (t % period) / period; // 0 -> 1 -> resets (pour cycle)
  const filledRows = Math.floor(fill * rows);

  for (let r = 0; r < rows; r++) {
    const w = Math.max(2, widthAtRow(r));
    const rowOffX = offX + Math.floor((cols - w) / 2);
    const isFilled = r >= rows - filledRows;
    for (let c = 0; c < w; c++) {
      const edge = c === 0 || c === w - 1 || r === rows - 1;
      let alpha = edge ? 0.22 : 0.06;
      if (isFilled) alpha = edge ? 0.55 : 0.4;
      ctx.fillStyle = `rgba(${INK},${alpha})`;
      ctx.fillRect(rowOffX + c * ps, offY + r * ps, ps - 1, ps - 1);
    }
  }
}

// ── Stock icon: crates stacking / count settling ──────────────────────────────
function drawStock(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const ps = Math.floor(W / 12);
  const crates = 3;
  const cw = ps * 3;
  const ch = ps * 2.2;
  const gap = ps * 0.4;
  const totalH = crates * ch + (crates - 1) * gap;
  const offX = Math.floor((W - cw) / 2);
  let y = W - (W - totalH) / 2 - ch;

  for (let i = 0; i < crates; i++) {
    const bob = Math.sin(t * 0.0018 + i * 1.4) * 1.4;
    const alpha = 0.22 + 0.5 * ((i + 1) / crates);
    ctx.fillStyle = `rgba(${INK},${alpha})`;
    ctx.fillRect(offX, y + bob, cw, ch - 2);
    // crate slats
    ctx.fillStyle = `rgba(${INK},${Math.min(0.9, alpha + 0.25)})`;
    for (let s = 1; s < 3; s++) {
      ctx.fillRect(offX + (cw / 3) * s - 1, y + bob, 1.5, ch - 2);
    }
    y -= ch + gap;
  }
}

// ── Bills icon: a ticket/receipt with a pulsing tear line ────────────────────
function drawBills(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const ps = Math.floor(W / 12);
  const w = ps * 7;
  const h = ps * 9;
  const offX = Math.floor((W - w) / 2);
  const offY = Math.floor((W - h) / 2);

  ctx.fillStyle = `rgba(${INK},0.14)`;
  ctx.fillRect(offX, offY, w, h);

  // lines of "text"
  const lineCount = 4;
  for (let i = 0; i < lineCount; i++) {
    const reveal = Math.min(1, Math.max(0, (t * 0.0006 - i * 0.4) % 2.4));
    const lw = w * 0.55 * reveal;
    ctx.fillStyle = `rgba(${INK},0.5)`;
    ctx.fillRect(offX + ps, offY + ps * (2 + i * 1.6), lw, ps * 0.6);
  }

  // pulsing balance chip bottom
  const pulse = 0.4 + 0.5 * Math.sin(t * 0.004);
  ctx.fillStyle = `rgba(${INK},${pulse})`;
  ctx.fillRect(offX + ps, offY + h - ps * 2, w - ps * 2, ps * 0.9);
}

// ── Offline icon: wifi arcs sweeping, with a device dot that stays lit ───────
function drawOffline(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const cx = W / 2;
  const cy = W * 0.68;
  const ps = Math.max(2, Math.floor(W / 20));

  ctx.fillStyle = `rgba(${INK},0.85)`;
  ctx.fillRect(cx - ps, cy - ps / 2, ps * 2, ps * 2);

  const arcs = 3;
  for (let i = 0; i < arcs; i++) {
    const radius = (i + 1) * (W * 0.14);
    const phase = (t * 0.0009 + i * 0.6) % 2.4;
    const alpha = phase < 1.6 ? Math.max(0, 0.7 - phase * 0.4) : 0;
    if (alpha <= 0) continue;
    ctx.fillStyle = `rgba(${INK},${alpha})`;
    const steps = 9;
    for (let s = 0; s <= steps; s++) {
      const angle = Math.PI + (s / steps) * Math.PI;
      const x = cx + Math.cos(angle) * radius;
      const y = cy + Math.sin(angle) * radius * 0.9;
      ctx.fillRect(Math.round(x / ps) * ps, Math.round(y / ps) * ps, ps - 1, ps - 1);
    }
  }
}

// ── Reports icon: bars growing, like a daily-sales chart ─────────────────────
function drawReports(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const ps = Math.floor(W / 12);
  const bars = 4;
  const bw = ps * 1.6;
  const gap = ps * 0.6;
  const total = bars * bw + (bars - 1) * gap;
  const offX = Math.floor((W - total) / 2);
  const maxH = W * 0.66;

  const heights = [0.4, 0.65, 0.5, 0.8];
  heights.forEach((h, i) => {
    const wave = Math.sin(t * 0.0016 + i) * 0.06;
    const animated = Math.max(0.12, h + wave);
    const bh = animated * maxH;
    const x = offX + i * (bw + gap);
    const y = W - bh - ps;

    const rowCount = Math.max(1, Math.floor(bh / ps));
    for (let row = 0; row < rowCount; row++) {
      const progress = 1 - row / rowCount;
      const alpha = 0.16 + progress * 0.6;
      ctx.fillStyle = `rgba(${INK},${alpha})`;
      ctx.fillRect(x, y + row * ps, bw, ps - 1);
    }
  });
}

// ── Catalogue icon: grid of product tiles lighting up in sequence ────────────
function drawCatalogue(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const cols = 4,
    rows = 3;
  const ps = Math.floor(W / (cols + 1.6));
  const gap = 3;
  const offX = Math.floor((W - cols * (ps + gap)) / 2);
  const offY = Math.floor((W - rows * (ps + gap)) / 2);
  const total = cols * rows;
  const wave = t * 0.0009;

  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      const idx = r * cols + c;
      const phase = (idx / total) * Math.PI * 2;
      const alpha = 0.1 + 0.6 * ((Math.sin(wave + phase) + 1) / 2);
      ctx.fillStyle = `rgba(${INK},${alpha})`;
      ctx.fillRect(offX + c * (ps + gap), offY + r * (ps + gap), ps, ps);
    }
  }
}

// ── People icon: staff figures, steady (accountability, not motion) ─────────
function drawPeople(ctx: CanvasRenderingContext2D, W: number, t: number) {
  const ps = Math.floor(W / 14);
  const figures = [-1, 0, 1];
  const cy = W / 2;
  figures.forEach((offset, i) => {
    const cx = W / 2 + offset * ps * 3.2;
    const alpha = offset === 0 ? 0.75 : 0.35 + 0.15 * Math.sin(t * 0.002 + i);
    ctx.fillStyle = `rgba(${INK},${alpha})`;
    // head
    ctx.fillRect(cx - ps / 2, cy - ps * 2.6, ps, ps);
    // body (trapezoid-ish via rect)
    ctx.fillRect(cx - ps * 1.1, cy - ps * 1.3, ps * 2.2, ps * 1.8);
  });
}

// ── Canvas wrapper ────────────────────────────────────────────────────────────
export function PixelIcon({ type, size = 40 }: PixelIconProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rafRef = useRef<number>(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d')!;

    const draw = (t: number) => {
      const dpr = window.devicePixelRatio || 1;
      canvas.width = size * dpr;
      canvas.height = size * dpr;
      ctx.scale(dpr, dpr);
      ctx.clearRect(0, 0, size, size);
      ctx.imageSmoothingEnabled = false;

      switch (type) {
        case 'notebook':
          drawNotebook(ctx, size, t);
          break;
        case 'sell':
          drawSell(ctx, size, t);
          break;
        case 'stock':
          drawStock(ctx, size, t);
          break;
        case 'bills':
          drawBills(ctx, size, t);
          break;
        case 'offline':
          drawOffline(ctx, size, t);
          break;
        case 'reports':
          drawReports(ctx, size, t);
          break;
        case 'catalogue':
          drawCatalogue(ctx, size, t);
          break;
        case 'people':
          drawPeople(ctx, size, t);
          break;
      }

      rafRef.current = requestAnimationFrame(draw);
    };

    rafRef.current = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(rafRef.current);
  }, [type, size]);

  return (
    <canvas
      ref={canvasRef}
      style={{
        width: size,
        height: size,
        imageRendering: 'pixelated',
        display: 'block',
        flexShrink: 0,
      }}
    />
  );
}
