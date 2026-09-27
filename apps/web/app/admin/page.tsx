"use client";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import NetworkMap from "@/components/NetworkMap";
import { api, canOperate, isStaff } from "@/lib/api";
import { fmtTime, useLive, useNetwork, useSession } from "@/lib/hooks";
import type { Incident } from "@/lib/types";

type Overview = {
  active_trains: number; avg_train_occupancy: number; avg_station_density: number; crowded_stations: number;
  delayed_trains: number; open_incidents: number; tickets_today_utc: number; ws_connections: number;
  store: string; redis: boolean; ml_healthy: boolean;
};
type Metric = { mae: number; rmse: number; mape_pct: number; r2: number };
type Models = {
  version: string; trained_at: string; data: { kind: string; rows: number; stations: number; days: number };
  crowd_forecast: { by_horizon: Record<string, { model: Metric; baseline_persistence: Metric; baseline_profile: Metric }> };
  anomaly_detection: { threshold_z: number; eval: { precision: number; recall: number; f1: number; false_positive_rate: number } };
  eta: { p50: { mae_min: number; median_ae_min: number; p95_ae_min: number }; baseline_schedule: { mae_min: number }; interval_p10_p90: { coverage: number; mean_width_min: number } };
  disclaimer: string;
};

const sevStyle: Record<string, string> = { CRITICAL: "bg-[#C8352E] text-panel", HIGH: "bg-[#E07B39] text-ink", MEDIUM: "bg-[#E3B448] text-ink", LOW: "bg-concrete text-ink" };

export default function Operations() {
  const user = useSession();
  const { net } = useNetwork();
  const live = useLive();
  const [ov, setOv] = useState<Overview | null>(null);
  const [inc, setInc] = useState<Incident[]>([]);
  const [models, setModels] = useState<Models | null>(null);
  const [modelErr, setModelErr] = useState<string | null>(null);
  const [token, setToken] = useState("");
  const [validation, setValidation] = useState<string | null>(null);

  const refresh = useCallback(() => {
    api<Overview>("/api/v1/admin/overview").then(setOv).catch(() => {});
    api<Incident[]>("/api/v1/incidents").then(setInc).catch(() => {});
  }, []);

  useEffect(() => {
    if (!isStaff(user?.role)) return;
    refresh();
    const t = setInterval(refresh, 5000);
    api<Models>("/api/v1/ml/models").then(setModels).catch((e) => setModelErr(e.message));
    return () => clearInterval(t);
  }, [user, refresh, live.incidents]);

  if (!user) return <p className="rounded-lg bg-panel p-5"><Link className="underline" href="/login?next=/admin">Sign in</Link> with a staff account.</p>;
  if (!isStaff(user.role)) return <p className="rounded-lg bg-panel p-5">Operations is available to operator, analyst and admin accounts.</p>;

  async function setStatus(id: string, status: string) {
    await api(`/api/v1/incidents/${id}`, { method: "PATCH", body: JSON.stringify({ status }) }).catch(() => {});
    refresh();
  }
  async function validate(e: React.FormEvent) {
    e.preventDefault();
    try {
      const r = await api<{ valid: boolean; reason?: string; ticket_id?: string }>("/api/v1/tickets/validate", { method: "POST", body: JSON.stringify({ token: token.trim() }) });
      setValidation(r.valid ? `Valid: ${r.ticket_id} admitted.` : `Rejected: ${r.reason}.`);
    } catch (err) { setValidation((err as Error).message); }
  }

  const deviation = [...live.stations].filter((s) => !s.closed)
    .sort((a, b) => (b.density - b.expected) - (a.density - a.expected)).slice(0, 10)
    .map((s) => ({ name: s.name, Observed: Math.round(s.density), Usual: Math.round(s.expected) }));

  const kpis: [string, string | number][] = ov ? [
    ["Trains running", ov.active_trains], ["Avg train load", `${ov.avg_train_occupancy}%`],
    ["Avg platform load", `${ov.avg_station_density}%`], ["Crowded stations", ov.crowded_stations],
    ["Trains held", ov.delayed_trains], ["Open incidents", ov.open_incidents],
    ["Tickets today (UTC)", ov.tickets_today_utc], ["Live viewers", ov.ws_connections],
  ] : [];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="font-sign text-4xl font-bold leading-none">Operations</h1>
        <p className="text-sm text-mute">
          Network time {fmtTime(live.simTime)}
          {ov && <> · storage {ov.store}{ov.store === "memory" && " (not persisted)"} · Redis {ov.redis ? "on" : "off"} · models {ov.ml_healthy ? "online" : "offline, using fallbacks"}</>}
        </p>
      </div>

      <dl className="grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-rule sm:grid-cols-4">
        {kpis.map(([k, v]) => (
          <div key={k} className="bg-panel p-4"><dt className="text-sm text-mute">{k}</dt><dd className="font-sign text-3xl font-bold">{v}</dd></div>
        ))}
      </dl>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="rounded-lg bg-panel p-2">{net && <NetworkMap net={net} stations={live.stations} trains={live.trains} />}</section>
        <section className="rounded-lg bg-panel p-4">
          <h2 className="font-sign text-xl font-bold">Stations furthest above their usual load</h2>
          <p className="mb-2 text-sm text-mute">Observed now against the demand-model baseline for this time and day.</p>
          <div className="h-[340px]">
            <ResponsiveContainer>
              <BarChart data={deviation} layout="vertical" margin={{ left: 20, right: 10 }}>
                <CartesianGrid strokeDasharray="3 3" horizontal={false} />
                <XAxis type="number" domain={[0, 100]} unit="%" />
                <YAxis type="category" dataKey="name" width={130} interval={0} tick={{ fontSize: 12 }} />
                <Tooltip />
                <Legend />
                <Bar dataKey="Usual" fill="#C9D0D7" />
                <Bar dataKey="Observed" fill="#16202B" />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </section>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        <section className="rounded-lg bg-panel p-4">
          <h2 className="font-sign text-xl font-bold">Incidents</h2>
          {inc.length === 0 && <p className="mt-2 text-mute">No incidents. Crowd anomalies appear here automatically; try a crowd surge in Simulation.</p>}
          <ul className="mt-2 divide-y divide-rule">
            {inc.map((i) => (
              <li key={i.id} className="py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className={`rounded px-2 py-0.5 text-xs font-semibold ${sevStyle[i.severity]}`}>{i.severity}</span>
                  <span className="font-semibold">{i.title}</span>
                  <span className="text-xs text-mute">{new Date(i.created_at).toLocaleTimeString()} · {i.status.toLowerCase()}</span>
                </div>
                <p className="mt-1 text-sm text-mute">{i.detail}</p>
                {canOperate(user.role) && i.status !== "RESOLVED" && (
                  <div className="mt-2 flex gap-3 text-sm">
                    {i.status === "OPEN" && <button className="underline" onClick={() => setStatus(i.id, "ACKNOWLEDGED")}>Acknowledge</button>}
                    <button className="underline" onClick={() => setStatus(i.id, "RESOLVED")}>Resolve</button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </section>
        {canOperate(user.role) && (
          <section className="h-fit rounded-lg bg-panel p-4">
            <h2 className="font-sign text-xl font-bold">Check a ticket</h2>
            <form onSubmit={validate} className="mt-2 space-y-2">
              <label className="block text-sm text-mute" htmlFor="tok">Scanned code</label>
              <input id="tok" value={token} onChange={(e) => setToken(e.target.value)} placeholder="MN1.TKT-…" className="w-full rounded-md border border-rule px-3 py-2 font-mono text-sm" />
              <button className="rounded-md bg-ink px-4 py-2 text-panel">Check ticket</button>
            </form>
            {validation && <p className="mt-2 text-sm" role="status">{validation}</p>}
          </section>
        )}
      </div>

      <section className="rounded-lg bg-panel p-4">
        <h2 className="font-sign text-xl font-bold">Model performance</h2>
        {modelErr && <p className="mt-2 text-mute">{modelErr}</p>}
        {models && (
          <>
            <p className="mt-1 text-sm text-mute">{models.version} · trained {new Date(models.trained_at).toLocaleString()} on {models.data.rows.toLocaleString()} {models.data.kind} rows ({models.data.stations} stations × {models.data.days} days). {models.disclaimer}</p>
            <div className="mt-4 overflow-x-auto">
              <table className="w-full min-w-[520px] text-left text-sm">
                <thead className="text-mute"><tr><th className="py-1 font-normal">Crowd forecast MAE (points)</th><th className="font-normal">Model</th><th className="font-normal">Persistence</th><th className="font-normal">Usual-pattern</th><th className="font-normal">Model R²</th></tr></thead>
                <tbody className="font-sign text-base">
                  {Object.entries(models.crowd_forecast.by_horizon).map(([h, v]) => (
                    <tr key={h} className="border-t border-rule"><td className="py-1.5">{h} ahead</td><td className="font-bold">{v.model.mae}</td><td>{v.baseline_persistence.mae}</td><td>{v.baseline_profile.mae}</td><td>{v.model.r2}</td></tr>
                  ))}
                </tbody>
              </table>
            </div>
            <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2">
              <div><dt className="text-mute">Anomaly detection (z ≥ {models.anomaly_detection.threshold_z}, injected spikes)</dt>
                <dd className="font-sign text-lg">Precision {models.anomaly_detection.eval.precision} · Recall {models.anomaly_detection.eval.recall} · F1 {models.anomaly_detection.eval.f1} · FPR {models.anomaly_detection.eval.false_positive_rate}</dd></div>
              <div><dt className="text-mute">Journey time, median prediction</dt>
                <dd className="font-sign text-lg">MAE {models.eta.p50.mae_min} min (timetable {models.eta.baseline_schedule.mae_min}) · p95 error {models.eta.p50.p95_ae_min} min · 10–90% range covers {Math.round(models.eta.interval_p10_p90.coverage * 100)}%</dd></div>
            </dl>
          </>
        )}
      </section>
    </div>
  );
}
