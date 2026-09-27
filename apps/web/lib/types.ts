export type Station = {
  id: string; name: string; x: number; y: number; lines: string[];
  density: number; expected: number; people: number; capacity: number;
  source: "simulated" | "cv"; closed: boolean; level: "low" | "medium" | "high" | "severe";
};
export type Train = {
  id: string; line: string; color: string; direction: number; towards: string;
  prev_station: string; next_station: string; eta_next_min: number; x: number; y: number;
  at_terminal: boolean; delayed: boolean; occupancy: number; coaches: number[];
};
export type Line = { id: string; name: string; color: string; coaches: number; headway_min: number; stations: string[] };
export type NetStation = { id: string; name: string; x: number; y: number; weight: number; platform_capacity: number };
export type Network = { note: string; stations: NetStation[]; lines: Line[] };
export type Scenario = { id: string; type: string; station_id?: string; line_id?: string; delay_min?: number; surge_pct?: number; until: string };
export type CoachRec = { recommended_coach: number; destination_exit_coach: number; scores: number[]; formula: string };
export type Leg = { line: string; color: string; from: string; to: string; stops: string[]; minutes: number };
export type RouteResult = {
  profile: string; matched_profiles: string[]; total_min: number; wait_min: number; ride_min: number;
  transfer_walk_min: number; transfers: number; stations_travelled: number; fare: number; crowd_score: number; legs: Leg[];
  eta_prediction: { p10_min: number; p50_min: number; p90_min: number; model: string; source: string };
  next_departure?: { train: Train; eta_min: number; coach_recommendation: CoachRec };
};
export type Ticket = {
  id: string; from_station: string; to_station: string; profile: string; coach: number; fare: number;
  token: string; status: "ACTIVE" | "USED"; created_at: string; expires_at: string; used_at?: string;
};
export type Incident = {
  id: string; type: string; severity: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL"; station_id?: string; line_id?: string;
  title: string; detail: string; source: string; status: "OPEN" | "ACKNOWLEDGED" | "RESOLVED"; created_at: string;
};
export type User = { id: string; email: string; name: string; role: string };
