import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    const authorization = req.headers.authorization;
    if (!authorization?.startsWith("Bearer ") || !authorization.slice(7).trim()) {
        return res.status(401).json({ error: "Missing or invalid Authorization header" });
    }
    if (req.method !== "DELETE") return res.setHeader("Allow", "DELETE").status(405).json({ error: "Method not allowed" });
    const id = req.query.id;
    if (typeof id !== "string" || !/^[1-9]\d*$/.test(id)) return res.status(400).json({ error: "Invalid user ID" });
    return forwardJSON(res, `/api/v1/admin/users/${id}`, "DELETE", undefined, authorization);
}
