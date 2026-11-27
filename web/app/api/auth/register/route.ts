import { NextResponse } from "next/server";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

const COOKIE = {
  name: "s24x7_at",
  httpOnly: true,
  sameSite: "lax" as const,
  secure: process.env.NODE_ENV === "production",
  path: "/",
};

/** POST /api/auth/register — proxies to the Go API and sets the httpOnly cookie. */
export async function POST(req: Request) {
  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: { code: "bad_request", message: "Malformed JSON." } }, { status: 400 });
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE}/api/v1/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    return NextResponse.json(
      { error: { code: "upstream_unavailable", message: "The monitoring API is unreachable. Is it running?" } },
      { status: 502 },
    );
  }

  const data = await res.json().catch(() => ({}));
  if (!res.ok || !data?.access_token) {
    return NextResponse.json(data, { status: res.status });
  }

  const out = NextResponse.json({ user: data.user, expires_in: data.expires_in }, { status: 201 });
  out.cookies.set(COOKIE.name, data.access_token, {
    ...COOKIE,
    maxAge: Number(data.expires_in ?? 900),
  });
  return out;
}
