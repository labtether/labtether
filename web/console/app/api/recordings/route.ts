import { NextResponse } from "next/server";

import { backendAuthHeadersWithCookie, resolvedBackendBaseURLs } from "../../../lib/backend";

export async function GET(request: Request) {
  try {
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/recordings`, {
      cache: "no-store",
      headers: {
        ...backendAuthHeadersWithCookie(request),
      },
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { recordings: [] }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "recordings query failed" },
      { status: 502 },
    );
  }
}

export async function POST(request: Request) {
  try {
    const body = await request.text();
    const base = await resolvedBackendBaseURLs();
    const response = await fetch(`${base.api}/recordings`, {
      method: "POST",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        ...backendAuthHeadersWithCookie(request),
      },
      body,
    });
    const payload = await safeJSON(response);
    return NextResponse.json(payload ?? { error: "failed to start recording" }, { status: response.status });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to start recording" },
      { status: 502 },
    );
  }
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
