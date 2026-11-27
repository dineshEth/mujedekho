import { NextResponse } from "next/server";

/** POST /api/auth/logout — clears the httpOnly cookie client-side. The access
 * token was short-lived; revocation endpoints land with Phase 6 hardening. */
export async function POST() {
  const out = NextResponse.json({ ok: true });
  out.cookies.set("s24x7_at", "", { httpOnly: true, path: "/", maxAge: 0 });
  return out;
}
