import type { ReactNode } from "react"

// A lightweight mobile-device frame used to present realistic justbarme
// screens throughout the marketing site. Kept intentionally simple (no
// hardware bezel illustration) so it stays crisp and fast on low-end phones.
export function PhoneFrame({
  children,
  className = "",
  statusLabel = "9:41",
}: {
  children: ReactNode
  className?: string
  statusLabel?: string
}) {
  return (
    <div className={`relative w-full max-w-[300px] mx-auto ${className}`}>
      <div className="relative rounded-[2.25rem] border border-jb-ink/[0.12] bg-jb-ink shadow-[0_30px_60px_-20px_rgba(18,26,20,0.35)] p-2">
        {/* Notch */}
        <div className="absolute left-1/2 top-2 -translate-x-1/2 w-20 h-4 rounded-full bg-jb-ink z-20" />
        <div className="relative rounded-[1.75rem] overflow-hidden bg-jb-cream">
          {/* Status bar */}
          <div className="flex items-center justify-between px-5 pt-2.5 pb-1 text-[10px] font-medium text-jb-ink/70">
            <span>{statusLabel}</span>
            <div className="flex items-center gap-1">
              <span className="w-3 h-2 rounded-[2px] border border-jb-ink/50" />
              <span className="w-2.5 h-2 rounded-full border border-jb-ink/50" />
            </div>
          </div>
          <div className="min-h-[420px] flex flex-col">{children}</div>
        </div>
      </div>
    </div>
  )
}
