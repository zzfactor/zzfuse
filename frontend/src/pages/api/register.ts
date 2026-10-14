import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    if (req.method !== "POST") return res.setHeader("Allow", "POST").status(405).json({ error: "Method not allowed" });
    const { username, email, password } = req.body || {};
    if (typeof username !== "string" || username.trim().length < 3 || username.trim().length > 64 || typeof email !== "string" || !email.trim() || typeof password !== "string" || password.length < 8) {
        return res.status(400).json({ error: "A username of 3 to 64 characters, email, and password of at least 8 characters are required." });
    }
    return forwardJSON(res, "/api/v1/auth/register", "POST", { username: username.trim(), email: email.trim(), password });
}
