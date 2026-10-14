import type { ReactNode } from "react";
import Header from "@/components/Header";
import { Sidebar } from "../dashboard/layout";

export default function SearchLayout({ children }: { children: ReactNode }) {
    // Sidebar is a named export in dashboard/layout.tsx, so we need to import it correctly
    // If Sidebar is not exported, we need to export it from dashboard/layout.tsx
    return (
        <div className="flex flex-col min-h-screen">
            <Header />
            <div className="flex flex-1">
                <Sidebar />
                <div className="flex-1" style={{ marginLeft: 256 }}>
                    {children}
                </div>
            </div>
        </div>
    );
}
