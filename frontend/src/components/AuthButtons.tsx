"use client";

import { useSession, signOut } from "next-auth/react";
import Link from "next/link";

export default function AuthButtons() {
    const { status } = useSession();

    if (status === "loading") {
        // Render a placeholder to prevent layout shift while session is loading
        return <div className="h-9 w-36" />;
    }

    if (status === "unauthenticated") {
        return (
            <>
                <Link href="/login" className="ml-4 px-4 py-2 rounded font-medium hover:bg-neutral-100 dark:hover:bg-neutral-800">Sign In</Link>
                <Link href="/register" className="px-4 py-2 rounded font-medium bg-black text-white hover:bg-neutral-900 dark:bg-white dark:text-black dark:hover:bg-neutral-200">Sign Up</Link>
            </>
        );
    }

    // When authenticated, show a logout button
    if (status === "authenticated") {
        return (
            <button
                onClick={() => signOut({ callbackUrl: "/" })}
                className="ml-4 px-4 py-2 rounded font-medium bg-red-600 text-white hover:bg-red-700 transition"
            >
                Logout
            </button>
        );
    }

    return null;
}
