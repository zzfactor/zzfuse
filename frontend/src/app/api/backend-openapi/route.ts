import { NextResponse } from "next/server";
import { backendURL } from "@/lib/backend";

export async function GET() {
    try {
        const response = await fetch(backendURL("/openapi.yaml"), { signal: AbortSignal.timeout(15_000), cache: "no-store" });
        return new NextResponse(await response.text(), { status: response.status, headers: { "Content-Type": "application/yaml" } });
    } catch {
        return NextResponse.json({ error: "API documentation is unavailable." }, { status: 502 });
    }
}
