"use client";

import { useSession } from "next-auth/react";
import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { FaUsersCog, FaHome, FaChevronLeft, FaChevronRight } from "react-icons/fa";
import Header from "@/components/Header";
import { useState, useRef, useEffect } from "react";
import { isAdmin } from "@/lib/roles";

export function Sidebar() {
    const { data: session, status } = useSession();
    const pathname = usePathname();
    const admin = status === "authenticated" && isAdmin(session);
    const user = session?.user;
    const [dropdownOpen, setDropdownOpen] = useState(false);
    const dropdownRef = useRef<HTMLDivElement>(null);
    // Collapsible sidebar state (persisted)
    const [collapsed, setCollapsed] = useState(() => {
        if (typeof window !== "undefined") {
            return localStorage.getItem("sidebar-collapsed") === "true";
        }
        return false;
    });
    useEffect(() => {
        localStorage.setItem("sidebar-collapsed", String(collapsed));
    }, [collapsed]);

    // Close dropdown on outside click
    useEffect(() => {
        function handleClick(e: MouseEvent) {
            if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
                setDropdownOpen(false);
            }
        }
        if (dropdownOpen) {
            document.addEventListener("mousedown", handleClick);
        } else {
            document.removeEventListener("mousedown", handleClick);
        }
        return () => document.removeEventListener("mousedown", handleClick);
    }, [dropdownOpen]);

    return (
        <aside className={`h-screen ${collapsed ? "w-20" : "w-64"} bg-background border-r flex flex-col justify-between fixed left-0 top-0 z-20 transition-all duration-200`} style={{ marginTop: 64 }}>
            {/* Account info at top */}
            <div className={`flex items-center gap-3 px-4 py-5 border-b ${collapsed ? "justify-center" : ""}`}>
                {!collapsed && (
                    <div>
                        <div className="font-semibold text-base">{user?.name || "Account"}</div>
                        <div className="text-xs text-gray-500">{user?.email}</div>
                    </div>
                )}
                <button
                    className={`ml-auto p-1 rounded hover:bg-muted transition ${collapsed ? "mx-0" : "mx-2"}`}
                    onClick={() => setCollapsed((c) => !c)}
                    aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                >
                    {collapsed ? <FaChevronRight /> : <FaChevronLeft />}
                </button>
            </div>
            <div className="flex-1 flex flex-col justify-between">
                <nav className={`mt-6 ${collapsed ? "px-1" : "px-2"}`}>
                    <div className={`text-xs text-gray-400 ${collapsed ? "px-0 text-center" : "px-4 mb-2"}`}>Application</div>
                    <Link href="/dashboard" className={`flex items-center gap-2 ${collapsed ? "justify-center" : "px-4"} py-2 rounded-lg mb-1 text-sm font-medium transition-colors ${pathname === "/dashboard" ? "bg-gray-100 dark:bg-gray-900 text-primary" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-900"}`}> <FaHome className="text-base" /> {!collapsed && "Dashboard"} </Link>
                    <Link href="/search" className={`flex items-center gap-2 ${collapsed ? "justify-center" : "px-4"} py-2 rounded-lg mb-1 text-sm font-medium transition-colors ${pathname === "/search" ? "bg-gray-100 dark:bg-gray-900 text-primary" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-900"}`}>🔍 {!collapsed && "Vector Search"}</Link>
                    <Link href="/signals" className={`flex items-center gap-2 ${collapsed ? "justify-center" : "px-4"} py-2 rounded-lg mb-1 text-sm font-medium transition-colors ${pathname === "/signals" ? "bg-gray-100 dark:bg-gray-900 text-primary" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-900"}`}>📶 {!collapsed && "SQL Query"}</Link>
                    <Link href="/api-docs" className={`flex items-center gap-2 ${collapsed ? "justify-center" : "px-4"} py-2 rounded-lg mb-1 text-sm font-medium transition-colors ${pathname === "/api-docs" ? "bg-gray-100 dark:bg-gray-900 text-primary" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-900"}`}>🧩 {!collapsed && "API"}</Link>
                    {admin && (
                        <>
                            <div className={`text-xs text-gray-400 ${collapsed ? "px-0 text-center mt-6" : "px-4 mt-6 mb-2"}`}>Admin</div>
                            <Link href="/admin/users" className={`flex items-center gap-2 ${collapsed ? "justify-center" : "px-4"} py-2 rounded-lg mb-1 text-sm font-medium transition-colors ${pathname === "/admin/users" ? "bg-gray-100 dark:bg-gray-900 text-primary" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-900"}`}><FaUsersCog className="text-base" /> {!collapsed && "User Management"}</Link>
                        </>
                    )}
                </nav>
            </div>
        </aside>
    );
}

export default function DashboardLayout({ children }: { children: ReactNode }) {
    // Track sidebar collapsed state to adjust main content margin
    const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
        if (typeof window !== "undefined") {
            return localStorage.getItem("sidebar-collapsed") === "true";
        }
        return false;
    });

    // Listen for storage changes to sync state across components
    useEffect(() => {
        const handleStorageChange = () => {
            setSidebarCollapsed(localStorage.getItem("sidebar-collapsed") === "true");
        };
        window.addEventListener("storage", handleStorageChange);

        // Also check periodically since localStorage events don't fire in same tab
        const interval = setInterval(handleStorageChange, 100);

        return () => {
            window.removeEventListener("storage", handleStorageChange);
            clearInterval(interval);
        };
    }, []);

    return (
        <div className="flex flex-col min-h-screen">
            <Header />
            <div className="flex flex-1" style={{ marginTop: 64 }}>
                <Sidebar />
                <div
                    className="flex-1 transition-all duration-200"
                    style={{ marginLeft: sidebarCollapsed ? 80 : 256 }}
                >
                    {children}
                </div>
            </div>
        </div>
    );
}
