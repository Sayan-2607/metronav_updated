"use client";
import Link from "next/link";
import { QRCodeSVG } from "qrcode.react";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { profileLabel } from "@/lib/format";
import { useNetwork, useSession } from "@/lib/hooks";
import type { Ticket } from "@/lib/types";

export default function Tickets() {
  const user = useSession();
  const { net } = useNetwork();
  const [tickets, setTickets] = useState<Ticket[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!user) return;
    api<Ticket[]>("/api/v1/tickets").then(setTickets).catch((e) => setError(e.message));
  }, [user]);

  const names = Object.fromEntries((net?.stations ?? []).map((s) => [s.id, s.name]));
  if (!user) return <p className="rounded-lg bg-panel p-5"><Link className="underline" href="/login?next=/tickets">Sign in</Link> to see your tickets.</p>;

  return (
    <div>
      <h1 className="font-sign text-4xl font-bold">My tickets</h1>
      <p className="mt-1 text-sm text-mute">Show the code at the gate. Each code works once and expires 3 hours after booking. No payment is taken in this prototype.</p>
      {error && <p role="alert" className="mt-4 text-[#C8352E]">{error}</p>}
      {tickets?.length === 0 && <p className="mt-6 rounded-lg bg-panel p-5">No tickets yet. <Link href="/" className="underline">Plan a journey</Link> to book one.</p>}
      <ul className="mt-6 grid gap-4 sm:grid-cols-2">
        {tickets?.map((t) => {
          const expired = new Date(t.expires_at) < new Date();
          const usable = t.status === "ACTIVE" && !expired;
          return (
            <li key={t.id} className="flex gap-4 rounded-lg bg-panel p-4">
              <div className={usable ? "" : "opacity-30"}><QRCodeSVG value={t.token} size={120} level="M" /></div>
              <div className="min-w-0">
                <p className="font-sign text-xl font-bold leading-tight">{names[t.from_station] ?? t.from_station} to {names[t.to_station] ?? t.to_station}</p>
                <p className="text-sm text-mute">₹{t.fare} · {profileLabel[t.profile] ?? t.profile}{t.coach > 0 && ` · board coach ${t.coach}`}</p>
                <p className="mt-2 text-sm">{t.status === "USED" ? "Used" : expired ? "Expired" : `Valid until ${new Date(t.expires_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`}</p>
                <p className="mt-1 truncate text-xs text-mute">{t.id}</p>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
