"use client";
import { occColor } from "@/lib/format";
import type { Network, Station, Train } from "@/lib/types";

type Props = {
  net: Network;
  stations?: Station[];
  trains?: Train[];
  selected?: string | null;
  highlight?: string[]; // station ids on a planned route
  onSelect?: (id: string) => void;
};

const VERTICAL = new Set(["BLUE"]);

/**
 * Schematic line diagram. Label placement follows the layout in data/network.json:
 * Blue runs vertically, Green and Yellow horizontally; labels are placed so they
 * never sit on another line.
 */
export default function NetworkMap({ net, stations = [], trains = [], selected, highlight = [], onSelect }: Props) {
  const live = new Map(stations.map((s) => [s.id, s]));
  const pos = new Map(net.stations.map((s) => [s.id, s]));
  const esplanadeY = pos.get("ESPLANADE")?.y ?? 0;
  const linesAt = new Map<string, string[]>();
  net.lines.forEach((l) => l.stations.forEach((s) => linesAt.set(s, [...(linesAt.get(s) ?? []), l.id])));
  const hl = new Set(highlight);

  function label(id: string, x: number, y: number, name: string) {
    const ls = linesAt.get(id) ?? [];
    const onlyBlue = ls.length === 1 && ls[0] === "BLUE";
    const bold = ls.length > 1 || hl.has(id);
    const common = { className: "font-sign", fontSize: 12.5, fontWeight: bold ? 700 : 500, fill: "#16202B" } as const;
    if (onlyBlue || id === "NOAPARA") {
      const right = y > esplanadeY;
      return <text {...common} x={right ? x + 13 : x - 13} y={y + 4} textAnchor={right ? "start" : "end"}>{name}</text>;
    }
    if (x < (pos.get("ESPLANADE")?.x ?? 0) && ls.includes("GREEN")) {
      return <text {...common} x={x - 6} y={y + 16} textAnchor="end" transform={`rotate(-40 ${x - 6} ${y + 16})`}>{name}</text>;
    }
    const dx = id === "ESPLANADE" ? 16 : 6; // clear the Blue line's train markers
    const dy = id === "ESPLANADE" ? -14 : -12;
    return <text {...common} x={x + dx} y={y + dy} transform={`rotate(-40 ${x + dx} ${y + dy})`}>{name}</text>;
  }

  return (
    <svg viewBox="200 -40 1000 830" className="w-full h-auto select-none" role="img" aria-label="Schematic metro network map">
      {net.lines.map((l) => (
        <polyline key={l.id} fill="none" stroke={l.color} strokeWidth={7} strokeLinecap="round" strokeLinejoin="round"
          points={l.stations.map((s) => `${pos.get(s)!.x},${pos.get(s)!.y}`).join(" ")} />
      ))}
      {highlight.length > 1 && (
        <polyline fill="none" stroke="#16202B" strokeOpacity={0.35} strokeWidth={16} strokeLinecap="round" strokeLinejoin="round"
          points={highlight.map((s) => `${pos.get(s)!.x},${pos.get(s)!.y}`).join(" ")} />
      )}
      {trains.map((t) => {
        const vert = VERTICAL.has(t.line);
        const off = t.direction === 1 ? 9 : -9;
        const x = vert ? t.x + off : t.x;
        const y = vert ? t.y : t.y + off;
        return (
          <g key={t.id} style={{ transition: "transform 0.5s linear" }} transform={`translate(${x} ${y})`}>
            <title>{`${t.id} towards ${t.towards} · ${Math.round(t.occupancy)}% full${t.delayed ? " · held" : ""}`}</title>
            <rect x={vert ? -4 : -8} y={vert ? -8 : -4} width={vert ? 8 : 16} height={vert ? 16 : 8} rx={2}
              fill={t.color} stroke={t.delayed ? "#C8352E" : "#FBFCFD"} strokeWidth={t.delayed ? 2.5 : 1.2} />
          </g>
        );
      })}
      {net.stations.map((s) => {
        const st = live.get(s.id);
        const inter = (linesAt.get(s.id) ?? []).length > 1;
        const fill = st ? (st.closed ? "#16202B" : occColor(st.density)) : "#FBFCFD";
        const sel = selected === s.id;
        return (
          <g key={s.id} onClick={() => onSelect?.(s.id)} className={onSelect ? "cursor-pointer" : undefined}
            tabIndex={onSelect ? 0 : undefined} role={onSelect ? "button" : undefined} aria-label={s.name}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onSelect?.(s.id)}>
            {sel && <circle cx={s.x} cy={s.y} r={15} fill="none" stroke="#16202B" strokeWidth={2} />}
            <circle cx={s.x} cy={s.y} r={inter ? 9 : 6.5} fill={fill}
              stroke={inter || !st ? "#16202B" : "#FBFCFD"} strokeWidth={inter ? 3 : st ? 2 : 1.5} />
            {label(s.id, s.x, s.y, s.name)}
          </g>
        );
      })}
    </svg>
  );
}
