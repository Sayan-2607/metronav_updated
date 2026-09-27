"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import CoachStrip from "@/components/CoachStrip";
import NetworkMap from "@/components/NetworkMap";
import RouteStrip from "@/components/RouteStrip";
import StationSelect from "@/components/StationSelect";
import { api } from "@/lib/api";
import { profileLabel, occWord } from "@/lib/format";
import { fmtTime, useNetwork, useSession } from "@/lib/hooks";
import type { RouteResult } from "@/lib/types";

type Search = { sim_time: string; routes: RouteResult[] };

export default function Planner() {
  const { net, error: netErr } = useNetwork();
  const user = useSession();
  const router = useRouter();
  const [from, setFrom] = useState("PARK_STREET");
  const [to, setTo] = useState("SALT_LAKE_SECTOR_V");
  const [res, setRes] = useState<Search | null>(null);
  const [picked, setPicked] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [booked, setBooked] = useState<string | null>(null);

  const names = useMemo(() => Object.fromEntries((net?.stations ?? []).map((s) => [s.id, s.name])), [net]);

  async function search(e?: React.FormEvent) {
    e?.preventDefault();
    if (!from || !to) return setError("Choose both stations.");
    if (from === to) return setError("Origin and destination are the same station.");
    setBusy(true); setError(null); setBooked(null);
    try {
      const r = await api<Search>(`/api/v1/routes/search?from=${from}&to=${to}`);
      setRes(r); setPicked(0);
    } catch (err) {
      setRes(null); setError((err as Error).message);
    } finally { setBusy(false); }
  }

  async function book(r: RouteResult) {
    if (!user) return router.push("/login?next=/");
    setBusy(true); setError(null);
    try {
      const t = await api<{ ticket: { id: string } }>("/api/v1/tickets", {
        method: "POST", body: JSON.stringify({ from, to, profile: r.matched_profiles[0] }),
      });
      setBooked(t.ticket.id);
    } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  }

  if (netErr) return <p className="rounded-md bg-panel p-4">{netErr}</p>;
  const route = res?.routes[picked];
  const routeStations = route ? route.legs.flatMap((l, i) => (i === 0 ? l.stops : l.stops.slice(1))) : [];

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,420px)_1fr]">
      <section>
        <h1 className="font-sign text-4xl font-bold leading-none">Where to?</h1>
        <p className="mt-2 max-w-sm text-mute">Routes are ranked by time, changes and how crowded the stations are right now.</p>
        <form onSubmit={search} className="mt-5 space-y-3 rounded-lg bg-panel p-4">
          {net && <>
            <StationSelect id="from" label="From" value={from} onChange={setFrom} stations={net.stations} />
            <button type="button" onClick={() => { setFrom(to); setTo(from); }} className="text-sm underline underline-offset-2">Swap stations</button>
            <StationSelect id="to" label="To" value={to} onChange={setTo} stations={net.stations} />
          </>}
          <button disabled={busy || !net} className="w-full rounded-md bg-ink py-3 font-sign text-lg font-semibold text-panel disabled:opacity-50">
            {busy ? "Finding routes…" : "Find routes"}
          </button>
          {error && <p role="alert" className="text-sm text-[#C8352E]">{error}</p>}
        </form>

        {res && (
          <div className="mt-4 space-y-2">
            <p className="text-sm text-mute">Network time {fmtTime(res.sim_time)}. {res.routes.length === 1 ? "One distinct route; every preference agrees." : `${res.routes.length} distinct routes.`}</p>
            {res.routes.map((r, i) => (
              <button key={i} onClick={() => setPicked(i)}
                className={`w-full rounded-lg border-2 bg-panel p-3 text-left ${i === picked ? "border-ink" : "border-transparent"}`}>
                <div className="flex items-baseline justify-between">
                  <span className="font-sign text-3xl font-bold">{Math.round(r.eta_prediction.p50_min)} min</span>
                  <span className="font-sign text-lg">₹{r.fare}</span>
                </div>
                <p className="text-sm text-mute">
                  {r.matched_profiles.map((p) => profileLabel[p]).join(", ")} · {r.transfers === 0 ? "No changes" : `${r.transfers} change${r.transfers > 1 ? "s" : ""}`} · {occWord(r.crowd_score)} stations
                </p>
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="min-w-0 space-y-4">
        {route ? (
          <div className="rounded-lg bg-panel p-5">
            <RouteStrip legs={route.legs} names={names} />
            <dl className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div><dt className="text-sm text-mute">Likely arrival</dt><dd className="font-sign text-2xl font-bold">{Math.round(route.eta_prediction.p10_min)}–{Math.round(route.eta_prediction.p90_min)} min</dd></div>
              <div><dt className="text-sm text-mute">Timetable</dt><dd className="font-sign text-2xl">{route.total_min} min</dd></div>
              <div><dt className="text-sm text-mute">Stations</dt><dd className="font-sign text-2xl">{route.stations_travelled}</dd></div>
              <div><dt className="text-sm text-mute">Station crowding</dt><dd className="font-sign text-2xl">{Math.round(route.crowd_score)}%</dd></div>
            </dl>
            <p className="mt-2 text-xs text-mute">
              Range is the 10th–90th percentile from {route.eta_prediction.source === "ml" ? "the ETA model" : "a fallback estimate (ML service unavailable)"}.
              Timetable includes {route.wait_min} min average wait{route.transfers > 0 && ` and ${route.transfer_walk_min} min walking between platforms`}.
            </p>

            {route.next_departure && (
              <div className="mt-6 border-t border-rule pt-5">
                <h2 className="font-sign text-xl font-bold">
                  Next {route.legs[0].line.toLowerCase()} line train from {names[route.legs[0].from]} in {Math.max(0, Math.round(route.next_departure.eta_min))} min
                </h2>
                <p className="mb-3 text-sm text-mute">
                  Towards {names[route.next_departure.train.towards]}. Board coach {route.next_departure.coach_recommendation.recommended_coach}: it balances space on board against the walk to the exit at {names[route.legs[0].to]}.
                </p>
                <CoachStrip coaches={route.next_departure.train.coaches}
                  recommended={route.next_departure.coach_recommendation.recommended_coach}
                  exitCoach={route.next_departure.coach_recommendation.destination_exit_coach} />
                <p className="mt-2 text-xs text-mute">Loads are the train's current simulated occupancy, not a forecast for when it reaches you.</p>
              </div>
            )}

            <div className="mt-6 flex flex-wrap items-center gap-3">
              <button onClick={() => book(route)} disabled={busy} className="rounded-md bg-ink px-5 py-2.5 font-sign text-lg font-semibold text-panel disabled:opacity-50">
                {user ? `Book ticket · ₹${route.fare}` : "Sign in to book"}
              </button>
              {booked && <p className="text-sm">Ticket {booked} booked. <Link className="underline" href="/tickets">Show QR code</Link></p>}
            </div>
          </div>
        ) : (
          <div className="rounded-lg bg-panel p-5 text-mute">Choose two stations to see routes, predicted arrival and where to board.</div>
        )}
        {net && <div className="rounded-lg bg-panel p-2"><NetworkMap net={net} highlight={routeStations} /></div>}
      </section>
    </div>
  );
}
