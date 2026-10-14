import { NextRequest } from "next/server";
import { proxySearch } from "@/lib/search-proxy";

export function POST(req: NextRequest) {
    return proxySearch(req, process.env.VECTOR_SEARCH_PATH, "Vector");
}
