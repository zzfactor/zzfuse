import { NextRequest, NextResponse } from "next/server";
import { backendURL, readBackendResponse } from "@/lib/backend";

export async function proxySearch(req: NextRequest, endpoint: string | undefined, kind: "SQL" | "Vector") {
    const authorization = req.headers.get("authorization");
    if (!authorization?.startsWith("Bearer ") || !authorization.slice(7).trim()) {
        return NextResponse.json({ error: "Missing or invalid Authorization header" }, { status: 401 });
    }
    let body: Record<string, unknown>;
    try {
        body = await req.json();
        if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("Invalid body");
    } catch {
        return NextResponse.json({ error: "Invalid JSON request body." }, { status: 400 });
    }
    if (typeof body.query !== "string" || !body.query.trim()) {
        return NextResponse.json({ error: "Query is required." }, { status: 400 });
    }
    if (!endpoint) {
        return NextResponse.json({ error: `${kind} search is unavailable in this deployment.` }, { status: 503 });
    }
    const payload: Record<string, unknown> = { query: body.query };
    if (kind === "Vector") {
        payload.k = typeof body.k === "number" && Number.isFinite(body.k) && body.k > 0 ? Math.min(100, Math.floor(body.k)) : 10;
        payload.offset = typeof body.offset === "number" && Number.isFinite(body.offset) && body.offset >= 0 ? Math.min(10_000, Math.floor(body.offset)) : 0;
        if (typeof body.namespace === "string") payload.namespace = body.namespace;
        if (body.filter && typeof body.filter === "object" && !Array.isArray(body.filter)) payload.filter = body.filter;
    }
    try {
        const response = await fetch(backendURL(endpoint), {
            method: "POST",
            headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: authorization },
            body: JSON.stringify(payload),
            signal: AbortSignal.timeout(15_000),
            cache: "no-store",
        });
        const data = await readBackendResponse(response);
        if (data === null) return new NextResponse(null, { status: response.status });
        return NextResponse.json(data, { status: response.status });
    } catch {
        return NextResponse.json({ error: "Failed to connect to backend." }, { status: 502 });
    }
}
