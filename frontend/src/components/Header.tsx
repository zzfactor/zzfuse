"use client";
import { useMounted } from "@/lib/use-mounted";
import Link from "next/link";
import { SunIcon, MoonIcon, ComputerDesktopIcon } from "@heroicons/react/24/outline";
import { useTheme } from "next-themes";
import AuthButtons from "./AuthButtons";

const menu = [
    { name: "Home", href: "#" },
];

export default function Header() {
    const { theme, setTheme, resolvedTheme } = useTheme();
    const mounted = useMounted();


    return (
        <header className="w-full flex items-center justify-between py-4 px-8 border-b bg-background/80 text-foreground backdrop-blur z-10 fixed top-0 left-0 right-0">
            {/* Logo */}
            <div className="font-bold text-xl tracking-tight">zzfuse</div>
            {/* Menu */}
            <nav className="hidden md:flex gap-8 text-base font-medium">
                {menu.map((item) => (
                    <Link key={item.name} href={item.href} className="hover:text-primary transition-colors">
                        {item.name}
                    </Link>
                ))}
            </nav>
            {/* Right: Theme toggle + Auth */}
            <div className="flex items-center gap-4">
                {/* Theme toggle - only render on client to avoid hydration mismatch */}
                {mounted && (
                    <>
                        <button
                            aria-label="Set light mode"
                            className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${resolvedTheme === "light" ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                            onClick={() => setTheme("light")}
                        >
                            <SunIcon className="w-5 h-5" />
                        </button>
                        <button
                            aria-label="Set dark mode"
                            className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${resolvedTheme === "dark" ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                            onClick={() => setTheme("dark")}
                        >
                            <MoonIcon className="w-5 h-5" />
                        </button>
                        <button
                            aria-label="Set system mode"
                            className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${theme === "system" ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                            onClick={() => setTheme("system")}
                        >
                            <ComputerDesktopIcon className="w-5 h-5" />
                        </button>
                    </>
                )}
                {/* Auth buttons */}
                <AuthButtons />
            </div>
        </header>
    );
}
