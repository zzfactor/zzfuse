import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    const authorization = req.headers.authorization;
    if (!authorization?.startsWith("Bearer ") || !authorization.slice(7).trim()) {
        return res.status(401).json({ error: "Missing or invalid Authorization header" });
    }
    const id = req.query.id;
    if (typeof id !== "string" || !/^[1-9]\d*$/.test(id) || !Number.isSafeInteger(Number(id))) {
        return res.status(400).json({ error: "Invalid user ID" });
    }
    if (req.method === "GET") {
        return forwardJSON(res, `/api/v1/admin/user/access?user_id=${id}`, "GET", undefined, authorization);
    }
    if (req.method !== "POST") {
        return res.setHeader("Allow", "GET, POST").status(405).json({ error: "Method not allowed" });
    }
    const body: unknown = req.body;
    if (!body || typeof body !== "object" || Array.isArray(body) || Object.keys(body).length !== 1) {
        return res.status(400).json({ error: "Provide one access change" });
    }
    const change = body as { schemas?: unknown; disabled?: unknown; revoke_sessions?: unknown };
    if (Array.isArray(change.schemas) && change.schemas.every(schema => typeof schema === "string")) {
        return forwardJSON(res, "/api/v1/admin/user/schemas", "POST",
            { user_id: Number(id), schemas: change.schemas }, authorization);
    }
    if (typeof change.disabled === "boolean") {
        return forwardJSON(res, "/api/v1/admin/user/disabled", "POST",
            { user_id: Number(id), disabled: change.disabled }, authorization);
    }
    if (change.revoke_sessions === true) {
        return forwardJSON(res, "/api/v1/admin/user/revoke-sessions", "POST",
            { user_id: Number(id) }, authorization);
    }
    return res.status(400).json({ error: "Invalid access change" });
}
