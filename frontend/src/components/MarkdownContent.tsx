"use client";
import { errorMessage } from "@/lib/errors";
import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useMounted } from "@/lib/use-mounted";
import { useTheme } from "next-themes";

// Use the raw GitHub URL for the actual markdown content
const GITHUB_RAW_URL = "https://raw.githubusercontent.com/zzfactor/zzfuse/main/README.md";

export default function MarkdownContent() {
    const [content, setContent] = useState<string>("");
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const { resolvedTheme } = useTheme();
    const mounted = useMounted();

    useEffect(() => {
        if (!mounted) return;
        fetch(GITHUB_RAW_URL)
            .then((res) => {
                if (!res.ok) throw new Error("Failed to fetch markdown");
                return res.text();
            })
            .then(setContent)
            .catch((err) => setError(errorMessage(err)))
            .finally(() => setLoading(false));
    }, [mounted]);

    // Dynamically import the correct CSS for the theme
    useEffect(() => {
        if (!mounted) return;
        if (resolvedTheme === "dark") {
            import("github-markdown-css/github-markdown-dark.css");
        } else {
            import("github-markdown-css/github-markdown-light.css");
        }
    }, [resolvedTheme, mounted]);

    if (!mounted) {
        // Render nothing on the server to avoid hydration mismatch
        return null;
    }

    if (loading) return <div className="text-center py-8 text-neutral-500">Loading project details…</div>;
    if (error) return <div className="text-center py-8 text-red-500">{error}</div>;

    return (
        <div className="markdown-body">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
        </div>
    );
}
