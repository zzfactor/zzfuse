import type { NextApiRequest, NextApiResponse } from "next";
import { forwardJSON } from "@/lib/pages-proxy";

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
    if (req.method !== "POST") return res.setHeader("Allow", "POST").status(405).json({ error: "Method not allowed" });
    const { email, token, password } = req.body || {};
    if (typeof email === "string" && email.trim() && !token) {
        return forwardJSON(res, "/api/v1/auth/request-password-reset", "POST", { email: email.trim() });
    }
    if (typeof token === "string" && token.trim() && typeof password === "string" && password.length >= 8) {
        return forwardJSON(res, "/api/v1/auth/reset-password", "POST", { token, new_password: password });
    }
    return res.status(400).json({ error: "Email or a reset token and password of at least 8 characters are required." });
}
