import { NextResponse } from "next/server";

export function GET() {
    return NextResponse.json({
        sql: Boolean(process.env.SQL_SEARCH_PATH),
        vector: Boolean(process.env.VECTOR_SEARCH_PATH),
    });
}
