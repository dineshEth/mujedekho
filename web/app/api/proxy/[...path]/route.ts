import { NextResponse } from "next/server";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";
const TOKEN_COOKIE = "s24x7_at";

/**
 * Catch-all authenticated proxy: /api/proxy/<api/v1 path>.
 * The browser never touches the backend origin and never sees the token:
 * the httpOnly cookie is attached server-side as an Authorization header.
 */
async function forward(req: Request, segments: string[]) {
  const token = readCookie(req);
  if (!token) {
    return NextResponse.json(
      { error: { code: "unauthorized", message: "Not signed in." } },
      { status: 401 },
    );
  }

  const url = new URL(req.url);
  const upstream = new URL(`/api/v1/${segments.join("/")}`, API_BASE);
  upstream.search = url.search;

  let res: Response;
  try {
    res = await fetch(upstream, {
      method: req.method,
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: hasBody(req.method) ? (await req.text()) || undefined : undefined,
      cache: "no-store",
    });
  } catch {
    return NextResponse.json(
      { error: { code: "upstream_unavailable", message: "The monitoring API is unreachable." } },
      { status: 502 },
    );
  }

  // 204/304 must not carry a body: the Response constructor throws otherwise.
  if (res.status === 204 || res.status === 304) {
    return new NextResponse(null, { status: res.status });
  }
  const data = await res.arrayBuffer();
  return new NextResponse(data, {
    status: res.status,
    headers: { "Content-Type": res.headers.get("content-type") ?? "application/json" },
  });
}

function readCookie(req: Request): string | null {
  const raw = req.headers.get("cookie") ?? "";
  for (const part of raw.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === TOKEN_COOKIE) return decodeURIComponent(v.join("="));
  }
  return null;
}

function hasBody(method: string): boolean {
  return ["POST", "PUT", "PATCH", "DELETE"].includes(method);
}

type Ctx = { params: Promise<{ path: string[] }> };

export async function GET(req: Request, ctx: Ctx) {
  const { path } = await ctx.params;
  return forward(req, path);
}
export async function POST(req: Request, ctx: Ctx) {
  const { path } = await ctx.params;
  return forward(req, path);
}
export async function PUT(req: Request, ctx: Ctx) {
  const { path } = await ctx.params;
  return forward(req, path);
}
export async function DELETE(req: Request, ctx: Ctx) {
  const { path } = await ctx.params;
  return forward(req, path);
}
