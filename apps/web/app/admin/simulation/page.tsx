"use client";
import Link from "next/link";
import { useState } from "react";
import { CartesianGrid, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, canOperate, isStaff } from "@/lib/api";
import { useLive, useNetwork, useSession } from "@/lib/hooks";

type WhatIf = {
  assumptions: string[]; arrival_rate_per_min: number; boarding_capacity_per_train: number; peak_queue: number;
  peak_platform_pct: number; spillover_minute: number; recovery_minute: number; max_left_behind: number;
  series: { minute: number; platform_pct: number; queue: number; train_arrived: boolean }[];
};

const field = "w-full rounded-md border border-rule bg-panel px-3 py-2";

export default function Simulation() {
  const user = useSession();
  const { net } = useNetwork();
  const live = useLive();
  const [type, setType] = useState("crowd_surge");
  const [station, setStation] = useState("ESPLANADE");
  const [line, setLine] = useState("BLUE");
  const [delay, setDelay] = useState(8);
  const [surge, setSurge] = useState(30);
  const [duration, setDuration] = useState(20);
  const [msg, setMsg] = useState<string | null>(null);
  const [wi, setWi] = useState<WhatIf | null>(null);

  if (!user) return <p className="rounded-lg bg-panel p-5"><Link className="underline" href="/login?next=/admin/simulation">Sign in</Link> with a staff account.</p>;
  if (!isStaff(user.role)) return <p className="rounded-lg bg-panel p-5">Simulation is available to operator, analyst and admin accounts.</p>;

  const names = Object.fromEntries((net?.stations ?? []).map((s) => [s.id, s.name]));
  const stations = [...(net?.stations ?? [])].sort((a, b) => a.name.localeCompare(b.name));
  const linesAtStation = net?.lines.filter((l) => l.stations.includes(station)) ?? [];

  async function apply(e: React.FormEvent) {
    e.preventDefault();
    const body: Record<string, unknown> = { type, duration_min: duration };
    if (type === "train_delay") Object.assign(body, { line_id: line, delay_min: delay });
    else Object.assign(body, { station_id: station, ...(type === "crowd_surge" ? { surge_pct: surge } : {}) });
    try {
      const sc = await api<{ id: string }>("/api/v1/simulation/scenarios", { method: "POST", body: JSON.stringify(body) });
      setMsg(`Scenario ${sc.id} is running on the live network.`);
    } catch (err) { setMsg((err as Error).message); }
  }

  async function clear(id: string) {
    await api(`/api/v1/simulation/scenarios/${id}`, { method: "DELETE" }).catch(() => {});
  }

  async function project() {
    const l = linesAtStation.find((x) => x.id === line) ? line : linesAtStation[0]?.id;
    try {
      setWi(await api<WhatIf>("/api/v1/simulation/whatif", {
        method: "POST", body: JSON.stringify({ station_id: station, line_id: l, delay_min: delay, surge_pct: surge, horizon_min: 45 }),
      }));
    } catch (err) { setMsg((err as Error).message); }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-sign text-4xl font-bold leading-none">Simulation</h1>
        <p className="mt-2 max-w-2xl text-mute">Project what a delay or surge would do to one platform, or inject a scenario into the live simulated network to see how routing, crowd maps and incident detection respond.</p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[360px_1fr]">
        <section className="space-y-3 rounded-lg bg-panel p-4">
          <label className="block"><span className="mb-1 block text-sm text-mute">Station</span>
            <select className={field} value={station} onChange={(e) => setStation(e.target.value)}>
              {stations.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <label className="block"><span className="mb-1 block text-sm text-mute">Line</span>
            <select className={field} value={line} onChange={(e) => setLine(e.target.value)}>
              {net?.lines.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}</select></label>
          <div className="grid grid-cols-3 gap-2">
            <label><span className="mb-1 block text-sm text-mute">Delay (min)</span><input type="number" min={0} max={120} className={field} value={delay} onChange={(e) => setDelay(+e.target.value)} /></label>
            <label><span className="mb-1 block text-sm text-mute">Surge (%)</span><input type="number" min={0} max={300} className={field} value={surge} onChange={(e) => setSurge(+e.target.value)} /></label>
            <label><span className="mb-1 block text-sm text-mute">Scenario lasts</span><input type="number" min={1} max={180} className={field} value={duration} onChange={(e) => setDuration(+e.target.value)} /></label>
          </div>
          <button onClick={project} className="w-full rounded-md bg-ink py-2.5 font-sign text-lg font-semibold text-panel">Project platform queue</button>

          {canOperate(user.role) && (
            <form onSubmit={apply} className="space-y-2 border-t border-rule pt-3">
              <label className="block"><span className="mb-1 block text-sm text-mute">Inject into live network</span>
                <select className={field} value={type} onChange={(e) => setType(e.target.value)}>
                  <option value="crowd_surge">Crowd surge at station</option>
                  <option value="train_delay">Hold all trains on line (delay)</option>
                  <option value="station_closure">Close station</option>
                </select></label>
              <button className="w-full rounded-md border-2 border-ink py-2 font-sign text-lg font-semibold">Start scenario</button>
            </form>
          )}
          {msg && <p role="status" className="text-sm">{msg}</p>}
        </section>

        <section className="min-w-0 space-y-4">
          <div className="rounded-lg bg-panel p-4">
            <h2 className="font-sign text-xl font-bold">Running scenarios</h2>
            {live.scenarios.length === 0 ? <p className="mt-1 text-mute">None. The network is running on its normal demand pattern.</p> : (
              <ul className="mt-2 divide-y divide-rule">
                {live.scenarios.map((s) => (
                  <li key={s.id} className="flex items-center justify-between py-2 text-sm">
                    <span>{s.id}: {s.type.replace("_", " ")} {s.station_id ? `at ${names[s.station_id]}` : `on ${s.line_id?.toLowerCase()} line`}
                      {s.surge_pct ? ` +${s.surge_pct}%` : ""}{s.delay_min ? ` ${s.delay_min} min` : ""}</span>
                    {canOperate(user.role) && <button onClick={() => clear(s.id)} className="underline">End</button>}
                  </li>
                ))}
              </ul>
            )}
          </div>

          {wi && (
            <div className="rounded-lg bg-panel p-4">
              <h2 className="font-sign text-xl font-bold">{names[station]}: platform over the next 45 minutes</h2>
              <dl className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
                <div><dt className="text-sm text-mute">Peak platform load</dt><dd className="font-sign text-2xl font-bold">{wi.peak_platform_pct}%</dd></div>
                <div><dt className="text-sm text-mute">Over capacity from</dt><dd className="font-sign text-2xl font-bold">{wi.spillover_minute < 0 ? "Never" : `min ${wi.spillover_minute}`}</dd></div>
                <div><dt className="text-sm text-mute">Most left behind</dt><dd className="font-sign text-2xl font-bold">{Math.round(wi.max_left_behind)}</dd></div>
                <div><dt className="text-sm text-mute">Cleared by</dt><dd className="font-sign text-2xl font-bold">{wi.recovery_minute < 0 ? "Not in window" : `min ${wi.recovery_minute}`}</dd></div>
              </dl>
              <div className="mt-4 h-[260px]">
                <ResponsiveContainer>
                  <LineChart data={wi.series} margin={{ right: 10 }}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="minute" unit=" min" />
                    <YAxis unit="%" domain={[0, (max: number) => Math.max(110, Math.ceil(max / 10) * 10)]} />
                    <Tooltip />
                    <ReferenceLine y={100} stroke="#C8352E" strokeDasharray="4 4" label={{ value: "Capacity", fill: "#C8352E", fontSize: 12 }} />
                    <Line type="stepAfter" dataKey="platform_pct" name="Platform load" stroke="#16202B" dot={false} strokeWidth={2} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
              <p className="mt-2 text-sm text-mute">Arrivals {wi.arrival_rate_per_min}/min; each train can take about {Math.round(wi.boarding_capacity_per_train)} people.</p>
              <details className="mt-2 text-sm"><summary className="cursor-pointer">Model assumptions</summary>
                <ul className="mt-1 list-disc pl-5 text-mute">{wi.assumptions.map((a) => <li key={a}>{a}</li>)}</ul></details>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
