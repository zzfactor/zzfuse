"use client";
import { errorMessage } from "@/lib/errors";
import React, { useState } from "react";
import { useSearchAvailable } from "@/lib/use-search-available";
import { useSession } from "next-auth/react";
import JsonView from '@uiw/react-json-view';
import { darkTheme } from '@uiw/react-json-view/dark';
import { lightTheme } from '@uiw/react-json-view/light';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useTheme } from "next-themes";

// Recursively parse all stringified JSON fields
function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

type SearchResult = { text: string; score?: number; parsedText: unknown };

function deepParseJSON(obj: unknown): unknown {
    if (typeof obj === "string") {
        try {
            const parsed = JSON.parse(obj);
            if (typeof parsed === "object" && parsed !== null) {
                return deepParseJSON(parsed);
            }
            return parsed;
        } catch {
            return obj; // Return original string if parsing fails
        }
    } else if (Array.isArray(obj)) {
        return obj.map(deepParseJSON);
    } else if (typeof obj === "object" && obj !== null) {
        const result: Record<string, unknown> = {};
        for (const [key, value] of Object.entries(obj)) {
            result[key] = deepParseJSON(value);
        }
        return result;
    }
    return obj;
}

export default function SearchPage() {
    const [query, setQuery] = useState("");
    const [results, setResults] = useState<SearchResult[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [expanded, setExpanded] = useState<{ [key: number]: boolean }>({});
    const { data: session, status } = useSession();
    const { resolvedTheme } = useTheme();
    const available = useSearchAvailable("vector");

    const handleSearch = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError("");
        setResults([]);
        if (status !== "authenticated" || !session?.jwt) {
            setError("You must be logged in to search.");
            setLoading(false);
            return;
        }
        try {
            const res = await fetch("/api/vector-search", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                    "Authorization": `Bearer ${session?.jwt}`,
                },
                body: JSON.stringify({ query }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || "Search failed");
            const processedResults = (Array.isArray(data.results) ? data.results : []).map((item: { text: string; score?: number }) => ({
                ...item,
                parsedText: deepParseJSON(item.text), // Store the parsed version
            }));
            setResults(processedResults || []);
            setExpanded({});
        } catch (err) {
            setError(errorMessage(err) || "Unknown error");
        } finally {
            setLoading(false);
        }
    };

    const toggleExpand = (idx: number) => {
        setExpanded((prev) => ({ ...prev, [idx]: !prev[idx] }));
    };

    return (
        <div className="min-h-screen bg-background text-foreground flex flex-col items-center py-12 px-4">
            <h1 className="text-2xl font-bold mb-6">Vector Store Search</h1>
            {available === false && <p className="text-muted-foreground mb-6">Vector search is unavailable in this deployment.</p>}
            <form
                onSubmit={handleSearch}
                className="w-full max-w-xl flex flex-col sm:flex-row gap-4 mb-8 bg-card/80 border border-border shadow-lg rounded-xl p-4"
            >
                <Input
                    type="text"
                    placeholder="Search logs…"
                    className="flex-1 bg-background border-none shadow-none focus:ring-2 focus:ring-primary/50 text-lg px-4 py-3 rounded-lg"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                />
                <Button
                    type="submit"
                    variant="default"
                    size="default"
                    className="font-semibold px-6 py-3 text-lg shadow-md"
                    disabled={loading || available !== true}
                >
                    {loading ? "Searching…" : "Search"}
                </Button>
            </form>
            {error && <div className="text-red-600 mb-4">{error}</div>}
            <div className="w-full max-w-4xl mx-auto">
                {results.length === 0 && !loading && (
                    <div className="text-muted-foreground text-center">No results yet. Try searching!</div>
                )}
                {results.length > 0 && (
                    <div className="overflow-x-auto rounded-lg shadow-lg bg-card border border-border">
                        <table className="w-full divide-y divide-border">
                            <thead>
                                <tr>
                                    <th className="px-6 py-3 text-left text-xs font-semibold text-muted-foreground uppercase tracking-wider w-32">Score</th>
                                    <th className="px-6 py-3 text-left text-xs font-semibold text-muted-foreground uppercase tracking-wider">Preview</th>
                                    <th className="px-6 py-3 w-32"></th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {results.map((result, idx) => {
                                    const parsedData = result.parsedText;

                                    // Preview: show log message/body if available, else fallback
                                    let previewText = "";
                                    if (isRecord(parsedData)) {
                                        if (isRecord(parsedData.PageContent) && typeof parsedData.PageContent.body === "string") {
                                            previewText = parsedData.PageContent.body;
                                        } else if (typeof parsedData.body === "string") {
                                            previewText = parsedData.body;
                                        } else {
                                            previewText = "No preview available";
                                        }
                                    } else if (typeof parsedData === "string") {
                                        previewText = parsedData;
                                    } else {
                                        previewText = "No preview available";
                                    }
                                    const displayPreview = previewText.length > 120
                                        ? previewText.substring(0, 117) + "..."
                                        : previewText;

                                    const displayScore =
                                        isRecord(parsedData) && typeof parsedData.Score === 'number'
                                            ? Number(parsedData.Score).toFixed(3)
                                            : (result.score !== undefined ? Number(result.score).toFixed(3) : "N/A");

                                    return (
                                        <React.Fragment key={idx}>
                                            <tr className="hover:bg-muted transition">
                                                <td className="px-6 py-4 font-mono text-sm w-32 align-top">{displayScore}</td>
                                                <td className="px-6 py-4 font-mono text-xs align-top" style={{ maxWidth: '1000px', wordBreak: 'break-word' }}>{displayPreview}</td>
                                                <td className="px-6 py-4 w-32 align-top">
                                                    <Button
                                                        type="button"
                                                        variant="outline"
                                                        size="sm"
                                                        onClick={() => toggleExpand(idx)}
                                                        className="font-semibold"
                                                    >
                                                        {expanded[idx] ? "Hide" : "Show"} JSON
                                                    </Button>
                                                </td>
                                            </tr>
                                            {expanded[idx] && (
                                                <tr>
                                                    <td colSpan={3} className="px-6 pb-6">
                                                        <div className="bg-background rounded-xl shadow-lg p-6 mt-2 border border-border overflow-x-auto">
                                                            <JsonView
                                                                value={typeof parsedData === "object" && parsedData !== null ? parsedData : { value: parsedData }}
                                                                style={resolvedTheme === "dark" ? darkTheme : lightTheme}
                                                                collapsed={2}
                                                                enableClipboard={true}
                                                                displayObjectSize={false}
                                                                displayDataTypes={false}
                                                            />
                                                        </div>
                                                    </td>
                                                </tr>
                                            )}
                                        </React.Fragment>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
        </div>
    );
}
