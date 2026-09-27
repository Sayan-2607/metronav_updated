"use client";
import { useEffect, useRef, useState } from "react";
import { api, session, WS } from "./api";
import type { Network, Scenario, Station, Train, User } from "./types";

export function useNetwork() {
  const [net, setNet] = useState<Network | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    api<Network>("/api/v1/network").then(setNet).catch((e) => setError(e.message));
  }, []);
  return { net, error };
}

export function useSession() {
  const [user, setUser] = useState<User | null>(null);
  useEffect(() => {
    const read = () => setUser(session()?.user ?? null);
    read();
    window.addEventListener("metronav-session", read);
    window.addEventListener("storage", read);
    return () => {
      window.removeEventListener("metronav-session", read);
      window.removeEventListener("storage", read);
    };
  }, []);
  return user;
}

export type LiveState = {
  trains: Train[]; stations: Station[]; scenarios: Scenario[]; simTime: string | null;
  status: "connecting" | "live" | "reconnecting"; incidents: number;
};

/** Subscribes to the realtime WebSocket, reconnecting with capped exponential backoff. */
export function useLive(): LiveState {
  const [s, setS] = useState<LiveState>({ trains: [], stations: [], scenarios: [], simTime: null, status: "connecting", incidents: 0 });
  const retry = useRef(0);
  useEffect(() => {
    let ws: WebSocket | null = null;
    let timer: ReturnType<typeof setTimeout>;
    let closed = false;
    const connect = () => {
      ws = new WebSocket(WS);
      ws.onopen = () => { retry.current = 0; setS((p) => ({ ...p, status: "live" })); };
      ws.onmessage = (ev) => {
        const m = JSON.parse(ev.data);
        setS((p) => {
          switch (m.event_type) {
            case "TRAIN_POSITIONS_UPDATED":
              return { ...p, trains: m.payload.trains ?? [], simTime: m.payload.sim_time };
            case "CROWD_UPDATED":
              return { ...p, stations: m.payload.stations ?? [], scenarios: m.payload.scenarios ?? [], simTime: m.payload.sim_time };
            case "INCIDENT_CREATED":
              return { ...p, incidents: p.incidents + 1 };
            default:
              return p;
          }
        });
      };
      ws.onclose = () => {
        if (closed) return;
        setS((p) => ({ ...p, status: "reconnecting" }));
        const delay = Math.min(15000, 500 * 2 ** retry.current++);
        timer = setTimeout(connect, delay);
      };
    };
    connect();
    return () => { closed = true; clearTimeout(timer); ws?.close(); };
  }, []);
  return s;
}

export function fmtTime(iso: string | null) {
  if (!iso) return "--:--";
  // sim_time is serialised with its own offset (Asia/Kolkata); show wall-clock digits from the string
  const m = iso.match(/T(\d{2}:\d{2})/);
  return m ? m[1] : "--:--";
}
