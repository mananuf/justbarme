import React from "react"
import type { Metadata, Viewport } from 'next'
import { Geist, Geist_Mono, IBM_Plex_Sans } from 'next/font/google'
import { Courier_Prime } from 'next/font/google'
import { Analytics } from '@vercel/analytics/next'
import './globals.css'

const _geist = Geist({ subsets: ["latin"] });
const _geistMono = Geist_Mono({ subsets: ["latin"] });
const _courierPrime = Courier_Prime({ weight: ["400", "700"], subsets: ["latin"] });
const _ibmPlexSans = IBM_Plex_Sans({ weight: ["300", "400", "500", "600"], subsets: ["latin"] });

export const metadata: Metadata = {
  title: 'justbarme — Run your bar. Simply.',
  description:
    'justbarme is a simple, mobile-first app for bars and lounges. Track sales, stock, tables and tabs, outstanding bills, expenses and staff activity from your phone — online or offline.',
  keywords: ['bar management app', 'bar POS Nigeria', 'lounge management', 'bar stock tracking', 'bar sales app', 'offline POS'],
  authors: [{ name: 'justbarme' }],
  openGraph: {
    title: 'justbarme — Run your bar. Simply.',
    description: 'Keep track of sales, stock, customer bills, expenses and staff activity from one simple app built for bars and lounges.',
    type: 'website',
    url: 'https://justbarme.app',
    siteName: 'justbarme',
  },
  twitter: {
    card: 'summary_large_image',
    title: 'justbarme — Run your bar. Simply.',
    description: 'Keep track of sales, stock, customer bills, expenses and staff activity from one simple app built for bars and lounges.',
  },
  icons: {
    icon: [
      { url: '/logo.svg', type: 'image/svg+xml' },
      { url: '/favicon-32.png', sizes: '32x32', type: 'image/png' },
    ],
    apple: '/apple-icon.png',
  },
}

export const viewport: Viewport = {
  themeColor: '#121A14',
  width: 'device-width',
  initialScale: 1,
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en">
      <body className={`font-sans antialiased`}>
        {children}
        <Analytics />
      </body>
    </html>
  )
}
