/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,

  async redirects() {
    return [
      { source: "/markets", destination: "/market", permanent: false },
      { source: "/fundamentals", destination: "/research", permanent: false },
      { source: "/settings", destination: "/account", permanent: false },
      { source: "/overview", destination: "/", permanent: false },
    ];
  },

  // The API origin is a separately deployed service; no rewrites proxy it, so
  // cookies and CSRF behave exactly as they would in production.
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
        ],
      },
    ];
  },
};

export default nextConfig;
