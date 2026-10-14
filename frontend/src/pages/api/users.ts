import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    const authorization = req.headers.authorization;
    if (!authorization?.startsWith("Bearer ") || !authorization.slice(7).trim()) {
        return res.status(401).json({ error: "Missing or invalid Authorization header" });
    }
    if (req.method !== "GET") return res.setHeader("Allow", "GET").status(405).json({ error: "Method not allowed" });
    return forwardJSON(res, "/api/v1/admin/users", "GET", undefined, authorization);
}
