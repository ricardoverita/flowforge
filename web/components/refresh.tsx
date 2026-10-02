"use client";

import { useTransition } from "react";
import { useRouter } from "next/navigation";

export function Refresh() {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  return <button className="secondary" disabled={pending} onClick={() => startTransition(() => router.refresh())}>
    {pending ? "Refreshing…" : "Refresh"}
  </button>;
}
