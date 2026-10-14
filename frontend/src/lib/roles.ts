import type { Session } from "next-auth";

export function isAdmin(session: Session | null): boolean {
    return session?.roles?.includes("admin") === true ||
        session?.user?.role === "admin" || session?.user?.roles?.includes("admin") === true;
}
