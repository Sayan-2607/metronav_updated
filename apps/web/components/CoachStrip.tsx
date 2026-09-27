import { occColor } from "@/lib/format";

/** A train drawn as its coaches, each filled to its occupancy. */
export default function CoachStrip({ coaches, recommended, exitCoach, compact = false }: {
  coaches: number[]; recommended?: number; exitCoach?: number; compact?: boolean;
}) {
  const h = compact ? 28 : 46;
  return (
    <div className="w-full">
      <div className="flex items-end gap-[3px]" aria-label="Coach occupancy, front of train on the left">
        {coaches.map((v, i) => {
          const n = i + 1;
          const rec = n === recommended;
          return (
            <div key={i} className="flex-1 min-w-0">
              <div className={`relative overflow-hidden bg-panel border ${rec ? "border-ink border-2" : "border-rule"} ${i === 0 ? "rounded-l-[10px]" : ""} ${i === coaches.length - 1 ? "rounded-r-[10px]" : ""} rounded-[3px]`}
                style={{ height: h }} title={`Coach ${n}: ${Math.round(v)}% full`}>
                <div className="absolute bottom-0 left-0 right-0" style={{ height: `${Math.min(100, v)}%`, background: occColor(v) }} />
                {!compact && <span className="absolute inset-x-0 top-1 text-center font-sign text-[13px] font-semibold text-ink">{Math.round(v)}</span>}
              </div>
              {!compact && (
                <div className="mt-1 text-center font-sign text-[12px] leading-tight text-mute">
                  C{n}{rec && <span className="block font-bold text-ink">Board</span>}
                  {exitCoach === n && !rec && <span className="block">Exit</span>}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
