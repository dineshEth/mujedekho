import { NextResponse, type NextRequest } from "next/server";

/**
 * Edge guard: app pages require the auth cookie. The cookie's presence only —
 * real verification happens server-side inside the Go API on every proxy call.
 */
const PROTECTED = ["/dashboard", "/monitors", "/incidents", "/notifications", "/status-pages", "/settings"];

export function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;
  const needsAuth = PROTECTED.some((p) => pathname === p || pathname.startsWith(p + "/"));
  if (!needsAuth) return NextResponse.next();

  const hasToken = req.cookies.has("s24x7_at");
  if (hasToken) return NextResponse.next();

  const login = new URL("/login", req.url);
  login.searchParams.set("next", pathname);
  return NextResponse.redirect(login);
}

export const config = {
  matcher: ["/dashboard/:path*", "/monitors/:path*", "/incidents/:path*", "/notifications/:path*", "/status-pages/:path*", "/settings/:path*"],
};
