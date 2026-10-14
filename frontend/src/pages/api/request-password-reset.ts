import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    if (req.method !== "POST") return res.setHeader("Allow", "POST").status(405).json({ error: "Method not allowed" });
    const { email } = req.body || {};
    if (typeof email !== "string" || !email.trim()) return res.status(400).json({ error: "Email is required" });
    return forwardJSON(res, "/api/v1/auth/request-password-reset", "POST", { email: email.trim() });
}
