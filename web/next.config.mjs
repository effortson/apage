/** @type {import('next').NextConfig} */
const apiBase = process.env.APAGE_API_URL || "http://localhost:8080";

const nextConfig = {
  reactStrictMode: true,
  // Proxy API calls to apage-api so the browser stays same-origin and the
  // session cookie flows (spec §12 frontend tech alignment).
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${apiBase}/api/v1/:path*` },
      { source: "/admin/v1/:path*", destination: `${apiBase}/admin/v1/:path*` },
    ];
  },
};

export default nextConfig;
