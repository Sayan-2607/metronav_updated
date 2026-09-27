import type { Leg } from "@/lib/types";

const lineName = (id: string) => id.charAt(0) + id.slice(1).toLowerCase() + " line";

/** In-car style route strip: one coloured bar per leg with a tick per stop, then the legs in words. */
export default function RouteStrip({ legs, names }: { legs: Leg[]; names: Record<string, string> }) {
  const total = legs.reduce((a, l) => a + Math.max(l.stops.length - 1, 1), 0);
  return (
    <div>
      <div className="flex w-full items-center gap-1">
        {legs.map((l, i) => (
          <div key={i} className="relative" style={{ width: `${(Math.max(l.stops.length - 1, 1) / total) * 100}%` }}>
            <div className="h-[6px] rounded-full" style={{ background: l.color }} />
            <div className="absolute inset-x-0 top-[-3px] flex justify-between">
              {l.stops.map((s, k) => {
                const end = k === 0 || k === l.stops.length - 1;
                return <span key={s} title={names[s] ?? s}
                  className={`block rounded-full border-2 border-panel ${end ? "h-3 w-3 bg-ink" : "mt-[2px] h-2 w-2"}`}
                  style={end ? undefined : { background: l.color }} />;
              })}
            </div>
          </div>
        ))}
      </div>
      <ol className="mt-4 space-y-1.5">
        {legs.map((l, i) => (
          <li key={i} className="flex items-baseline gap-2">
            <span className="h-3 w-3 shrink-0 translate-y-[1px] rounded-sm" style={{ background: l.color }} aria-hidden />
            <span>
              <span className="font-sign text-lg font-semibold">{names[l.from]} to {names[l.to]}</span>
              <span className="text-sm text-mute"> · {lineName(l.line)} · {l.stops.length - 1} stop{l.stops.length === 2 ? "" : "s"} · {l.minutes} min</span>
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}
