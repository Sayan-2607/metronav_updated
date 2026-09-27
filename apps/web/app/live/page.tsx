"use client";
import { useEffect, useState } from "react";
import CoachStrip from "@/components/CoachStrip";
import NetworkMap from "@/components/NetworkMap";
import { api } from "@/lib/api";
import { occColor, occWord } from "@/lib/format";
import { fmtTime, useLive, useNetwork } from "@/lib/hooks";
import type { Station, Train } from "@/lib/types";

type Departures = { station: Station; departures: { line: string; towards: string; arrivals: { train: Train; eta_min: number }[] }[] };
type Forecast = { current: number; forecast_15m: number; forecast_30m: number; forecast_60m: number; source: string; model: string };

export default function Live() {
  const { net, error } = useNetwork();
  const live = useLive();
  const [sel, setSel] = useState<string | null>("ESPLANADE");
  const [dep, setDep] = useState<Departures | null>(null);
  const [fc, setFc] = useState<Forecast | null>(null);

  useEffect(() => {
    if (!sel) return;
    let stop = false;
    const load = () => {
      api<Departures>(`/api/v1/stations/${sel}`).then((d) => !stop && setDep(d)).catch(() => {});
      api<Forecast>(`/api/v1/predictions/crowd?station=${sel}`).then((f) => !stop && setFc(f)).catch(() => setFc(null));
    };
    load();
    const t = setInterval(load, 10000);
    return () => { stop = true; clearInterval(t); };
  }, [sel]);

  if (error) return <p className="rounded-md bg-panel p-4">{error}</p>;
  const names = Object.fromEntries((net?.stations ?? []).map((s) => [s.id, s.name]));
  const st = live.stations.find((s) => s.id === sel);

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
      <section className="min-w-0">
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h1 className="font-sign text-4xl font-bold leading-none">Live network</h1>
          <p className="text-sm text-mute">
            {live.status === "live" ? `Updating live · network time ${fmtTime(live.simTime)}` : live.status === "connecting" ? "Connecting…" : "Connection lost, retrying…"}
            {" · "}{live.trains.length} trains
          </p>
        </div>
        <div className="rounded-lg bg-panel p-2">
          {net && <NetworkMap net={net} stations={live.stations} trains={live.trains} selected={sel} onSelect={setSel} />}
        </div>
        <div className="mt-3 flex flex-wrap gap-4 text-sm text-mute">
          {[20, 50, 80, 95].map((v) => (
            <span key={v} className="flex items-center gap-1.5"><span className="h-3 w-3 rounded-full" style={{ background: occColor(v) }} />{occWord(v)}</span>
          ))}
          <span className="flex items-center gap-1.5"><span className="h-3 w-3 rounded-full bg-ink" />Closed</span>
          <span>Trains held by a delay have a red outline.</span>
        </div>
      </section>

      <aside className="space-y-4">
        {st ? (
          <div className="rounded-lg bg-panel p-4">
            <h2 className="font-sign text-3xl font-bold leading-tight">{st.name}</h2>
            <p className="text-sm text-mute">{st.lines.map((l) => l.charAt(0) + l.slice(1).toLowerCase()).join(" and ")} line · {st.source === "cv" ? "camera count" : "simulated"}</p>
            {st.closed ? <p className="mt-3 font-semibold">Station closed.</p> : (
              <>
                <div className="mt-4 flex items-end gap-3">
                  <span className="font-sign text-5xl font-bold" style={{ color: occColor(st.density) }}>{Math.round(st.density)}%</span>
                  <span className="pb-2 text-sm text-mute">of platform capacity<br />≈ {st.people.toLocaleString()} people · usual {Math.round(st.expected)}%</span>
                </div>
                {fc && (
                  <table className="mt-4 w-full text-left text-sm">
                    <caption className="mb-1 text-left text-mute">Forecast ({fc.source === "ml" ? "crowd model" : "fallback estimate"})</caption>
                    <tbody className="font-sign text-lg">
                      <tr>{[["+15 min", fc.forecast_15m], ["+30 min", fc.forecast_30m], ["+60 min", fc.forecast_60m]].map(([k, v]) => (
                        <td key={k as string}><span className="block text-xs text-mute">{k}</span>{Math.round(v as number)}%</td>))}</tr>
                    </tbody>
                  </table>
                )}
              </>
            )}
          </div>
        ) : <p className="rounded-lg bg-panel p-4 text-mute">Select a station on the map.</p>}

        {dep?.departures.map((d) => (
          <div key={d.line + d.towards} className="rounded-lg bg-panel p-4">
            <h3 className="font-sign text-lg font-bold">Towards {names[d.towards]}</h3>
            <ul className="mt-2 space-y-3">
              {d.arrivals.map((a) => (
                <li key={a.train.id}>
                  <div className="mb-1 flex justify-between text-sm">
                    <span>{a.train.id}{a.train.delayed && " · held"}</span>
                    <span className="font-sign text-base font-semibold">{a.eta_min < 0.5 ? "Now" : `${Math.round(a.eta_min)} min`}</span>
                  </div>
                  <CoachStrip coaches={a.train.coaches} compact />
                </li>
              ))}
            </ul>
          </div>
        ))}
      </aside>
    </div>
  );
}
