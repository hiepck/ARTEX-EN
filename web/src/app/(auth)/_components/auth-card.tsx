import type { ReactNode } from "react";

import { APP_CONFIG } from "@/config/app-config";

/**
 * Neutral sign-in shell: a plain centered card on a muted background, the way a
 * typical internal admin portal looks. Deliberately avoids the large branded hero
 * panel so the page carries no visual signature of its own.
 */
export function AuthCard({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col bg-slate-100">
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 sm:px-8">
        <div className="flex items-center gap-2.5">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/logo.png" alt="" width={26} height={26} className="shrink-0" />
          <span className="font-medium text-slate-700 text-sm">{APP_CONFIG.name}</span>
        </div>
        <span className="hidden text-slate-400 text-xs sm:inline">Internal systems</span>
      </header>

      <main className="flex flex-1 items-center justify-center px-4 py-12">
        <div className="w-full max-w-sm rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
          <div className="mb-6 space-y-1">
            <h1 className="font-semibold text-lg text-slate-900 tracking-tight">{title}</h1>
            <p className="text-slate-500 text-sm">{subtitle}</p>
          </div>
          {children}
          <p className="mt-6 text-center text-slate-400 text-xs">
            Authorized access only. Activity on this system may be logged and monitored.
          </p>
        </div>
      </main>

      <footer className="shrink-0 px-6 py-4 text-center text-slate-400 text-xs">
        {APP_CONFIG.copyright} Internal use only.
      </footer>
    </div>
  );
}
