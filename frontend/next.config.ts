import type { NextConfig } from "next";

const resetHeaders = [
    { key: "Referrer-Policy", value: "no-referrer" },
    { key: "Cache-Control", value: "no-store" },
];

const nextConfig: NextConfig = {
    output: "standalone",
    async headers() {
        return [
            { source: "/reset-password", headers: resetHeaders },
            { source: "/reset-password/:path*", headers: resetHeaders },
        ];
    },
};

export default nextConfig;
