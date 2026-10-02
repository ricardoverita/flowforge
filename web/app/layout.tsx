import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "FlowForge", template: "%s · FlowForge" },
  description: "Workflow definitions, durable executions, and execution history.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body>
    <header className="app-header">
      <Link href="/" className="brand"><span className="brand-mark" aria-hidden="true">F</span>FlowForge</Link>
      <nav aria-label="Main navigation">
        <Link href="/">Dashboard</Link><Link href="/workflows">Workflows</Link><Link href="/executions">Executions</Link>
      </nav>
      <span className="environment">Development console</span>
    </header>
    <main>{children}</main>
    <footer>FlowForge · Sequential workflow orchestration · Times shown in UTC</footer>
  </body></html>;
}
