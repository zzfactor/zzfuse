// Server-only configuration. Keep credentials on the configured backend origin.
export function backendURL(path: string): string {
    if (!path.startsWith("/") || path.startsWith("//")) {
        throw new Error("Backend endpoint must be an absolute path.");
    }
    return new URL(path, process.env.BACKEND_URL || "http://localhost:18888").toString();
}

export async function readBackendResponse(response: Response): Promise<unknown> {
    const text = await response.text();
    if (!text) return null;
    try {
        return JSON.parse(text);
    } catch {
        return { error: text };
    }
}
