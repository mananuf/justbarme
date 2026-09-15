"use client"

import React, { useRef, useEffect, useState, useCallback } from "react"
import { PixelIcon } from "@/components/pixel-icon"
import { RevealText } from "@/components/reveal-text"
import { MobileNav } from "@/components/mobile-nav"
import { PhoneFrame } from "@/components/phone-frame"
import { Logo } from "@/components/logo"
import { IntroAnimation, HERO_REVEAL_MS } from "@/components/intro-animation"
import {
  DashboardPreview,
  QuickSellPreview,
  StockReceivePreview,
  OutstandingBillPreview,
  BrandedBillPreview,
  CatalogueSetupPreview,
  OfflineSyncBadge,
} from "@/components/ui-previews"

// ─── Intersection Observer hook ──────────────────────────────────────────────
function useInView(threshold = 0.15) {
  const ref = useRef<HTMLDivElement>(null)
  const [inView, setInView] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const obs = new IntersectionObserver(([e]) => { if (e.isIntersecting) setInView(true) }, { threshold })
    obs.observe(el)
    return () => obs.disconnect()
  }, [threshold])
  return { ref, inView }
}

// ─── Bento card ──────────────────────────────────────────────────────────────
function BentoCard({ children, className = "", delay = 0 }: { children: React.ReactNode; className?: string; delay?: number }) {
  const { ref, inView } = useInView(0.1)
  return (
    <div
      ref={ref}
      className={`group relative rounded-2xl border border-jb-ink/[0.08] bg-white/70 overflow-hidden transition-all duration-700 hover:border-jb-ink/[0.16] hover:bg-white ${className}`}
      style={{
        opacity: inView ? 1 : 0,
        transform: inView ? "translateY(0)" : "translateY(28px)",
        transition: `opacity 0.7s ease ${delay}ms, transform 0.7s ease ${delay}ms, border-color 0.3s ease, background-color 0.3s ease`,
      }}
    >
      {children}
    </div>
  )
}

// ─── Pill tag ─────────────────────────────────────────────────────────────────
function Tag({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] tracking-widest font-medium text-jb-ink/45 bg-jb-ink/[0.05]">
      {children}
    </span>
  )
}

function SectionIntro({
  icon,
  tag,
  title,
  id,
}: {
  icon: Parameters<typeof PixelIcon>[0]["type"]
  tag: string
  title: string
  id?: string
}) {
  return (
    <div id={id} className="mb-14 scroll-mt-28">
      <PixelIcon type={icon} size={40} />
      <div className="mt-4"><Tag>{tag}</Tag></div>
      <RevealText className="mt-5 text-3xl md:text-4xl lg:text-5xl font-light tracking-tight leading-[1.08] text-jb-ink">
        {title}
      </RevealText>
    </div>
  )
}

const PROBLEMS = [
  "You don't know exactly what was sold today.",
  "Stock counts never quite match what's on the shelf.",
  "Nobody remembers who recorded which sale.",
  "Customer debts get forgotten until it's too late.",
  "Owners and staff all have different numbers.",
  "Closing time turns into an hour of reconciling.",
  "A network drop interrupts the whole operation.",
]

const FLOW = ["Stock", "Sales", "Bills", "Payments", "Expenses", "Reports"]

const FEATURES: { icon: Parameters<typeof PixelIcon>[0]["type"]; title: string; desc: string }[] = [
  { icon: "sell", title: "Sales", desc: "Record drinks and other sales in one or two taps, from any table or the counter." },
  { icon: "stock", title: "Stock", desc: "Know what came in, what was sold and what should remain — without a spreadsheet." },
  { icon: "bills", title: "Tables & Tabs", desc: "Keep track of open customer bills and table orders as the night goes on." },
  { icon: "bills", title: "Outstanding Payments", desc: "See who owes the business and send a branded reminder in a tap." },
  { icon: "notebook", title: "Expenses", desc: "Record money spent on ice, transport, fuel, supplies and more." },
  { icon: "people", title: "Staff Activity", desc: "Know who did what and when — every sale, payment and adjustment." },
  { icon: "reports", title: "Reports", desc: "Understand daily sales, stock movement and expenses at a glance." },
  { icon: "offline", title: "Offline Mode", desc: "Keep recording operations even when the internet goes down." },
]

const CUSTOMIZATIONS = [
  "Products", "Drink sizes", "Selling prices", "Tables",
  "Expense categories", "Stock adjustment reasons", "Receipt & bill branding", "Staff access",
]

const OPERATIONS = [
  { n: "01", title: "Record a sale", desc: "Select product → Select quantity → Done." },
  { n: "02", title: "Receive stock", desc: "Select product → Enter quantity → Save." },
  { n: "03", title: "Send a bill", desc: "Open bill → Share → Done." },
  { n: "04", title: "Record payment", desc: "Open outstanding bill → Record payment → Done." },
]

const AUDIENCE = ["Local bars", "Lounges", "Beer parlours", "Clubs", "Small hospitality businesses", "One-owner bars", "Multi-owner lounges", "Bars with several staff"]

const STEPS = [
  { n: "01", title: "Create your business", desc: "Enter your business name and basic details. That's it to get started." },
  { n: "02", title: "Choose what you sell", desc: "Select common drinks from the catalogue, or add your own products." },
  { n: "03", title: "Set your prices", desc: "Configure prices, sizes, tables and other preferences — editable anytime." },
  { n: "04", title: "Start operating", desc: "Record sales, stock, bills and expenses from your phone. Online or offline." },
]

export default function JustBarmePage() {
  const [heroReady, setHeroReady] = useState(false)
  const handleIntroDone = useCallback(() => setHeroReady(true), [])

  // Fallback: if the intro is skipped/interrupted for any reason, still reveal the hero.
  useEffect(() => {
    const t = setTimeout(() => setHeroReady(true), HERO_REVEAL_MS + 400)
    return () => clearTimeout(t)
  }, [])

  const reveal = (delayMs: number) => ({
    opacity: heroReady ? 1 : 0,
    filter: heroReady ? "blur(0px)" : "blur(16px)",
    transform: heroReady ? "translateY(0px)" : "translateY(18px)",
    transition: `opacity 0.8s cubic-bezier(0.16,1,0.3,1) ${delayMs}ms, filter 0.8s cubic-bezier(0.16,1,0.3,1) ${delayMs}ms, transform 0.8s cubic-bezier(0.16,1,0.3,1) ${delayMs}ms`,
  })

  return (
    <div className="bg-jb-cream text-jb-ink min-h-screen font-sans antialiased">
      <IntroAnimation onDone={handleIntroDone} />
      <MobileNav />

      {/* ── HERO ──────────────────────────────────────────────────────────── */}
      <section className="relative px-6 md:px-12 lg:px-20 pt-28 pb-16 md:pt-40 md:pb-24 overflow-hidden">
        <div className="max-w-6xl mx-auto grid md:grid-cols-2 gap-14 items-center">
          <div>
            <h1
              className="text-[2.75rem] leading-[1.02] sm:text-6xl md:text-6xl lg:text-7xl font-light tracking-tight text-jb-ink"
              style={{ fontFamily: "var(--font-display)", ...reveal(0) }}
            >
              Run your bar.
              <br />
              Simply.
            </h1>
            <p className="mt-6 text-base md:text-lg text-jb-ink/55 leading-relaxed max-w-md" style={reveal(120)}>
              Keep track of sales, stock, customer bills, expenses and staff activity from one simple app built for bars and lounges.
            </p>
            <div className="mt-8 flex flex-col sm:flex-row gap-3" style={reveal(200)}>
              <a href="/onboarding" className="px-7 py-3.5 rounded-xl bg-jb-ink text-jb-cream text-sm font-medium text-center hover:bg-jb-green transition-colors active:scale-[0.98]">
                Get Started
              </a>
              <a href="#how-it-works" className="px-7 py-3.5 rounded-xl border border-jb-ink/15 text-jb-ink text-sm font-medium text-center hover:bg-jb-ink/[0.04] transition-colors">
                See How It Works
              </a>
            </div>
            <div className="mt-10 flex flex-wrap gap-x-6 gap-y-2.5" style={reveal(280)}>
              {["Works offline", "Set up in minutes", "Built for Android"].map((label) => (
                <div key={label} className="flex items-center gap-2 text-xs text-jb-ink/45 tracking-wide">
                  <span className="w-1.5 h-1.5 rounded-full bg-jb-ink/30" />
                  {label}
                </div>
              ))}
            </div>
          </div>

          <div style={reveal(160)}>
            <PhoneFrame>
              <DashboardPreview />
            </PhoneFrame>
          </div>
        </div>
      </section>

      {/* ── PROBLEM ───────────────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto">
          <div className="mb-14 max-w-2xl">
            <Tag>THE PROBLEM</Tag>
            <RevealText className="mt-5 text-3xl md:text-4xl lg:text-5xl font-light tracking-tight leading-[1.1] text-jb-ink">
              {"Your bar shouldn't run on\nmemory, notebooks and\nWhatsApp messages."}
            </RevealText>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {PROBLEMS.map((p, i) => (
              <BentoCard key={p} className="p-5 flex items-start gap-3" delay={i * 60}>
                <span className="mt-1 w-1.5 h-1.5 rounded-full bg-jb-ink/25 shrink-0" />
                <p className="text-[14px] text-jb-ink/65 leading-relaxed">{p}</p>
              </BentoCard>
            ))}
          </div>
        </div>
      </section>

      {/* ── PRODUCT OVERVIEW ─────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto">
          <div className="mb-14 max-w-2xl">
            <Tag>THE IDEA</Tag>
            <RevealText className="mt-5 text-3xl md:text-4xl lg:text-5xl font-light tracking-tight leading-[1.1] text-jb-ink">
              {"Everything important about\nyour bar, in one place."}
            </RevealText>
            <p className="mt-5 text-sm text-jb-ink/50 leading-relaxed">
              Set it up once. Run it in a few taps. justbarme quietly connects the everyday parts of running a bar so nothing falls through the cracks.
            </p>
          </div>
          <BentoCard className="p-8 md:p-12">
            <div className="flex flex-wrap items-center justify-center gap-3">
              {FLOW.map((step, i) => (
                <React.Fragment key={step}>
                  <div className="px-4 py-2.5 rounded-xl bg-jb-ink text-jb-cream text-sm font-medium tracking-wide">{step}</div>
                  {i < FLOW.length - 1 && <span className="text-jb-ink/25 text-lg">→</span>}
                </React.Fragment>
              ))}
            </div>
          </BentoCard>
        </div>
      </section>

      {/* ── CORE FEATURES ────────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto">
          <SectionIntro id="features" icon="catalogue" tag="FEATURES" title={"Everything you need to\nrun a bar, nothing you don't."} />
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            {FEATURES.map((f, i) => (
              <BentoCard key={f.title} className="p-6 min-h-[168px] flex flex-col justify-between" delay={i * 60}>
                <div className="w-9 h-9 rounded-xl border border-jb-ink/10 flex items-center justify-center mb-5">
                  <PixelIcon type={f.icon} size={18} />
                </div>
                <div>
                  <h3 className="text-[15px] font-medium mb-1.5 text-jb-ink">{f.title}</h3>
                  <p className="text-[13px] text-jb-ink/50 leading-relaxed">{f.desc}</p>
                </div>
              </BentoCard>
            ))}
          </div>
        </div>
      </section>

      {/* ── BUILT AROUND YOUR BAR ────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto">
          <SectionIntro icon="people" tag="MADE TO FIT" title={"Your bar works differently.\njustbarme adapts."} />
          <div className="flex flex-wrap gap-2.5 mb-10">
            {CUSTOMIZATIONS.map((c) => (
              <span key={c} className="px-4 py-2 rounded-full border border-jb-ink/10 bg-white/60 text-[13px] text-jb-ink/65">
                {c}
              </span>
            ))}
          </div>
          <p className="text-2xl md:text-3xl font-light tracking-tight text-jb-ink max-w-xl leading-snug">
            Flexible for owners. <span className="text-jb-ink/40">Simple for staff.</span>
          </p>
        </div>
      </section>

      {/* ── PREFILLED CATALOGUE ──────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto grid md:grid-cols-2 gap-14 items-center">
          <div>
            <PixelIcon type="catalogue" size={40} />
            <div className="mt-4"><Tag>QUICK SETUP</Tag></div>
            <RevealText className="mt-5 text-3xl md:text-4xl lg:text-5xl font-light tracking-tight leading-[1.1] text-jb-ink">
              {"Start with the drinks\nyou already sell."}
            </RevealText>
            <p className="mt-5 text-sm text-jb-ink/50 leading-relaxed max-w-md">
              Pick from a prefilled catalogue of common Nigerian drinks — Guinness, Star, Heineken, Trophy, Jameson, Hennessy, Gordon&apos;s, Coke, Fanta, Sprite, Water — instead of typing everything from scratch.
            </p>
            <ul className="mt-6 space-y-2.5">
              {["Select product sizes", "Set your own prices", "Add custom products anytime", "Hide products you don't sell"].map((x) => (
                <li key={x} className="flex items-center gap-2.5 text-sm text-jb-ink/60">
                  <span className="w-1 h-1 rounded-full bg-jb-ink/30" /> {x}
                </li>
              ))}
            </ul>
          </div>
          <PhoneFrame>
            <CatalogueSetupPreview />
          </PhoneFrame>
        </div>
      </section>

      {/* ── ONE / TWO TAP OPERATIONS ─────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto">
          <SectionIntro icon="sell" tag="DESIGNED FOR THE BUSY NIGHT" title={"Less typing.\nMore running your bar."} />
          <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
            {OPERATIONS.map((step, i) => (
              <BentoCard key={step.n} className="p-7 min-h-[180px] flex flex-col" delay={i * 80}>
                <span className="font-pixel text-[11px] text-jb-ink/25 tracking-widest block mb-6">{step.n}</span>
                <h3 className="text-lg font-medium mb-2 text-jb-ink mt-auto">{step.title}</h3>
                <p className="text-[13px] text-jb-ink/50 leading-relaxed">{step.desc}</p>
              </BentoCard>
            ))}
          </div>
          <p className="mt-6 text-sm text-jb-ink/40 max-w-lg">
            Checking today&apos;s sales is even simpler: open the dashboard, see the summary. No taps required.
          </p>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-8 mt-16 max-w-2xl mx-auto">
            <div>
              <p className="text-center text-[13px] text-jb-ink/45 mb-4">Recording a sale</p>
              <PhoneFrame>
                <QuickSellPreview />
              </PhoneFrame>
            </div>
            <div>
              <p className="text-center text-[13px] text-jb-ink/45 mb-4">Settling an outstanding tab</p>
              <PhoneFrame>
                <OutstandingBillPreview />
              </PhoneFrame>
            </div>
          </div>
        </div>
      </section>

      {/* ── OFFLINE ───────────────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto grid md:grid-cols-2 gap-14 items-center">
          <div>
            <SectionIntro id="offline" icon="offline" tag="OFFLINE MODE" title={"Your bar keeps moving,\neven when the network doesn't."} />
            <p className="text-sm text-jb-ink/55 leading-relaxed max-w-md">
              If the internet goes down, you can keep recording sales and other activity. Pending activity clearly shows it&apos;s waiting, and synchronizes as soon as the connection returns.
            </p>
            <p className="mt-4 text-sm text-jb-ink/40 leading-relaxed max-w-md">
              We keep this honest: your activity is kept safe on your device, but two disconnected phones won&apos;t always see the exact same numbers in the moment. justbarme flags anything that needs a second look once you&apos;re back online.
            </p>
            <div className="mt-6"><OfflineSyncBadge /></div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <BentoCard className="p-6" delay={0}>
              <div className="text-[10px] tracking-widest text-jb-ink/35 mb-3">NO CONNECTION</div>
              <p className="text-[13px] text-jb-ink/60 leading-relaxed">Sales, payments and stock are saved on the device right away — nothing waits on a signal.</p>
            </BentoCard>
            <BentoCard className="p-6" delay={100}>
              <div className="text-[10px] tracking-widest text-jb-ink/35 mb-3">BACK ONLINE</div>
              <p className="text-[13px] text-jb-ink/60 leading-relaxed">Everything pending syncs automatically, and the app shows exactly what&apos;s still catching up.</p>
            </BentoCard>
          </div>
        </div>
      </section>

      {/* ── CUSTOMER BILLS & REMINDERS ───────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto grid md:grid-cols-2 gap-14 items-center">
          <div className="order-2 md:order-1 flex justify-center">
            <PhoneFrame>
              <BrandedBillPreview />
            </PhoneFrame>
          </div>
          <div className="order-1 md:order-2">
            <SectionIntro id="bills" icon="bills" tag="TABS & DEBTS" title={"No more forgotten\ncustomer debts."} />
            <ul className="space-y-3">
              {[
                "Create customer tabs and open bills",
                "Track outstanding balances as they change",
                "Record full or partial payments",
                "Share bills through WhatsApp, email or a link",
                "Send friendly payment reminders",
                "Customize bills with your bar's branding and payment details",
              ].map((x) => (
                <li key={x} className="flex items-start gap-2.5 text-sm text-jb-ink/60">
                  <span className="mt-1.5 w-1 h-1 rounded-full bg-jb-ink/30 shrink-0" /> {x}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </section>

      {/* ── STOCK & ACCOUNTABILITY ───────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto grid md:grid-cols-2 gap-14 items-center">
          <div>
            <SectionIntro icon="stock" tag="STOCK" title={"Know what came in, what\nwent out, and what's left."} />
            <ul className="space-y-3">
              {[
                "Receive stock and record purchase cost",
                "Track how sales move stock automatically",
                "Count physical stock when you need to",
                "Record adjustments with a clear reason",
                "See discrepancies instead of guessing",
                "Review full stock history anytime",
              ].map((x) => (
                <li key={x} className="flex items-start gap-2.5 text-sm text-jb-ink/60">
                  <span className="mt-1.5 w-1 h-1 rounded-full bg-jb-ink/30 shrink-0" /> {x}
                </li>
              ))}
            </ul>
            <p className="mt-5 text-[13px] text-jb-ink/40 max-w-md">
              Different purchase prices over time are preserved, so old records never quietly change.
            </p>
          </div>
          <PhoneFrame>
            <StockReceivePreview />
          </PhoneFrame>
        </div>
      </section>

      {/* ── WHO IS IT FOR ─────────────────────────────────────────────────── */}
      <section className="py-16 border-t border-jb-ink/[0.06] overflow-hidden select-none">
        <div className="max-w-6xl mx-auto px-6 md:px-12 lg:px-20 mb-8">
          <Tag>WHO IT'S FOR</Tag>
          <RevealText className="mt-5 text-3xl md:text-4xl font-light tracking-tight leading-[1.1] text-jb-ink">
            {"A small bar run by one person.\nA lounge run by a team."}
          </RevealText>
        </div>
        <div className="flex border-y border-jb-ink/[0.06]" style={{ animation: "marqueeLeft 26s linear infinite" }}>
          {[...Array(3)].map((_, rep) => (
            <div key={rep} className="flex shrink-0">
              {AUDIENCE.map((a) => (
                <div key={a} className="flex items-center gap-3 px-8 py-5 border-r border-jb-ink/[0.06] shrink-0">
                  <span className="w-1.5 h-1.5 rounded-full bg-jb-ink/20 shrink-0" />
                  <span className="text-sm text-jb-ink/50 whitespace-nowrap tracking-wide">{a}</span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </section>

      {/* ── HOW IT WORKS ──────────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-3xl mx-auto">
          <div id="how-it-works" className="mb-14 scroll-mt-28">
            <PixelIcon type="notebook" size={40} />
            <div className="mt-4"><Tag>HOW IT WORKS</Tag></div>
            <RevealText className="mt-5 text-3xl md:text-4xl lg:text-5xl font-light tracking-tight leading-[1.1] text-jb-ink">
              {"Four simple steps."}
            </RevealText>
          </div>
          <div className="relative">
            <div className="absolute left-[19px] top-2 bottom-2 w-px bg-jb-ink/10" aria-hidden="true" />
            <div className="space-y-8">
              {STEPS.map((step) => (
                <div key={step.n} className="relative flex gap-5">
                  <div className="w-10 h-10 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center font-pixel text-[11px] shrink-0 z-10">
                    {step.n}
                  </div>
                  <div className="pt-1.5">
                    <h3 className="text-base font-medium text-jb-ink mb-1">{step.title}</h3>
                    <p className="text-sm text-jb-ink/50 leading-relaxed max-w-sm">{step.desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <a
            href="/onboarding"
            className="mt-12 inline-block px-8 py-3.5 rounded-xl bg-jb-ink text-jb-cream text-sm font-medium hover:bg-jb-green transition-colors"
          >
            Get Started
          </a>
        </div>
      </section>

      {/* ── FINAL CTA ─────────────────────────────────────────────────────── */}
      <section className="py-24 md:py-32 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06] bg-jb-green-dark text-jb-cream">
        <div className="max-w-2xl mx-auto text-center">
          <h2 className="text-3xl md:text-5xl font-light tracking-tight leading-[1.08] mb-6" style={{ fontFamily: "var(--font-display)" }}>
            Run your bar the simple way.
          </h2>
          <p className="text-sm md:text-base text-jb-cream/60 leading-relaxed mb-10 max-w-md mx-auto">
            Set it up once. Run it in a few taps. justbarme is ready whenever your bar is.
          </p>
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            <a href="/onboarding" className="px-8 py-3.5 rounded-xl bg-jb-cream text-jb-ink text-sm font-medium hover:bg-white transition-colors">
              Create Your Business
            </a>
            <a href="#features" className="px-8 py-3.5 rounded-xl border border-jb-cream/25 text-jb-cream text-sm font-medium hover:bg-jb-cream/10 transition-colors">
              Explore Features
            </a>
          </div>
          <a href="/install" className="mt-8 inline-block text-xs text-jb-cream/40 hover:text-jb-cream/70 tracking-wide transition-colors">
            Already set up? Install justbarme on your phone →
          </a>
        </div>
      </section>

      {/* ── FOOTER ────────────────────────────────────────────────────────── */}
      <footer className="py-10 px-6 md:px-12 lg:px-20 border-t border-jb-ink/[0.06]">
        <div className="max-w-6xl mx-auto flex flex-col md:flex-row items-start md:items-center justify-between gap-8">
          <div className="flex items-center gap-2">
            <Logo className="w-5 h-5 text-jb-ink/50" />
            <span className="font-pixel text-xs tracking-[0.25em] text-jb-ink/50">JUSTBARME</span>
          </div>
          <div className="flex flex-wrap items-center gap-x-8 gap-y-3">
            {[
              { label: "Features", href: "#features" },
              { label: "How it works", href: "#how-it-works" },
              { label: "Offline", href: "#offline" },
              { label: "Bills", href: "#bills" },
              { label: "Install", href: "/install" },
            ].map((l) => (
              <a key={l.label} href={l.href} className="text-xs text-jb-ink/40 hover:text-jb-ink/80 transition-colors tracking-widest">
                {l.label}
              </a>
            ))}
          </div>
          <div className="flex items-center gap-6">
            {["Privacy", "Terms", "Help"].map((l) => (
              <a key={l} href="#" className="text-xs text-jb-ink/25 hover:text-jb-ink/55 transition-colors tracking-widest">
                {l}
              </a>
            ))}
          </div>
        </div>
        <div className="max-w-6xl mx-auto mt-8 pt-6 border-t border-jb-ink/[0.04]">
          <span className="text-xs text-jb-ink/25">© 2026 justbarme. Built for bars and lounges.</span>
        </div>
      </footer>
    </div>
  )
}
