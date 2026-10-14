"use client";
import { errorMessage } from "@/lib/errors";
import React, { useState, useRef } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Parser } from "node-sql-parser";
import { useSession } from "next-auth/react";
import {
    useReactTable,
    getCoreRowModel,
    getSortedRowModel,
    flexRender,
    SortingState,
    ColumnDef,
    Header as TanStackHeader,
} from "@tanstack/react-table";
import {
    Table,
    TableHeader,
    TableBody,
    TableRow,
    TableHead,
    TableCell,
} from "@/components/ui/table";
import { DndContext, closestCenter, DragEndEvent } from '@dnd-kit/core';
import { arrayMove, SortableContext, horizontalListSortingStrategy, useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import JsonView from '@uiw/react-json-view';
import { darkTheme } from '@uiw/react-json-view/dark';
import { lightTheme } from '@uiw/react-json-view/light';
import { useTheme } from "next-themes";
import { useSearchAvailable } from "@/lib/use-search-available";

type SignalRow = Record<string, unknown>;
const parser = new Parser();
const preferredOrder = ["id", "signal_type", "body", "attributes", "resource_attributes", "metadata", "timestamp", "created_at", "updated_at", "name", "status"];

// Function to safely parse JSON strings
function tryParseJSON(str: string) {
    try {
        const parsed: unknown = JSON.parse(str);
        return typeof parsed === "object" && parsed !== null ? parsed : str;
    } catch {
        return str;
    }
}

// Check if a column contains JSON data
function isJSONColumn(col: string): boolean {
    return ['attributes', 'resource_attributes', 'metadata'].includes(col);
}

// Drag-and-drop for column reordering
function DraggableTableHead({ header }: { header: TanStackHeader<SignalRow, unknown> }) {
    const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: header.id });
    const style = {
        ...header.getSize() ? { width: header.getSize() } : {},
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
        cursor: header.column.getCanSort() ? "pointer" : "default",
        position: "relative" as const,
        zIndex: isDragging ? 100 : undefined,
    };
    return (
        <TableHead
            ref={setNodeRef}
            style={style}
            onClick={header.column.getCanSort() ? header.column.getToggleSortingHandler() : undefined}
            className={
                (header.column.getCanSort() ? "cursor-pointer select-none " : "") +
                "group"
            }
            {...attributes}
        >
            <span {...listeners} className="inline-block mr-1 align-middle cursor-grab" title="Drag to reorder">≡</span>
            {flexRender(header.column.columnDef.header, header.getContext())}
            {header.column.getCanSort() && (
                <span className="ml-1">
                    {header.column.getIsSorted() === "asc" ? "▲" : header.column.getIsSorted() === "desc" ? "▼" : ""}
                </span>
            )}
            {header.column.getCanResize() && (
                <div
                    onMouseDown={header.getResizeHandler()}
                    onTouchStart={header.getResizeHandler()}
                    className="absolute right-0 top-0 h-full w-2 cursor-col-resize group-hover:bg-primary/30 transition-colors"
                    style={{ zIndex: 10 }}
                    role="separator"
                    aria-orientation="vertical"
                    tabIndex={-1}
                />
            )}
        </TableHead>
    );
}

export default function SignalsPage() {
    const [namespace, setNamespace] = useState("trazr_vectors");
    const [query, setQuery] = useState(() => `SELECT * FROM ${"trazr_vectors"} ORDER BY id DESC`);
    const [results, setResults] = useState<SignalRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [validationError, setValidationError] = useState<string | null>(null);
    const { data: session, status } = useSession();
    const { resolvedTheme } = useTheme();
    const available = useSearchAvailable("sql");
    const [sorting, setSorting] = useState<SortingState>([]);
    const formRef = useRef<HTMLFormElement>(null);
    const [page, setPage] = useState(1);
    const [pageSize] = useState(100);
    const [hasMore, setHasMore] = useState(false);
    // Preferred column order (developer-controlled)
    // Compute initial column order from preferredOrder + any extra columns
    const initialColumnOrder = React.useMemo(() => {
        if (results.length === 0) return [];
        const allCols = Object.keys(results[0]);
        const ordered = preferredOrder.filter(col => allCols.includes(col));
        const extras = allCols.filter(col => !preferredOrder.includes(col));
        return [...ordered, ...extras];
    }, [results]);
    const [columnOrder, setColumnOrder] = useState<string[]>(initialColumnOrder);
    const [showFullContent, setShowFullContent] = useState(false);
    const effectiveColumnOrder = React.useMemo(() => [
        ...columnOrder.filter(col => initialColumnOrder.includes(col)),
        ...initialColumnOrder.filter(col => !columnOrder.includes(col)),
    ], [columnOrder, initialColumnOrder]);

    // Columns definition uses columnOrder
    const columns = React.useMemo<ColumnDef<SignalRow, unknown>[]>(() => {
        if (results.length === 0) return [];
        return effectiveColumnOrder.map((col) => {
            const isTimestamp = /time|date/i.test(col);
            return {
                accessorKey: col,
                header: () => col,
                cell: info => {
                    const val = info.getValue();
                    let display = val == null ? "" : String(val);
                    let isJSON = false;
                    let parsedJSON: object | null = null;

                    if (isTimestamp && typeof val === "string" && val.length >= 10) {
                        const date = new Date(val);
                        if (!isNaN(date.getTime())) {
                            display = date.toLocaleString();
                        }
                    } else if (typeof val === "object" && val !== null) {
                        display = JSON.stringify(val);
                        isJSON = isJSONColumn(col);
                        parsedJSON = val;
                    } else if (typeof val === "string" && isJSONColumn(col)) {
                        // Try to parse JSON string
                        const parsed = tryParseJSON(val);
                        parsedJSON = typeof parsed === "object" && parsed !== null ? parsed : null;
                        isJSON = parsedJSON !== null;
                        display = val;
                    } else if (val !== undefined && val !== null) {
                        display = String(val);
                    }

                    // Show JSON viewer when showFullContent is enabled and it's a JSON column
                    if (showFullContent && isJSON && parsedJSON) {
                        return (
                            <div className="max-w-full">
                                <JsonView
                                    value={parsedJSON}
                                    style={resolvedTheme === "dark" ? darkTheme : lightTheme}
                                    collapsed={2}
                                    enableClipboard={true}
                                    displayObjectSize={false}
                                    displayDataTypes={false}
                                />
                            </div>
                        );
                    }

                    return (
                        <span
                            className={showFullContent ? "block whitespace-pre-wrap break-words" : "truncate max-w-[200px] block"}
                            title={typeof display === "string" ? display : undefined}
                        >
                            {display}
                        </span>
                    );
                },
                enableResizing: true,
                size: col === 'id' ? 80 :
                    col === 'signal_type' ? 120 :
                        col === 'timestamp' ? 180 :
                            col === 'body' ? 250 :
                                col === 'attributes' || col === 'resource_attributes' || col === 'metadata' ? 200 :
                                    150,
                minSize: col === 'id' ? 60 : 100,
                maxSize: 800,
            };
        });
    }, [results, effectiveColumnOrder, showFullContent, resolvedTheme]);

    // React Compiler is not enabled; TanStack Table v8 remains intentionally mutable.
    // eslint-disable-next-line react-hooks/incompatible-library
    const table = useReactTable({
        data: results,
        columns,
        state: { sorting, columnOrder: effectiveColumnOrder },
        onSortingChange: setSorting,
        onColumnOrderChange: setColumnOrder,
        getCoreRowModel: getCoreRowModel(),
        getSortedRowModel: getSortedRowModel(),
        enableColumnResizing: true,
        columnResizeMode: "onChange",
        debugTable: false,
    });

    function validateQuery(query: string): string | null {
        try {
            const ast = parser.astify(query);
            if (Array.isArray(ast)) return "Only single queries are allowed.";
            if (ast.type !== "select") return "Only SELECT statements are allowed.";
            return null;
        } catch {
            return "Invalid SQL syntax.";
        }
    }

    function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
        setQuery(e.target.value);
        setValidationError(validateQuery(e.target.value));
        setPage(1);
    }

    function handleNamespaceChange(e: React.ChangeEvent<HTMLInputElement>) {
        setNamespace(e.target.value);
        setQuery(`SELECT * FROM ${e.target.value}`);
        setPage(1);
    }

    // Update query with LIMIT/OFFSET for pagination
    function buildPagedQuery(baseQuery: string, page: number, pageSize: number) {
        // Remove any existing LIMIT/OFFSET
        let q = baseQuery.replace(/\s+limit\s+\d+/i, '').replace(/\s+offset\s+\d+/i, '');
        q = q.trim().replace(/;$/, '');
        return `${q} LIMIT ${pageSize} OFFSET ${(page - 1) * pageSize}`;
    }

    async function searchPage(targetPage: number) {
        const err = validateQuery(query);
        setValidationError(err);
        if (err) return;
        setLoading(true);
        setError(null);
        setResults([]);
        setHasMore(false);
        if (status !== "authenticated" || !session?.jwt) {
            setError("You must be logged in to search.");
            setLoading(false);
            return;
        }
        try {
            const pagedQuery = buildPagedQuery(query, targetPage, pageSize);
            const res = await fetch("/api/signal-search", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                    "Authorization": `Bearer ${session?.jwt}`,
                },
                body: JSON.stringify({ query: pagedQuery }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || "Unknown error");
            const rows: SignalRow[] = Array.isArray(data.results) ? data.results : [];
            setResults(rows);
            setPage(targetPage);
            setHasMore(rows.length === pageSize);
            // Optionally, if backend returns total count, setTotalResults(data.total)
        } catch (err) {
            setError(errorMessage(err) || "Error searching signals");
        } finally {
            setLoading(false);
        }
    }

    function handleSearch(e: React.FormEvent) {
        e.preventDefault();
        void searchPage(1);
    }

    return (
        <div className="min-h-screen bg-background text-foreground flex flex-col items-center py-12 px-4">
            <h1 className="text-2xl font-bold mb-8">SQL Query</h1>
            {available === false && <p className="text-muted-foreground mb-6">SQL search is unavailable in this deployment.</p>}
            <div className="mb-2 w-full max-w-xl flex flex-col sm:flex-row gap-4 items-center justify-between">
                <div className="text-muted-foreground text-sm">Current namespace:</div>
                <Input
                    type="text"
                    value={namespace}
                    onChange={handleNamespaceChange}
                    className="w-48 bg-background border border-border rounded px-2 py-1 text-sm"
                />
            </div>
            <form
                ref={formRef}
                onSubmit={handleSearch}
                className="w-full max-w-xl flex flex-col sm:flex-row gap-4 mb-2 bg-card/80 border border-border shadow-lg rounded-xl p-4"
            >
                <Input
                    type="text"
                    placeholder="Enter SQL query (e.g. SELECT * FROM trazr_vectors LIMIT 10)"
                    className="flex-1 bg-background border-none shadow-none focus:ring-2 focus:ring-primary/50 text-lg px-4 py-3 rounded-lg"
                    value={query}
                    onChange={handleInputChange}
                />
                <Button type="submit" className="text-lg px-6 py-3" disabled={loading || !!validationError || available !== true}>
                    {loading ? "Searching..." : "Search"}
                </Button>
            </form>
            <div className="w-full max-w-xl mb-6">
                <div className="text-xs text-muted-foreground font-mono break-all">
                    SQL statement: <span className="font-semibold">{query}</span>
                </div>
            </div>
            {results.length > 0 && (
                <div className="w-full mb-4 flex justify-center">
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setShowFullContent(!showFullContent)}
                    >
                        {showFullContent ? "Truncate Columns" : "Show Full Content"}
                    </Button>
                </div>
            )}
            {validationError && <div className="text-red-500 mb-2">{validationError}</div>}
            {error && <div className="text-red-500 mb-4">{error}</div>}
            <div className="w-full px-4">
                {results.length === 0 && !loading && (
                    <div className="text-muted-foreground text-center">No results yet. Try searching!</div>
                )}
                {results.length > 0 && (
                    <>
                        <div className="flex items-center justify-between mt-2 mb-4">
                            <Button
                                onClick={() => void searchPage(Math.max(1, page - 1))}
                                disabled={page === 1 || loading}
                                variant="outline"
                            >
                                Previous
                            </Button>
                            <span className="text-sm text-muted-foreground">
                                Page {page}{hasMore ? '' : ' (last page)'}
                            </span>
                            <Button
                                onClick={() => { if (hasMore) void searchPage(page + 1); }}
                                disabled={!hasMore || loading}
                                variant="outline"
                            >
                                Next
                            </Button>
                        </div>
                        <div className="overflow-x-auto rounded-lg shadow-lg bg-card border border-border">
                            <Table className="w-full table-auto">
                                <TableHeader>
                                    <TableRow>
                                        <DndContext
                                            collisionDetection={closestCenter}
                                            onDragEnd={(event: DragEndEvent) => {
                                                const { active, over } = event;
                                                if (over && active.id !== over.id) {
                                                    const oldIndex = effectiveColumnOrder.indexOf(active.id as string);
                                                    const newIndex = effectiveColumnOrder.indexOf(over.id as string);
                                                    setColumnOrder(arrayMove(effectiveColumnOrder, oldIndex, newIndex));
                                                }
                                            }}
                                        >
                                            <SortableContext items={table.getHeaderGroups()[0]?.headers.map(h => h.id) || []} strategy={horizontalListSortingStrategy}>
                                                {table.getHeaderGroups()[0]?.headers.map(header => (
                                                    <DraggableTableHead key={header.id} header={header} />
                                                ))}
                                            </SortableContext>
                                        </DndContext>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {table.getRowModel().rows.map(row => (
                                        <TableRow key={row.id}>
                                            {row.getVisibleCells().map(cell => (
                                                <TableCell key={cell.id}>
                                                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                                                </TableCell>
                                            ))}
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        </div>
                        <div className="flex items-center justify-between mt-4">
                            <Button
                                onClick={() => void searchPage(Math.max(1, page - 1))}
                                disabled={page === 1 || loading}
                                variant="outline"
                            >
                                Previous
                            </Button>
                            <span className="text-sm text-muted-foreground">
                                Page {page}{hasMore ? '' : ' (last page)'}
                            </span>
                            <Button
                                onClick={() => { if (hasMore) void searchPage(page + 1); }}
                                disabled={!hasMore || loading}
                                variant="outline"
                            >
                                Next
                            </Button>
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
