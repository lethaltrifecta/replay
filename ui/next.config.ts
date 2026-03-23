import type { NextConfig } from "next";

const apiOrigin = process.env.REPLAY_API_ORIGIN?.replace(/\/$/, "");

const nextConfig: NextConfig = {
  async redirects() {
    return [
      {
        source: "/selection",
        destination: "/launchpad",
        permanent: false,
      },
      {
        source: "/drift",
        destination: "/divergence",
        permanent: false,
      },
      {
        source: "/drift/:path*",
        destination: "/divergence/:path*",
        permanent: false,
      },
      {
        source: "/compare",
        destination: "/shadow-replay",
        permanent: false,
      },
      {
        source: "/experiments",
        destination: "/gauntlet",
        permanent: false,
      },
      {
        source: "/experiments/:path*",
        destination: "/gauntlet/:path*",
        permanent: false,
      },
    ];
  },
  async rewrites() {
    if (!apiOrigin) {
      return [];
    }

    return [
      {
        source: "/api/v1/:path*",
        destination: `${apiOrigin}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
