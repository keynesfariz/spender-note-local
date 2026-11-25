import { Geist_Mono, Inter, Figtree, Manrope } from "next/font/google"
import type { Metadata } from "next"

import "./globals.css"
import { AppShell } from "@/components/app-shell"
import { Providers } from "@/components/providers"
import { cn } from "@/lib/utils"

const manropeHeading = Manrope({subsets:['latin'],variable:'--font-heading'});

const inter = Inter({subsets:['latin'],variable:'--font-sans'})
const fontMono = Geist_Mono({ subsets: ["latin"], variable: "--font-mono" })

export const metadata: Metadata = {
  title: "Spender — Monthly Spending",
  description:
    "Personal spending tracker powered by Next.js 16.5 Go routes and local GGUF LLM.",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn(
              "antialiased",
              fontMono.variable
            , "font-sans", inter.variable, manropeHeading.variable)}
    >
      <body>
        <Providers>
          <AppShell>{children}</AppShell>
        </Providers>
      </body>
    </html>
  )
}
