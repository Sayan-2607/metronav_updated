"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { isStaff, signOut } from "@/lib/api";
import { useSession } from "@/lib/hooks";

export default function Nav() {
  const user = useSession();
  const path = usePathname();
  const items = [
    { href: "/", label: "Plan a journey" },
    { href: "/live", label: "Live network" },
    { href: "/tickets", label: "My tickets" },
    ...(isStaff(user?.role) ? [{ href: "/admin", label: "Operations" }, { href: "/admin/simulation", label: "Simulation" }] : []),
  ];
  return (
    <header className="border-b border-rule bg-panel">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
        <Link href="/" className="flex items-center gap-2 font-sign text-2xl font-bold tracking-tight text-ink">
          <span aria-hidden className="flex h-7 w-7 items-center justify-center rounded-full border-[3px] border-ink text-sm leading-none">M</span>
          MetroNav
        </Link>
        <nav className="order-last -mx-1 flex basis-full gap-1 overflow-x-auto whitespace-nowrap text-[15px] md:order-none md:basis-auto md:flex-1">
          {items.map((i) => {
            const active = i.href === "/" ? path === "/" : path.startsWith(i.href) && !(i.href === "/admin" && path.startsWith("/admin/simulation"));
            return (
              <Link key={i.href} href={i.href}
                className={`rounded px-2.5 py-1.5 ${active ? "bg-ink text-panel" : "text-ink hover:bg-concrete"}`}>
                {i.label}
              </Link>
            );
          })}
        </nav>
        {user ? (
          <div className="ml-auto flex items-center gap-3 text-sm">
            <span className="text-mute">{user.name}{user.role !== "PASSENGER" && ` (${user.role.toLowerCase()})`}</span>
            <button onClick={signOut} className="underline underline-offset-2">Sign out</button>
          </div>
        ) : (
          <Link href="/login" className="ml-auto text-sm underline underline-offset-2">Sign in</Link>
        )}
      </div>
    </header>
  );
}
