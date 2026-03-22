import { NextResponse } from "next/server";
import { backendBaseURLs } from "../../../lib/backend";

export async function GET() {
  const { api } = backendBaseURLs();
  try {
    const res = await fetch(`${api}/healthz`, { cache: "no-store" });
    if (!res.ok) {
      return NextResponse.json({ status: "error" }, { status: res.status });
    }
    return NextResponse.json({ status: "ok" });
  } catch {
    return NextResponse.json({ status: "error" }, { status: 502 });
  }
}
