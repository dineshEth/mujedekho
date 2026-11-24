/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  async rewrites() {
    // Public status pages are served by the Go backend (Phase 9); the Next
    // dev server proxies them so one origin serves everything locally.
    return [];
  },
};

export default nextConfig;
