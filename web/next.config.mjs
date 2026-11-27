/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // NOTE: keep `output: "standalone"` out until the Phase 9 Dockerfile — it
  // breaks `next start` (npm start) and has no benefit in local dev.
  async rewrites() {
    // Public status pages are served by the Go backend (Phase 9); the Next
    // dev server proxies them so one origin serves everything locally.
    return [];
  },
};

export default nextConfig;
