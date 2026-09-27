import type { NetStation } from "@/lib/types";

export default function StationSelect({ id, label, value, onChange, stations }: {
  id: string; label: string; value: string; onChange: (v: string) => void; stations: NetStation[];
}) {
  const sorted = [...stations].sort((a, b) => a.name.localeCompare(b.name));
  return (
    <label htmlFor={id} className="block">
      <span className="mb-1 block text-sm text-mute">{label}</span>
      <select id={id} value={value} onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-md border border-rule bg-panel px-3 py-2.5 font-sign text-lg text-ink">
        <option value="">Choose a station</option>
        {sorted.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
      </select>
    </label>
  );
}
