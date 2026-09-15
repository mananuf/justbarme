"use client"

import { useState } from "react"
import { Logo } from "@/components/logo"

const NAV_LINKS = [
  { label: "Features", href: "#features" },
  { label: "How it works", href: "#how-it-works" },
  { label: "Offline", href: "#offline" },
  { label: "Bills", href: "#bills" },
]

const NAV_STYLE = {
  backdropFilter: "blur(16px)",
  WebkitBackdropFilter: "blur(16px)",
  background: "rgba(244,241,232,0.55)",
  boxShadow: "0 8px 32px rgba(18,26,20,0.08), 0 2px 8px rgba(18,26,20,0.06)",
} as const

export function MobileNav() {
  const [open, setOpen] = useState(false)
  const close = () => setOpen(false)

  return (
    <div className="fixed top-4 inset-x-0 z-50 flex justify-center px-4 pointer-events-none">
      <div className="pointer-events-auto w-full max-w-3xl">
        {/* Main bar */}
        <nav
          className="flex items-center justify-between px-5 py-3 rounded-2xl border border-jb-ink/[0.08]"
          style={NAV_STYLE}
        >
          <a href="/" className="flex items-center gap-2 shrink-0">
            <Logo className="w-6 h-6 sm:w-7 sm:h-7 text-jb-ink shrink-0" />
            <span className="font-pixel text-[11px] sm:text-xs tracking-[0.2em] text-jb-ink/80">JUSTBARME</span>
          </a>

          {/* Desktop links */}
          <div className="hidden md:flex items-center gap-7">
            {NAV_LINKS.map((l) => (
              <a
                key={l.label}
                href={l.href}
                className="text-[13px] text-jb-ink/60 hover:text-jb-ink transition-colors duration-200 tracking-wide"
              >
                {l.label}
              </a>
            ))}
          </div>

          <div className="flex items-center gap-2">
            <a
              href="/onboarding"
              className="text-[13px] px-4 py-2 rounded-xl bg-jb-green-dark text-jb-cream hover:bg-jb-green transition-all duration-200 tracking-wide hidden md:block"
            >
              Get Started
            </a>

            {/* Burger — mobile only */}
            <button
              onClick={() => setOpen((v) => !v)}
              className="md:hidden flex flex-col justify-center items-center w-9 h-9 gap-[5px] rounded-lg hover:bg-jb-ink/[0.05] transition-colors"
              aria-label={open ? "Close menu" : "Open menu"}
            >
              <span
                className="block h-px bg-jb-ink/70 transition-all duration-300 origin-center"
                style={{ width: "18px", transform: open ? "translateY(6px) rotate(45deg)" : "none" }}
              />
              <span
                className="block h-px bg-jb-ink/70 transition-all duration-300"
                style={{ width: "18px", opacity: open ? 0 : 1, transform: open ? "scaleX(0)" : "none" }}
              />
              <span
                className="block h-px bg-jb-ink/70 transition-all duration-300 origin-center"
                style={{ width: "18px", transform: open ? "translateY(-6px) rotate(-45deg)" : "none" }}
              />
            </button>
          </div>
        </nav>

        {/* Mobile dropdown */}
        <div
          className="md:hidden mt-2 overflow-hidden transition-all duration-300 ease-in-out"
          style={{ maxHeight: open ? "360px" : "0px", opacity: open ? 1 : 0 }}
        >
          <div className="rounded-2xl border border-jb-ink/[0.08] px-2 py-2 flex flex-col" style={NAV_STYLE}>
            {NAV_LINKS.map((l) => (
              <a
                key={l.label}
                href={l.href}
                onClick={close}
                className="px-4 py-3.5 text-[15px] text-jb-ink/70 hover:text-jb-ink hover:bg-jb-ink/[0.04] rounded-xl transition-colors tracking-wide"
              >
                {l.label}
              </a>
            ))}
            <div className="mt-1 px-2 pb-1">
              <a
                href="/onboarding"
                className="block text-center w-full text-sm px-4 py-3.5 rounded-xl bg-jb-green-dark text-jb-cream hover:bg-jb-green transition-all duration-200 tracking-wide"
              >
                Get Started
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
