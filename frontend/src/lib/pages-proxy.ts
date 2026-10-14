import type { NextApiResponse } from "next";
import { backendURL, readBackendResponse } from "@/lib/backend";

export async function forwardJSON(res: NextApiResponse, path: string, method: string, body?: unknown, authorization?: string) {
    try {
        const backend = await fetch(backendURL(path), {
            method,
            headers: {
                "Content-Type": "application/json",
                ...(authorization ? { Authorization: authorization } : {}),
            },
            ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
            signal: AbortSignal.timeout(15_000),
            cache: "no-store",
        });
        const data = await readBackendResponse(backend);
        if (data === null) return res.status(backend.status).end();
        return res.status(backend.status).json(data);
    } catch {
        return res.status(502).json({ error: "Failed to connect to backend." });
    }
}
