"use client";
import { useEffect, useState } from "react";

export function useSearchAvailable(kind: "sql" | "vector") {
    const [available, setAvailable] = useState<boolean | null>(null);
    useEffect(() => {
        const controller = new AbortController();
        fetch("/api/search-capabilities", { signal: controller.signal })
            .then(response => response.json())
            .then(data => setAvailable(data[kind] === true))
            .catch(() => { if (!controller.signal.aborted) setAvailable(false); });
        return () => controller.abort();
    }, [kind]);
    return available;
}
