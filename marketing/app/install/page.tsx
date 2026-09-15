"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import { Logo } from "@/components/logo"

type Platform = "android" | "ios"

const ANDROID_STEPS = [
  { title: "Tap the browser menu", desc: "Look for the ⋮ icon in the top-right corner of Chrome." },
  { title: "Select “Install app”", desc: "It may also say “Add to Home screen.”" },
  { title: "Confirm installation", desc: "Tap Install when Chrome asks you to confirm." },
  { title: "Open justbarme", desc: "Find it on your home screen, just like any other app." },
]

const IOS_STEPS = [
  { title: "Tap the Share button", desc: "It's the square with an arrow, at the bottom of Safari." },
  { title: "Select “Add to Home Screen”", desc: "Scroll down the share sheet if you don't see it right away." },
  { title: "Tap Add", desc: "Confirm in the top-right corner." },
  { title: "Open justbarme", desc: "Find it on your home screen, just like any other app." },
]

function PlatformIcon({ platform }: { platform: Platform }) {
  return (
    <div className="w-10 h-10 rounded-xl border border-jb-ink/10 bg-white flex items-center justify-center">
      {platform === "android" ? (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#16201A" strokeWidth="1.6">
          <rect x="6" y="2" width="12" height="20" rx="2" />
          <circle cx="12" cy="18" r="0.8" fill="#16201A" />
          <path d="M9 2h6" />
        </svg>
      ) : (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#16201A" strokeWidth="1.6">
          <rect x="7" y="2" width="10" height="20" rx="2.5" />
          <circle cx="12" cy="18" r="0.8" fill="#16201A" />
        </svg>
      )}
    </div>
  )
}

export default function InstallGuidePage() {
  const [platform, setPlatform] = useState<Platform>("android")

  useEffect(() => {
    const ua = window.navigator.userAgent || ""
    if (/iPhone|iPad|iPod/i.test(ua)) setPlatform("ios")
  }, [])

  const steps = platform === "android" ? ANDROID_STEPS : IOS_STEPS

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col">
      <div className="w-full max-w-sm mx-auto px-6 pt-8 pb-16 flex-1 flex flex-col">
        <div className="flex items-center gap-4 mb-10">
          <Link href="/dashboard" aria-label="Back" className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none">←</Link>
          <span className="font-pixel text-[10px] tracking-widest text-jb-ink/35">SETTINGS · INSTALL</span>
        </div>

        <div className="w-16 h-16 rounded-2xl bg-jb-green-dark flex items-center justify-center mb-5">
          <Logo className="w-9 h-9 text-jb-cream" />
        </div>

        <h1 className="text-2xl font-light tracking-tight mb-1.5" style={{ fontFamily: "var(--font-display)" }}>
          Install justbarme on your phone
        </h1>
        <p className="text-[13px] text-jb-ink/45 mb-8 leading-relaxed">
          Add justbarme to your home screen so you can open it like a normal app — no browser bar, no typing a web address. This is the icon you&apos;ll see.
        </p>

        <div className="flex gap-2 mb-6 p-1 rounded-xl bg-jb-ink/[0.05]">
          {(["android", "ios"] as Platform[]).map((p) => (
            <button
              key={p}
              onClick={() => setPlatform(p)}
              className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
                platform === p ? "bg-white text-jb-ink shadow-sm" : "text-jb-ink/45"
              }`}
            >
              {p === "android" ? "Android / Chrome" : "iPhone / Safari"}
            </button>
          ))}
        </div>

        <div className="space-y-3 flex-1">
          {steps.map((step, i) => (
            <div key={step.title} className="flex items-start gap-3.5 rounded-xl border border-jb-ink/10 bg-white p-4">
              <div className="w-7 h-7 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-[12px] font-medium shrink-0">
                {i + 1}
              </div>
              <div>
                <h3 className="text-[14px] font-medium text-jb-ink mb-0.5">{step.title}</h3>
                <p className="text-[12.5px] text-jb-ink/45 leading-relaxed">{step.desc}</p>
              </div>
            </div>
          ))}
        </div>

        <div className="flex items-center gap-2.5 mt-6 mb-2">
          <PlatformIcon platform={platform} />
          <p className="text-[11.5px] text-jb-ink/35 leading-relaxed">
            Three simple steps to keep justbarme on your phone — no app store required.
          </p>
        </div>

        <div className="mt-6 flex flex-col gap-2.5">
          <Link href="/dashboard" className="w-full text-center rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors">
            Done — Open justbarme
          </Link>
          <Link href="/dashboard" className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 text-center transition-colors">
            Maybe later
          </Link>
        </div>
        <p className="text-center text-[11px] text-jb-ink/30 mt-2">You can always find this guide in Settings.</p>
      </div>
    </div>
  )
}
