"use client"

import { useEffect, useState } from "react"
import { Logo } from "@/components/logo"

const LOGO_IN_DUR     = 700   // logo mark appearing
const HOLD_DURATION   = 350   // hold fully visible
const LOGO_IN_TOTAL   = LOGO_IN_DUR + HOLD_DURATION

const LOGO_OUT_DUR    = 500   // logo mark fading out

const CURTAIN_DELAY    = LOGO_IN_TOTAL + 60
const CURTAIN_DURATION = 1300 // matches the CSS transition on the curtain div
const ANIM_TOTAL       = CURTAIN_DELAY + CURTAIN_DURATION + 100

// Exported: moment the curtain finishes retracting — when the bg is fully visible
export const INTRO_DURATION_MS = CURTAIN_DELAY + CURTAIN_DURATION
// Exported: ms before curtain fully done to start hero animations (overlap for smoothness)
export const HERO_REVEAL_MS = CURTAIN_DELAY + CURTAIN_DURATION - 150

type Phase = "idle" | "in" | "out" | "done"

export function IntroAnimation({ onDone }: { onDone: () => void }) {
  const [phase, setPhase] = useState<Phase>("idle")
  const [curtainUp, setCurtainUp] = useState(false)

  useEffect(() => {
    const t0 = setTimeout(() => setPhase("in"), 80)
    const t1 = setTimeout(() => setPhase("out"), LOGO_IN_TOTAL)
    const t2 = setTimeout(() => setCurtainUp(true), CURTAIN_DELAY)
    const t3 = setTimeout(() => onDone(), HERO_REVEAL_MS)
    const t4 = setTimeout(() => setPhase("done"), ANIM_TOTAL)

    return () => { clearTimeout(t0); clearTimeout(t1); clearTimeout(t2); clearTimeout(t3); clearTimeout(t4) }
  }, [onDone])

  if (phase === "done") return null

  const isIdle = phase === "idle"
  const isIn = phase === "in"
  const isOut = phase === "out"

  const opacity = isIdle ? 0 : isIn ? 1 : isOut ? 0 : 1
  const blur = isIdle ? 20 : isIn ? 0 : isOut ? 10 : 0
  const scale = isIdle ? 0.82 : isIn ? 1 : isOut ? 1.08 : 1

  const transition = isOut
    ? `opacity ${LOGO_OUT_DUR}ms cubic-bezier(0.4,0,1,1), filter ${LOGO_OUT_DUR}ms cubic-bezier(0.4,0,1,1), transform ${LOGO_OUT_DUR}ms cubic-bezier(0.4,0,1,1)`
    : isIn
    ? `opacity ${LOGO_IN_DUR}ms cubic-bezier(0.16,1,0.3,1), filter ${LOGO_IN_DUR}ms cubic-bezier(0.16,1,0.3,1), transform ${LOGO_IN_DUR}ms cubic-bezier(0.16,1,0.3,1)`
    : "none"

  return (
    <div className="fixed inset-0 z-[100] pointer-events-none" aria-hidden="true">
      {/* Cream curtain — retracts upward, revealing the page from the bottom */}
      <div
        className="absolute inset-x-0 top-0 bg-jb-cream"
        style={{
          bottom: curtainUp ? "100%" : "0%",
          transition: curtainUp ? "bottom 1.3s cubic-bezier(0.76, 0, 0.24, 1)" : "none",
        }}
      />

      {/* justbarme mark */}
      <div className="absolute inset-0 flex items-center justify-center">
        <Logo
          className="text-jb-ink w-16 h-16 sm:w-20 sm:h-20"
          style={{
            opacity,
            filter: `blur(${blur}px)`,
            transform: `scale(${scale})`,
            transition,
            willChange: "opacity, filter, transform",
          }}
        />
      </div>
    </div>
  )
}
