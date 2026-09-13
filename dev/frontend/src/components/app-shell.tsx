import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="mx-auto min-h-screen max-w-[1600px] px-6 py-8">
      <header className="mb-8 flex items-center justify-between">
        <Link to="/knowledge" className="font-semibold">
          Knowledge Base
        </Link>
        <nav className="flex gap-4 text-sm">
          <Link to="/knowledge">知識</Link>
          <Link to="/mcp-tokens">MCPトークン</Link>
        </nav>
      </header>
      <main>{children}</main>
    </div>
  );
}
